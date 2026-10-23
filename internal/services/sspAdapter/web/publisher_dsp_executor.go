package sppAdapterWeb

import (
	"context"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

const (
	publisherDSPWorkers  = 8
	publisherDSPCapacity = 1024
)

type publisherDSPTask struct{ id, target string }

// Best-effort notifications only. No billing, Redis lookup or durable retry is
// allowed here. The same legacy HTTP client is used by ordinary callbacks.
type publisherDSPExecutor struct {
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	stopped bool
	tasks   chan publisherDSPTask
	wg      sync.WaitGroup
	active  atomic.Int32
	dropped atomic.Uint64
}

func newPublisherDSPExecutor(parent context.Context, client *http.Client, workers, capacity int) *publisherDSPExecutor {
	ctx, cancel := context.WithCancel(parent)
	e := &publisherDSPExecutor{ctx: ctx, cancel: cancel, tasks: make(chan publisherDSPTask, capacity)}
	for i := 0; i < workers; i++ {
		e.wg.Add(1)
		go func() {
			defer e.wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case task := <-e.tasks:
					if ctx.Err() != nil {
						return
					}
					e.active.Add(1)
					callCtx, stop := context.WithTimeout(ctx, 3*time.Second)
					// Invalid targets and transport/status/body errors are diagnostic;
					// internal BURL has already committed its guard before admission.
					_ = sendBURLCallback(callCtx, task.id, task.target, client, true)
					stop()
					e.active.Add(-1)
				}
			}
		}()
	}
	return e
}

func (e *publisherDSPExecutor) enqueue(id, target string) {
	if target == "" {
		return
	}
	if e == nil {
		log.Printf("publisher DSP executor unavailable uuid=%s; notification dropped", id)
		return
	}
	e.mu.Lock()
	accepted := false
	if !e.stopped && e.ctx.Err() == nil {
		select {
		case e.tasks <- publisherDSPTask{id: id, target: target}:
			accepted = true
		default:
		}
	}
	e.mu.Unlock()
	if !accepted {
		n := e.dropped.Add(1)
		if n == 1 || n&(n-1) == 0 {
			log.Printf("publisher DSP queue full or stopping uuid=%s dropped=%d", id, n)
		}
	}
}

// Cancel in-flight HTTP and discard queued best-effort notifications. Admission
// never waits for these workers, including during concurrent/repeated Close.
func (e *publisherDSPExecutor) Close() {
	e.mu.Lock()
	e.stopped = true
	e.cancel()
	e.mu.Unlock()
	e.wg.Wait()
}
