package outbox

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"time"

	bolt "go.etcd.io/bbolt"
)

// Publisher tasks share the existing fsynced Bolt outbox, but have their own
// due-time index. Billing recovery never loads this backlog into memory.
var taskBucket = []byte("publisher_burl_tasks_v1")
var dueBucket = []byte("publisher_burl_due_v1")

type Task struct {
	ID        string          `json:"id"`
	CreatedAt time.Time       `json:"created_at"`
	Payload   json.RawMessage `json:"payload"`
	Due       time.Time       `json:"due"`
	Attempts  int             `json:"attempts"`
}

func dueKey(t Task) []byte {
	b := make([]byte, 8+len(t.ID))
	binary.BigEndian.PutUint64(b, uint64(t.Due.UnixNano()))
	copy(b[8:], t.ID)
	return b
}
func taskBuckets(tx *bolt.Tx) (*bolt.Bucket, *bolt.Bucket, error) {
	b, e := tx.CreateBucketIfNotExists(taskBucket)
	if e != nil {
		return nil, nil, e
	}
	d, e := tx.CreateBucketIfNotExists(dueBucket)
	return b, d, e
}

// SaveTask durably retains failed/overflow Publisher work before its retry.
// Repeated fallback for a winner cannot change its trusted payload.
func (s *Store) SaveTask(id string, payload json.RawMessage) error {
	if s == nil || s.db == nil {
		return errors.New("publisher outbox unavailable")
	}
	if id == "" || !json.Valid(payload) {
		return errors.New("invalid publisher task")
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b, d, e := taskBuckets(tx)
		if e != nil {
			return e
		}
		if old := b.Get([]byte(id)); old != nil {
			var t Task
			if e = json.Unmarshal(old, &t); e != nil {
				return e
			}
			if string(t.Payload) != string(payload) {
				return ErrEventConflict
			}
			return nil
		}
		t := Task{ID: id, Payload: payload, Due: time.Now().UTC(), CreatedAt: time.Now().UTC()}
		raw, e := json.Marshal(t)
		if e != nil {
			return e
		}
		if e = b.Put([]byte(id), raw); e != nil {
			return e
		}
		return d.Put(dueKey(t), []byte(id))
	})
}

// ClaimTasks atomically leases at most limit jobs. A crash, canceled worker, or
// failed ACK leaves the durable lease due again. Memory is O(limit), not O(backlog).
func (s *Store) ClaimTasks(now time.Time, limit int, lease time.Duration) ([]Task, error) {
	tasks := make([]Task, 0, limit)
	e := s.db.Update(func(tx *bolt.Tx) error {
		b, d, e := taskBuckets(tx)
		if e != nil {
			return e
		}
		c := d.Cursor()
		for k, v := c.First(); k != nil && len(tasks) < limit; k, v = c.Next() {
			if int64(binary.BigEndian.Uint64(k[:8])) > now.UnixNano() {
				break
			}
			raw := b.Get(v)
			if raw == nil {
				return errors.New("publisher outbox index mismatch")
			}
			var t Task
			if e = json.Unmarshal(raw, &t); e != nil {
				return e
			}
			tasks = append(tasks, t)
		}
		for i := range tasks {
			t := &tasks[i]
			if e = d.Delete(dueKey(*t)); e != nil {
				return e
			}
			t.Due = now.Add(lease)
			t.Attempts++
			raw, e := json.Marshal(t)
			if e != nil {
				return e
			}
			if e = b.Put([]byte(t.ID), raw); e != nil {
				return e
			}
			if e = d.Put(dueKey(*t), []byte(t.ID)); e != nil {
				return e
			}
		}
		return nil
	})
	return tasks, e
}
func (s *Store) FinishTask(t Task, retry time.Time) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b, d, e := taskBuckets(tx)
		if e != nil {
			return e
		}
		raw := b.Get([]byte(t.ID))
		if raw == nil {
			return nil
		}
		var current Task
		if e = json.Unmarshal(raw, &current); e != nil {
			return e
		}
		// A stale completion cannot erase a newer lease.
		if !current.Due.Equal(t.Due) || current.Attempts != t.Attempts {
			return errors.New("stale publisher task lease")
		}
		if e = d.Delete(dueKey(current)); e != nil {
			return e
		}
		if retry.IsZero() {
			return b.Delete([]byte(t.ID))
		}
		current.Due = retry
		raw, e = json.Marshal(current)
		if e != nil {
			return e
		}
		if e = b.Put([]byte(t.ID), raw); e != nil {
			return e
		}
		return d.Put(dueKey(current), []byte(t.ID))
	})
}
func (s *Store) TaskCount() (int, error) {
	n := 0
	e := s.db.View(func(tx *bolt.Tx) error {
		if b := tx.Bucket(taskBucket); b != nil {
			n = b.Stats().KeyN
		}
		return nil
	})
	return n, e
}
