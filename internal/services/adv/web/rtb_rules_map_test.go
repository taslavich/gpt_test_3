package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/adv/rtbrules"
)

func TestRTBRulesMapHTTP(t *testing.T) {
	const a = "8cf66405-6013-423c-a967-a76c08c69ac4"
	const b = "9e4173f7-7aee-44a6-8039-db364cb6f4f5"
	initial := []byte(`{"` + a + `,` + b + `":{"add":[{"field":"imp.ext.type","value":"pop"}]}}`)
	path := filepath.Join(t.TempDir(), "rtb_rules.json")
	if err := os.WriteFile(path, initial, 0600); err != nil {
		t.Fatal(err)
	}
	store, err := rtbrules.NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	InitHttpRoutes(router, nil, nil, nil, NewWorkController(), AntiPerekrutHTTPConfig{RTBRules: store})
	get := func(route string) map[string]json.RawMessage {
		t.Helper()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, route, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s: status %d, body %s", route, w.Code, w.Body.String())
		}
		var data map[string]json.RawMessage
		if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
			t.Fatal(err)
		}
		return data
	}
	saved := get(GetRTBRulesMapURL)
	if len(saved) != 1 {
		t.Fatalf("saved map must preserve grouped key: %+v", saved)
	}
	memory := get(GetDebugRTBRulesMapURL)
	if len(memory) != 2 {
		t.Fatalf("runtime map must expand grouped keys: %+v", memory)
	}
	invalid := []byte(`{"` + a + `":{"delete":[{"field":"nonexistent.field"}]}}`)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPut, PutRTBRulesMapURL, bytes.NewReader(invalid)))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad PUT status=%d %s", w.Code, w.Body.String())
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, initial) {
		t.Fatal("invalid PUT altered persisted file")
	}
	if len(get(GetDebugRTBRulesMapURL)) != 2 {
		t.Fatal("invalid PUT altered runtime")
	}
	valid := []byte(`{"` + a + `":{"delete":[{"field":"imp.banner","formats":["POP"]}]}}`)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPut, PutRTBRulesMapURL, bytes.NewReader(valid)))
	if w.Code != http.StatusNoContent {
		t.Fatalf("valid PUT: %d %s", w.Code, w.Body.String())
	}
	if len(get(GetDebugRTBRulesMapURL)) != 1 || len(get(GetRTBRulesMapURL)) != 1 {
		t.Fatal("PUT did not update both")
	}
}
