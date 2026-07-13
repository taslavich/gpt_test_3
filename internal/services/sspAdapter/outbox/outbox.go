package outbox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"
)

var pendingBucket = []byte("pending")

type Record struct {
	EventID    string    `json:"event_id"`
	UserID     string    `json:"user_id"`
	CampaignID string    `json:"campaign_id"`
	Price      float64   `json:"price"`
	Format     string    `json:"format"`
	Source     string    `json:"source"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	Attempts   int       `json:"attempts"`
	LastError  string    `json:"last_error"`
}

type Store struct{ db *bolt.DB }

func Open(path string) (*Store, error) {
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		path = filepath.Join(path, "billing_outbox.db")
	}
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	err = db.Update(func(tx *bolt.Tx) error { _, err := tx.CreateBucketIfNotExists(pendingBucket); return err })
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}
func (s *Store) Save(r Record) error {
	now := time.Now().UTC()
	if r.CreatedAt.IsZero() {
		r.CreatedAt = now
	}
	r.UpdatedAt = now
	return s.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(pendingBucket)
		if err != nil {
			return err
		}
		if existing := b.Get([]byte(r.EventID)); existing != nil {
			return nil
		}
		enc, err := json.Marshal(r)
		if err != nil {
			return err
		}
		return b.Put([]byte(r.EventID), enc)
	})
}
func (s *Store) List() ([]Record, error) {
	out := []Record{}
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(pendingBucket)
		if b == nil {
			return nil
		}
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			_ = k
			var r Record
			if err := json.Unmarshal(v, &r); err != nil {
				return err
			}
			out = append(out, r)
		}
		return nil
	})
	return out, err
}
func (s *Store) ForEach(fn func(Record) error) error {
	rs, err := s.List()
	if err != nil {
		return err
	}
	for _, r := range rs {
		if err := fn(r); err != nil {
			return err
		}
	}
	return nil
}
func (s *Store) UpdateFailure(eventID, lastErr string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(pendingBucket)
		if b == nil {
			return nil
		}
		raw := b.Get([]byte(eventID))
		if raw == nil {
			return nil
		}
		var r Record
		if err := json.Unmarshal(raw, &r); err != nil {
			return err
		}
		r.Attempts++
		r.LastError = lastErr
		r.UpdatedAt = time.Now().UTC()
		enc, err := json.Marshal(r)
		if err != nil {
			return err
		}
		return b.Put([]byte(eventID), enc)
	})
}
func (s *Store) Delete(eventID string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(pendingBucket)
		if b == nil {
			return nil
		}
		return b.Delete([]byte(eventID))
	})
}
