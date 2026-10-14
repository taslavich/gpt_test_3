package pub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/redis/go-redis/v9"
	shard "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/redis"
	"go.etcd.io/bbolt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// Only pre-admission errors may enter this outbox. It retries the same atomic
// Redis HASH+ready-set script, never publishes a separate Kafka event.
// Batch coalesces concurrent durable writes; callers wait for fsync and quota
// failures propagate as backpressure (503). No unbounded in-memory queue.
type Telemetry struct {
	db       *bbolt.DB
	clients  []*redis.Client
	set      string
	maxBytes int64
	mu       sync.Mutex
	wake     chan struct{}
}

func NewTelemetry(dir string, clients []*redis.Client, set string, maxBytes int64) (*Telemetry, error) {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return nil, e
	}
	db, e := bbolt.Open(filepath.Join(dir, "errors-v2.db"), 0600, &bbolt.Options{Timeout: time.Second})
	if e != nil {
		return nil, e
	}
	db.MaxBatchSize = 1000
	db.MaxBatchDelay = 5 * time.Millisecond
	e = db.Update(func(tx *bbolt.Tx) error {
		for _, name := range []string{"errors", "meta"} {
			if _, e := tx.CreateBucketIfNotExists([]byte(name)); e != nil {
				return e
			}
		}
		return nil
	})
	if e != nil {
		db.Close()
		return nil, e
	}
	return &Telemetry{db: db, clients: clients, set: set, maxBytes: maxBytes, wake: make(chan struct{}, 1)}, nil
}
func (t *Telemetry) Record(id string, fields map[string]string) error {
	if t == nil {
		return errors.New("telemetry unavailable")
	}
	b, e := json.Marshal(fields)
	if e != nil {
		return e
	}
	e = t.db.Batch(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte("errors"))
		if bucket.Get([]byte(id)) != nil {
			return nil
		}
		meta := tx.Bucket([]byte("meta"))
		used, _ := strconv.ParseInt(string(meta.Get([]byte("bytes"))), 10, 64)
		used += int64(len(id) + len(b))
		if used > t.maxBytes {
			return errors.New("durable error outbox full")
		}
		if e := bucket.Put([]byte(id), b); e != nil {
			return e
		}
		return meta.Put([]byte("bytes"), []byte(strconv.FormatInt(used, 10)))
	})
	if e == nil {
		select {
		case t.wake <- struct{}{}:
		default:
		}
	}
	return e
}
func (t *Telemetry) Flush(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	// Drain continuously in bounded batches; throughput is not capped at 1000/s.
	for ctx.Err() == nil {
		type item struct {
			id   string
			data []byte
		}
		batch := []item{}
		e := t.db.View(func(tx *bbolt.Tx) error {
			cur := tx.Bucket([]byte("errors")).Cursor()
			for k, v := cur.First(); k != nil && len(batch) < 256; k, v = cur.Next() {
				batch = append(batch, item{string(k), append([]byte{}, v...)})
			}
			return nil
		})
		if e != nil {
			return e
		}
		if len(batch) == 0 {
			return nil
		}
		done := []item{}
		// Pipeline independent Lua commits across shards. Never retry an adapter's
		// object as a new error; errorLua atomically returns 0 for claimed UUIDs.
		pipes := make(map[*redis.Client]redis.Pipeliner)
		commands := make([]*redis.Cmd, len(batch))
		for i, v := range batch {
			client, _, err := shard.SelectShard(t.clients, v.id)
			if err != nil {
				return err
			}
			pipe := pipes[client]
			if pipe == nil {
				pipe = client.Pipeline()
				pipes[client] = pipe
			}
			commands[i] = pipe.Eval(ctx, errorLua, []string{v.id, v.id + ":pub-uuid", t.set}, v.data, int64(24*time.Hour/time.Millisecond))
		}
		for _, pipe := range pipes {
			_, err := pipe.Exec(ctx)
			if err != nil {
				e = err
			}
		}
		for i, cmd := range commands {
			if _, err := cmd.Int(); err == nil {
				done = append(done, batch[i])
			}
		}
		if len(done) > 0 {
			if err := t.db.Update(func(tx *bbolt.Tx) error {
				bucket := tx.Bucket([]byte("errors"))
				meta := tx.Bucket([]byte("meta"))
				used, _ := strconv.ParseInt(string(meta.Get([]byte("bytes"))), 10, 64)
				for _, v := range done {
					if old := bucket.Get([]byte(v.id)); old != nil {
						used -= int64(len(v.id) + len(old))
						if e := bucket.Delete([]byte(v.id)); e != nil {
							return e
						}
					}
				}
				return meta.Put([]byte("bytes"), []byte(strconv.FormatInt(used, 10)))
			}); err != nil {
				return err
			}
		}
		if e != nil {
			return fmt.Errorf("error outbox Redis flush: %w", e)
		}
	}
	return ctx.Err()
}
func (t *Telemetry) Run(ctx context.Context) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.wake:
		case <-tick.C:
		}
		work, cancel := context.WithTimeout(ctx, 5*time.Second)
		e := t.Flush(work)
		cancel()
		if e != nil && ctx.Err() == nil {
			log.Printf("publisher error outbox: %v", e)
		}
	}
}
func (t *Telemetry) Close() error { return t.db.Close() }
