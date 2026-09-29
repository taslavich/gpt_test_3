package sppAdapterWeb

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFeedStoreManualFileEditNeedsRestartButPutUpdatesRuntime(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "feeds.json")
	if err := os.WriteFile(filename, []byte(`{"feed-a":"ssp-a.example"}`), 0644); err != nil {
		t.Fatal(err)
	}

	store, err := NewFeedStore(filename)
	if err != nil {
		t.Fatal(err)
	}
	if domain, ok := store.Lookup("feed-a"); !ok || domain != "ssp-a.example" {
		t.Fatalf("startup lookup = %q, %v", domain, ok)
	}

	if err := os.WriteFile(filename, []byte(`{"feed-a":"ssp-edited.example"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if domain, _ := store.Lookup("feed-a"); domain != "ssp-a.example" {
		t.Fatalf("manual edit unexpectedly changed runtime: %q", domain)
	}
	raw, err := store.ReadRaw()
	if err != nil {
		t.Fatal(err)
	}
	if raw["feed-a"] != "ssp-edited.example" {
		t.Fatalf("raw map did not reflect disk edit: %#v", raw)
	}

	if err := store.Update(FeedMap{"feed-a": "ssp-put.example"}); err != nil {
		t.Fatal(err)
	}
	if domain, _ := store.Lookup("feed-a"); domain != "ssp-put.example" {
		t.Fatalf("PUT did not update runtime: %q", domain)
	}

	restarted, err := NewFeedStore(filename)
	if err != nil {
		t.Fatal(err)
	}
	if domain, _ := restarted.Lookup("feed-a"); domain != "ssp-put.example" {
		t.Fatalf("restart did not load persisted PUT: %q", domain)
	}
}

func TestFormatFeedRoutesSelectsFormatAndVertical(t *testing.T) {
	mk := func(name string) *FeedStore {
		filename := filepath.Join(t.TempDir(), name+".json")
		if err := os.WriteFile(filename, []byte(`{"1":"`+name+`"}`), 0644); err != nil {
			t.Fatal(err)
		}
		store, err := NewFeedStore(filename)
		if err != nil {
			t.Fatal(err)
		}
		return store
	}

	popAdult := mk("pop-adult")
	ippMainstream := mk("ipp-mainstream")
	routes := &FormatFeedRoutesV25{
		POP: FormatFeedRouteV25{Adult: popAdult},
		IPP: FormatFeedRouteV25{Mainstream: ippMainstream},
	}

	if routes.Select("POP", ADULT) != popAdult {
		t.Fatal("POP/ADULT selected wrong store")
	}
	if routes.Select("IPP", MAINSTREAM) != ippMainstream {
		t.Fatal("IPP/MAINSTREAM selected wrong store")
	}
	if routes.Select("VID", ADULT) != nil {
		t.Fatal("unexpected VID/ADULT store")
	}
	if requestedFeedFormat("") != "POP" {
		t.Fatal("empty format must preserve POP compatibility")
	}
}
