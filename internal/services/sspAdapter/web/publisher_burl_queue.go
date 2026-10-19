package sppAdapterWeb

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/callback"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/billing"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/outbox"
)

type publisherBURLPayload struct {
	ID     string
	Record callback.Record
	Winner billing.Winner
}

type PublisherBURLOptions struct {
	Workers   int
	Timeout   time.Duration
	HighWater int
	MaxAge    time.Duration
}

type publisherBURLQueue struct {
	box              *outbox.Store
	dsp              *redis.Client
	adv              *billing.Store
	process          func(context.Context, resolvedCallback) error
	timeout          time.Duration
	ctx              context.Context
	cancel           context.CancelFunc
	mu               sync.Mutex
	stopped          bool
	fallback         chan resolvedCallback
	fallbackAttempts atomic.Uint64
	lost             atomic.Uint64
	wg               sync.WaitGroup
	slots            chan struct{}
	wake             chan struct{}
	highWater        int
	failure          chan error
}

func newPublisherBURLQueue(parent context.Context, box *outbox.Store, dsp *redis.Client, adv *billing.Store, concurrency, highWater int, timeout, maxAge time.Duration, process func(context.Context, resolvedCallback) error, report ...func(error)) *publisherBURLQueue {
	ctx, cancel := context.WithCancel(parent)
	q := &publisherBURLQueue{box: box, dsp: dsp, adv: adv, process: process, timeout: timeout, ctx: ctx, cancel: cancel, fallback: make(chan resolvedCallback, concurrency*128), slots: make(chan struct{}, concurrency), wake: make(chan struct{}, 1), highWater: highWater, failure: make(chan error, 1)}
	// One fixed persister handles overflow; successful immediate jobs never enter
	// Bolt. A finite staging buffer keeps filesystem stalls off the HTTP path.
	q.wg.Add(1)
	go func() {
		defer q.wg.Done()
		for r := range q.fallback {
			q.persist(r)
		}
	}()
	if box == nil {
		return q
	}
	q.wg.Add(1)
	go func() {
		defer q.wg.Done()
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		lastReport := time.Time{}
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			case <-q.wake:
			case err := <-q.failure:
				if len(report) > 0 {
					report[0](err)
				}
			}
			if time.Since(lastReport) > 10*time.Second {
				lastReport = time.Now()
				if pending, e := box.TaskCount(); e == nil {
					log.Printf("publisher BURL queue pending=%d active=%d capacity=%d staging=%d fallback_attempts=%d lost=%d", pending, len(q.slots), cap(q.slots), len(q.fallback), q.fallbackAttempts.Load(), q.lost.Load())
					if pending >= q.highWater && len(report) > 0 {
						report[0](fmt.Errorf("publisher BURL durable backlog high water: %d", pending))
					}
				}
			}
			free := cap(q.slots) - len(q.slots)
			if free == 0 {
				continue
			}
			tasks, e := box.ClaimTasks(time.Now().UTC(), free, timeout+3*time.Minute)
			if e != nil {
				log.Printf("publisher BURL claim: %v", e)
				continue
			}
			for _, t := range tasks {
				q.slots <- struct{}{}
				q.wg.Add(1)
				go func(t outbox.Task) {
					defer q.wg.Done()
					defer func() {
						<-q.slots
						select {
						case q.wake <- struct{}{}:
						default:
						}
					}()
					taskCtx, stop := context.WithTimeout(ctx, timeout)
					defer stop()
					var payload publisherBURLPayload
					var e error
					held := t.CreatedAt.IsZero() || time.Since(t.CreatedAt) >= maxAge
					if held {
						e = fmt.Errorf("publisher task beyond automatic retry horizon; retained for operator review")
						if len(report) > 0 {
							report[0](e)
						}
					} else {
						e = json.Unmarshal(t.Payload, &payload)
					}
					if e == nil {
						if payload.ID != t.ID || !callback.ValidID(payload.ID) || payload.Record.Format != "POP" || payload.Record.PublisherID == "" {
							e = fmt.Errorf("invalid persisted Publisher POP task")
						} else {
							e = payload.Record.Validate()
						}
					}
					if e == nil {
						resolved := resolvedCallback{ID: payload.ID, Record: payload.Record, Winner: payload.Winner, Redis: dsp}
						if payload.Record.WinnerType == "adv" {
							if adv == nil {
								e = fmt.Errorf("ADV store unavailable")
							} else {
								resolved.Redis = adv.CallbackRedis()
							}
						}
						if e == nil {
							e = process(taskCtx, resolved)
						}
					}
					retry := time.Time{}
					if e != nil {
						backoff := time.Second
						for i := 1; i < t.Attempts && backoff < time.Minute; i++ {
							backoff *= 2
						}
						if backoff > time.Minute {
							backoff = time.Minute
						}
						if held {
							backoff = 24 * time.Hour
						}
						retry = time.Now().UTC().Add(backoff)
						log.Printf("publisher BURL task uuid=%s attempt=%d: %v", t.ID, t.Attempts, e)
					}
					if e = box.FinishTask(t, retry); e != nil {
						log.Printf("publisher BURL ACK uuid=%s: %v", t.ID, e)
					}
				}(t)
			}
		}
	}()
	return q
}

