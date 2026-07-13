package outbox

import "testing"

func TestStorePersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Save(Record{EventID: "event-1", UserID: "user-1", CampaignID: "camp-1", Price: 1.25, Format: "BAN", Source: "burl"}); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	count := 0
	if err := s.ForEach(func(r Record) error {
		count++
		if r.EventID != "event-1" {
			t.Fatalf("unexpected event %q", r.EventID)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected one record, got %d", count)
	}
	if err := s.Delete("event-1"); err != nil {
		t.Fatal(err)
	}
	count = 0
	_ = s.ForEach(func(r Record) error { count++; return nil })
	if count != 0 {
		t.Fatalf("expected empty outbox after delete, got %d", count)
	}
}
