package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	auction "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/adv/service"
)

func TestPublisherPercentMapHTTPPersistenceAndRollback(t *testing.T) {
	const a = "11111111-1111-4111-8111-111111111111"
	const b = "22222222-2222-4222-8222-222222222222"
	path := filepath.Join(t.TempDir(), "publisher.json")
	store, err := auction.NewPublisherPercentStore(path)
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	InitHttpRoutes(router, nil, nil, nil, NewWorkController(), AntiPerekrutHTTPConfig{PublisherPercents: store})
	call := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}
	group := a + "," + b
	good := `{"` + group + `":0.1}`
	if w := call(http.MethodPut, PublisherPercentMapURL, good); w.Code != 204 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, item := range []struct {
		path    string
		entries int
	}{{PublisherPercentMapURL, 1}, {DebugPublisherPercentMapURL, 2}} {
		w := call(http.MethodGet, item.path, "")
		var result map[string]float64
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || len(result) != item.entries {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	before, _ := os.ReadFile(path)
	for _, bad := range []string{`{"ALL":0.2}`, `{"` + a + `":1.1}`, `{"` + a + `":null}`, `{"` + a + `":20}`, good + ` {}`, `null`, `[]`, `{"` + group + `":0.2,"` + a + `":0.3}`} {
		if w := call(http.MethodPut, PublisherPercentMapURL, bad); w.Code != 400 {
			t.Fatal("bad input accepted", bad, w.Code)
		}
		after, _ := os.ReadFile(path)
		if string(after) != string(before) {
			t.Fatal("rejected PUT modified saved map")
		}
		if p, ok := store.Lookup(a); !ok || p != .1 {
			t.Fatal("rejected PUT modified active map")
		}
	}
	if w := call(http.MethodPut, PublisherPercentMapURL, `{}`); w.Code != 204 {
		t.Fatal(w.Code)
	}
	if _, ok := store.Lookup(a); ok {
		t.Fatal("empty PUT did not remove override")
	}
	restarted, err := auction.NewPublisherPercentStore(path)
	if err != nil || len(restarted.Memory()) != 0 {
		t.Fatal("restart failed", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if w := call(http.MethodPut, PublisherPercentMapURL, good); w.Code != 500 {
		t.Fatal("write error is not HTTP 500", w.Code)
	}
	if len(store.Memory()) != 0 {
		t.Fatal("failed persistence changed active map")
	}
}
