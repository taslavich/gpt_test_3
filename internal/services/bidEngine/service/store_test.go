package bidEngine

import (
	"os"
	"path/filepath"
	"testing"
)

func writeMap(t *testing.T, body string) string {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "site_id_dsp_percents.json")
	if err := os.WriteFile(filename, []byte(body), 0644); err != nil {
		t.Fatalf("write map: %v", err)
	}
	return filename
}

func TestLookupSupportsCommaKeysAndALL(t *testing.T) {
	store, err := NewStore(writeMap(t, `{
		"site-a, site-b": {
			"dsp-one, dsp-two": 0.25,
			"ALL": 0.30
		},
		"ALL": {
			"dsp-fallback": 0.40
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		site, dsp string
		want      float32
		matched   bool
	}{
		{"site-a", "dsp-one", 0.25, true},
		{"site-b", "dsp-two", 0.25, true},
		{"site-a", "other", 0.30, true},
		{"other", "dsp-fallback", 0.40, true},
		{"other", "other", 0, false},
	}
	for _, tt := range tests {
		got, matched := store.Lookup(tt.site, tt.dsp)
		if got != tt.want || matched != tt.matched {
			t.Fatalf("Lookup(%q,%q)=(%v,%v), want (%v,%v)", tt.site, tt.dsp, got, matched, tt.want, tt.matched)
		}
	}
}

func TestValidationRejectsOutOfRangeAndDuplicateRules(t *testing.T) {
	if _, err := NewStore(writeMap(t, `{"site":{"dsp":1.01}}`)); err == nil {
		t.Fatal("percent above 1 must be rejected")
	}
	if _, err := NewStore(writeMap(t, `{"site-a,site-b":{"dsp":0.2},"site-a":{"dsp":0.3}}`)); err == nil {
		t.Fatal("duplicate expanded pair must be rejected")
	}
}

func TestEmptyFileAndMissingFileMeanNoOverride(t *testing.T) {
	empty := writeMap(t, "")
	store, err := NewStore(empty)
	if err != nil {
		t.Fatal(err)
	}
	if _, matched := store.Lookup("site", "dsp"); matched {
		t.Fatal("empty file must not create an override")
	}

	missing := filepath.Join(t.TempDir(), "missing.json")
	store, err = NewStore(missing)
	if err != nil {
		t.Fatal(err)
	}
	if _, matched := store.Lookup("site", "dsp"); matched {
		t.Fatal("missing file must not create an override")
	}
}

func TestUpdatePersistsAndPublishesExpandedSnapshot(t *testing.T) {
	filename := writeMap(t, `{}`)
	store, err := NewStore(filename)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update(Map{"a,b": {"x,y": 0.35}}); err != nil {
		t.Fatal(err)
	}
	if got, ok := store.Lookup("b", "y"); !ok || got != 0.35 {
		t.Fatalf("runtime lookup=(%v,%v), want (0.35,true)", got, ok)
	}
	raw, err := store.ReadRaw()
	if err != nil {
		t.Fatal(err)
	}
	if got := raw["a,b"]["x,y"]; got != 0.35 {
		t.Fatalf("raw persisted percent=%v want 0.35", got)
	}
}
