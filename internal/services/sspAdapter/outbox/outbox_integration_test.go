package outbox

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestOutboxPersistenceAndConflict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "outbox.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	record := Record{EventID: "e", UserID: "u", CampaignID: "c", Price: 0.1, Format: "IPP", Source: "adm", CreatedAt: time.Now().UTC(), Attempts: 1}
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	records, err := store.List()
	if err != nil || len(records) != 1 || records[0].EventID != "e" {
		t.Fatalf("records=%#v err=%v", records, err)
	}
	conflict := record
	conflict.Price = 0.2
	if err := store.Save(conflict); !errors.Is(err, ErrEventConflict) {
		t.Fatalf("want conflict, got %v", err)
	}
}
