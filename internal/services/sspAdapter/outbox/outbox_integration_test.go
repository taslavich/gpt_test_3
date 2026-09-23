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
	record := Record{EventID: "e", UserID: "u", CampaignID: "c", PromoStateCaptured: true, PromoActive: true, PromoGeneration: 4, Price: 0.1, Format: "IPP", Source: "adm", CreatedAt: time.Now().UTC(), Attempts: 1}
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
	if err != nil || len(records) != 1 || records[0].EventID != "e" || !records[0].PromoStateCaptured || !records[0].PromoActive || records[0].PromoGeneration != 4 {
		t.Fatalf("records=%#v err=%v", records, err)
	}
	conflict := record
	conflict.Price = 0.2
	if err := store.Save(conflict); !errors.Is(err, ErrEventConflict) {
		t.Fatalf("want conflict, got %v", err)
	}
	generationConflict := record
	generationConflict.PromoGeneration = 5
	if err := store.Save(generationConflict); !errors.Is(err, ErrEventConflict) {
		t.Fatalf("want promo generation conflict, got %v", err)
	}
}

func TestADMOutboxPersistsUnknownWinnerAndResolution(t *testing.T) {
	path := filepath.Join(t.TempDir(), "adm-outbox.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	record := Record{
		Kind:       KindADM,
		EventID:    "adm:click-1",
		GlobalID:   "winner-1",
		ClickID:    "click-1",
		WinnerType: WinnerUnknown,
		Format:     "IPP",
		Source:     "adm",
	}
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateResolution(record.EventID, WinnerADV, "user-1", "campaign-1", 0.5, 2, true, true, 4); err != nil {
		t.Fatal(err)
	}
	records, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("records=%#v", records)
	}
	got := records[0]
	if got.WinnerType != WinnerADV || got.UserID != "user-1" || got.CampaignID != "campaign-1" || got.TypeModel != 2 || !got.PromoStateCaptured || !got.PromoActive || got.PromoGeneration != 4 || got.Price != 0.5 {
		t.Fatalf("resolved record=%#v", got)
	}
}
