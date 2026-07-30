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

const (
	KindBilling = "billing"
	KindADM     = "adm"

	WinnerUnknown = "unknown"
	WinnerDSP     = "dsp"
	WinnerADV     = "adv"
)

type Record struct {
	Kind          string    `json:"kind,omitempty"`
	EventID       string    `json:"event_id"`
	GlobalID      string    `json:"global_id,omitempty"`
	ClickID       string    `json:"click_id,omitempty"`
	WinnerType    string    `json:"winner_type,omitempty"`
	UserID        string    `json:"user_id,omitempty"`
	CampaignID    string    `json:"campaign_id,omitempty"`
	Price         float64   `json:"price,omitempty"`
	Format        string    `json:"format"`
	Source        string    `json:"source"`
	CreatedAt     time.Time `json:"created_at"`
	Attempts      int       `json:"attempts"`
	LastError     string    `json:"last_error,omitempty"`
	LastAttemptAt time.Time `json:"last_attempt_at,omitempty"`
}

func NormalizeKind(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return KindBilling
	}
	return value
}

func NormalizeWinnerType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case WinnerDSP:
		return WinnerDSP
	case WinnerADV:
		return WinnerADV
	default:
		return WinnerUnknown
	}
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
	record = normalizeRecord(record)
	if err := validateRecord(record); err != nil {
		return err
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
			if !sameEvent(normalizeRecord(old), record) {
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
			result = append(result, normalizeRecord(record))
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
		record = normalizeRecord(record)
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

func (s *Store) UpdateResolution(eventID, winnerType, userID, campaignID string, price float64) error {
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
		record = normalizeRecord(record)
		if NormalizeKind(record.Kind) != KindADM {
			return errors.New("winner resolution is only valid for ADM records")
		}
		record.WinnerType = NormalizeWinnerType(winnerType)
		record.UserID = strings.TrimSpace(userID)
		record.CampaignID = strings.TrimSpace(campaignID)
		record.Price = price
		if err := validateRecord(record); err != nil {
			return err
		}
		encoded, err := json.Marshal(record)
		if err != nil {
			return err
		}
		return bucket.Put([]byte(eventID), encoded)
	})
}

func normalizeRecord(record Record) Record {
	record.Kind = NormalizeKind(record.Kind)
	record.EventID = strings.TrimSpace(record.EventID)
	record.GlobalID = strings.TrimSpace(record.GlobalID)
	record.ClickID = strings.TrimSpace(record.ClickID)
	record.WinnerType = NormalizeWinnerType(record.WinnerType)
	record.UserID = strings.TrimSpace(record.UserID)
	record.CampaignID = strings.TrimSpace(record.CampaignID)
	record.Format = strings.ToUpper(strings.TrimSpace(record.Format))
	record.Source = strings.ToLower(strings.TrimSpace(record.Source))
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	} else {
		record.CreatedAt = record.CreatedAt.UTC()
	}
	if record.Attempts < 1 {
		record.Attempts = 1
	}
	return record
}

func validateRecord(record Record) error {
	if record.EventID == "" {
		return errors.New("outbox record has empty event_id")
	}
	if record.Format == "" || record.Source == "" {
		return errors.New("outbox record has empty format or source")
	}
	switch NormalizeKind(record.Kind) {
	case KindBilling:
		if record.UserID == "" || record.CampaignID == "" {
			return errors.New("billing outbox record has empty IDs")
		}
		if record.Price <= 0 || math.IsNaN(record.Price) || math.IsInf(record.Price, 0) {
			return errors.New("billing outbox record has invalid price")
		}
	case KindADM:
		if record.GlobalID == "" || record.ClickID == "" {
			return errors.New("ADM outbox record has empty global_id or click_id")
		}
		winnerType := NormalizeWinnerType(record.WinnerType)
		if winnerType == WinnerADV {
			if record.UserID == "" || record.CampaignID == "" {
				return errors.New("ADV ADM outbox record has empty winner IDs")
			}
			if record.Price <= 0 || math.IsNaN(record.Price) || math.IsInf(record.Price, 0) {
				return errors.New("ADV ADM outbox record has invalid price")
			}
		}
	default:
		return fmt.Errorf("unsupported outbox record kind %q", record.Kind)
	}
	return nil
}

func sameEvent(a, b Record) bool {
	return NormalizeKind(a.Kind) == NormalizeKind(b.Kind) &&
		a.EventID == b.EventID && a.GlobalID == b.GlobalID && a.ClickID == b.ClickID &&
		NormalizeWinnerType(a.WinnerType) == NormalizeWinnerType(b.WinnerType) &&
		a.UserID == b.UserID && a.CampaignID == b.CampaignID && a.Price == b.Price &&
		strings.EqualFold(a.Format, b.Format) && strings.EqualFold(a.Source, b.Source)
}
