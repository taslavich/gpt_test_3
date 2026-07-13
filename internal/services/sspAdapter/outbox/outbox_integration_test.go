package outbox

import "testing"

func TestStoreIdempotentSaveAndUpdateFailure(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Save(Record{EventID: "event-1", UserID: "u1", CampaignID: "c1", Price: 1, Format: "IPP", Source: "adm"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(Record{EventID: "event-1", UserID: "u2", CampaignID: "c2", Price: 2, Format: "BAN", Source: "burl"}); err != nil {
		t.Fatal(err)
	}
	recs, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].UserID != "u1" || recs[0].Price != 1 {
		t.Fatalf("idempotent save overwritten record: %+v", recs)
	}
	if err := s.UpdateFailure("event-1", "redis down"); err != nil {
		t.Fatal(err)
	}
	recs, err = s.List()
	if err != nil {
		t.Fatal(err)
	}
	if recs[0].Attempts != 1 || recs[0].LastError != "redis down" {
		t.Fatalf("failure state not updated: %+v", recs[0])
	}
}