// Admission is nonblocking and bounded. The route drains handlers before Close.
func (q *publisherBURLQueue) enqueue(_ context.Context, r resolvedCallback) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.stopped {
		q.fail(r.ID, fmt.Errorf("publisher BURL admission after shutdown"))
		return
	}
	select {
	case q.slots <- struct{}{}:
		q.wg.Add(1)
		go func() {
			defer q.wg.Done()
			defer func() {
				<-q.slots
				select {
				case q.wake <- struct{}{}:
				default:
				}
			}()
			taskCtx, stop := context.WithTimeout(q.ctx, q.timeout)
			defer stop()
			// External BURL and internal ADM must share the same DSP winner guard.
			if r.Record.WinnerType == "dsp" && q.dsp != nil {
				r.Redis = q.dsp
			}
			if e := q.process(taskCtx, r); e != nil {
				log.Printf("publisher BURL fast path uuid=%s: %v", r.ID, e)
				q.persist(r)
			}
		}()
	default:
		select {
		case q.fallback <- r:
		default:
			// No bounded nonblocking queue can absorb unlimited arrivals while disk
			// is stalled. Signal overload and explicitly lose this best-effort BURL;
			// never delay navigation or replace the independent ADV financial intent.
			q.fail(r.ID, fmt.Errorf("publisher BURL fallback staging full"))
		}
	}
}

func (q *publisherBURLQueue) persist(r resolvedCallback) {
	q.fallbackAttempts.Add(1)
	payload, e := json.Marshal(publisherBURLPayload{ID: r.ID, Record: r.Record, Winner: r.Winner})
	if e == nil {
		if q.box == nil {
			e = fmt.Errorf("publisher BURL outbox unavailable")
		} else {
			e = q.box.SaveTask(r.ID, payload)
		}
	}
	if e != nil {
		q.fail(r.ID, e)
		return
	}
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

func (q *publisherBURLQueue) fail(id string, e error) {
	n := q.lost.Add(1)
	// Log the first loss and powers of two to avoid an overload log flood.
	if n == 1 || n&(n-1) == 0 {
		log.Printf("CRITICAL publisher BURL not durable uuid=%s lost=%d: %v", id, n, e)
	}
	select {
	case q.failure <- e:
	default:
	}
}

// Cancel active work, persist failed fast jobs, and drain overflow staging before
// closing the shared outbox. An abrupt crash may lose RAM jobs by agreement.
func (q *publisherBURLQueue) Close() {
	q.mu.Lock()
	if !q.stopped {
		q.stopped = true
		q.cancel()
		close(q.fallback)
	}
	q.mu.Unlock()
	q.wg.Wait()
}
