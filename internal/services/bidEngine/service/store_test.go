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
			"US, PHL": {"dsp-one, dsp-two": 0.25, "ALL": 0.30},
			"ALL": {"dsp-one": 0.35, "ALL": 0.40}
		},
		"ALL": {
			"US": {"dsp-one": 0.45, "ALL": 0.50},
			"ALL": {"dsp-one": 0.55, "ALL": 0.60}
		},
		"site-zero": {"US": {"dsp-one": 0}},
		"site-empty-geo": {"": {"dsp-one": 0.10}}
	}`))
	if err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		site, geo, dsp string
		want           float32
	}{
		{"site-a", "US", "dsp-one", 0.25},
		{"site-b", "PHL", "dsp-two", 0.25},
		{"site-a", "US", "other", 0.30},
		{"site-a", "other", "dsp-one", 0.35},
		{"site-a", "other", "other", 0.40},
		{"other", "US", "dsp-one", 0.45},
		{"other", "US", "other", 0.50},
		{"other", "other", "dsp-one", 0.55},
		{"other", "other", "other", 0.60},
		{"site-zero", "US", "dsp-one", 0},
		{"site-a", "", "dsp-one", 0.35},
		{"site-empty-geo", "", "dsp-one", 0.10},
	} {
		got, matched := store.Lookup(tt.site, tt.geo, tt.dsp)
		if got != tt.want || !matched {
			t.Errorf("Lookup(%q,%q,%q)=(%v,%v), want (%v,true)", tt.site, tt.geo, tt.dsp, got, matched, tt.want)
		}
	}
}

func TestValidationRejectsOutOfRangeAndDuplicateRules(t *testing.T) {
	for _, tt := range []struct{ name, body string }{
		{"percent above 1", `{"site":{"US":{"dsp":1.01}}}`},
		{"duplicate expanded site", `{"site-a,site-b":{"US":{"dsp":0.2}},"site-a":{"US":{"dsp":0.3}}}`},
		{"duplicate expanded GEO", `{"site":{"US,PHL":{"dsp":0.2},"PHL":{"dsp":0.3}}}`},
		{"duplicate expanded DSP", `{"site":{"US":{"a,b":0.2,"b":0.3}}}`},
		{"null GEO object", `{"site":null}`},
		{"null DSP object", `{"site":{"US":null}}`},
		{"old two-level shape", `{"site":{"dsp":0.2}}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewStore(writeMap(t, tt.body)); err == nil {
				t.Fatal("invalid rule accepted")
			}
		})
	}
}

func TestEmptyFileAndMissingFileMeanNoOverride(t *testing.T) {
	empty := writeMap(t, "")
	store, err := NewStore(empty)
	if err != nil {
		t.Fatal(err)
	}
	if _, matched := store.Lookup("site", "US", "dsp"); matched {
		t.Fatal("empty file must not create an override")
	}

	missing := filepath.Join(t.TempDir(), "missing.json")
	store, err = NewStore(missing)
	if err != nil {
		t.Fatal(err)
	}
	if _, matched := store.Lookup("site", "US", "dsp"); matched {
		t.Fatal("missing file must not create an override")
	}
}

func TestUpdatePersistsAndPublishesExpandedSnapshot(t *testing.T) {
	filename := writeMap(t, `{}`)
	store, err := NewStore(filename)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update(Map{"a,b": {"US,PHL": {"x,y": 0.35}}}); err != nil {
		t.Fatal(err)
	}
	if got, ok := store.Lookup("b", "PHL", "y"); !ok || got != 0.35 {
		t.Fatalf("runtime lookup=(%v,%v), want (0.35,true)", got, ok)
	}
	raw, err := store.ReadRaw()
	if err != nil {
		t.Fatal(err)
	}
	if got := raw["a,b"]["US,PHL"]["x,y"]; got != 0.35 {
		t.Fatalf("raw persisted percent=%v want 0.35", got)
	}
}

func TestSnapshotCannotChangeRuntimeRule(t *testing.T) {
	store, err := NewStore(writeMap(t, `{"site":{"US":{"dsp":0.25}}}`))
	if err != nil {
		t.Fatal(err)
	}
	snapshot := store.Snapshot()
	snapshot["site"]["US"]["dsp"] = 0.5
	if got, ok := store.Lookup("site", "US", "dsp"); !ok || got != 0.25 {
		t.Fatalf("mutated snapshot affected lookup: (%v,%v)", got, ok)
	}
}
