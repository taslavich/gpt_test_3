package outbox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

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
type Store struct{ dir string }

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(path, 0700); err != nil {
		return nil, err
	}
	return &Store{dir: path}, nil
}
func (s *Store) Close() error { return nil }
func (s *Store) Save(r Record) error {
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}
	r.UpdatedAt = time.Now().UTC()
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	tmp := filepath.Join(s.dir, r.EventID+".tmp")
	dst := filepath.Join(s.dir, r.EventID+".json")
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}
func (s *Store) Delete(eventID string) error {
	err := os.Remove(filepath.Join(s.dir, eventID+".json"))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
func (s *Store) ForEach(fn func(Record) error) error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(s.dir, e.Name()))
		if err != nil {
			return err
		}
		var r Record
		if err := json.Unmarshal(b, &r); err != nil {
			return err
		}
		if err := fn(r); err != nil {
			return err
		}
	}
	return nil
}
