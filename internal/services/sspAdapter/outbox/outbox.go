package outbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

var bucketName = []byte("adv_billing_events")

var ErrEventConflict = errors.New("outbox event ID already exists with different payload")

type Record struct {
	EventID       string    `json:"event_id"`
	UserID        string    `json:"user_id"`
	CampaignID    string    `json:"campaign_id"`
	Price         float64   `json:"price"`
	Format        string    `json:"format"`
	Source        string    `json:"source"`
	CreatedAt     time.Time `json:"created_at"`
	Attempts      int       `json:"attempts"`
	LastError     string    `json:"last_error,omitempty"`
	LastAttemptAt time.Time `json:"last_attempt_at,omitempty"`
}

type Store struct {
	db *bolt.DB
}

func Open(path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("outbox path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("create outbox directory: %w", err)
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open outbox: %w", err)
	}
	store := &Store{db: db}
	if err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucketName)
		return err
	}); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize outbox: %w", err)
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) Save(record Record) error {
	if s == nil || s.db == nil {
		return errors.New("outbox is not initialized")
	}
	if err := validateRecord(record); err != nil {
		return err
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	if record.Attempts < 1 {
		record.Attempts = 1
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bucketName)
		existing := bucket.Get([]byte(record.EventID))
		if existing != nil {
			var old Record
			if err := json.Unmarshal(append([]byte(nil), existing...), &old); err != nil {
				return err
			}
			if !sameEvent(old, record) {
				return ErrEventConflict
			}
			return nil
		}
		return bucket.Put([]byte(record.EventID), encoded)
	})
}

func (s *Store) List() ([]Record, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("outbox is not initialized")
	}
	result := make([]Record, 0)
	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bucketName)
		return bucket.ForEach(func(_, value []byte) error {
			var record Record
			if err := json.Unmarshal(append([]byte(nil), value...), &record); err != nil {
				return err
			}
			result = append(result, record)
			return nil
		})
	})
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result, err
}

func (s *Store) Delete(eventID string) error {
	if s == nil || s.db == nil {
		return errors.New("outbox is not initialized")
	}
	return s.db.Update(func(tx *bolt.Tx) error { return tx.Bucket(bucketName).Delete([]byte(eventID)) })
}

func (s *Store) UpdateFailure(eventID string, applyErr error) error {
	if s == nil || s.db == nil {
		return errors.New("outbox is not initialized")
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bucketName)
		raw := bucket.Get([]byte(eventID))
		if raw == nil {
			return errors.New("outbox record not found")
		}
		var record Record
		if err := json.Unmarshal(append([]byte(nil), raw...), &record); err != nil {
			return err
		}
		record.Attempts++
		record.LastAttemptAt = time.Now().UTC()
		if applyErr != nil {
			record.LastError = applyErr.Error()
		}
		encoded, err := json.Marshal(record)
		if err != nil {
			return err
		}
		return bucket.Put([]byte(eventID), encoded)
	})
}

func validateRecord(record Record) error {
	if strings.TrimSpace(record.EventID) == "" || strings.TrimSpace(record.UserID) == "" || strings.TrimSpace(record.CampaignID) == "" {
		return errors.New("outbox record has empty IDs")
	}
	if record.Price <= 0 || math.IsNaN(record.Price) || math.IsInf(record.Price, 0) {
		return errors.New("outbox record has invalid price")
	}
	if strings.TrimSpace(record.Format) == "" || strings.TrimSpace(record.Source) == "" {
		return errors.New("outbox record has empty format or source")
	}
	return nil
}

func sameEvent(a, b Record) bool {
	return a.EventID == b.EventID && a.UserID == b.UserID && a.CampaignID == b.CampaignID &&
		a.Price == b.Price && strings.EqualFold(a.Format, b.Format) && strings.EqualFold(a.Source, b.Source)
}
