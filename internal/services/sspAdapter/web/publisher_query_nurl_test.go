package sppAdapterWeb

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ggicci/httpin"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/outbox"
)

func TestADMQueryNURLSurvivesParserAndOutboxRestart(t *testing.T) {
	ctx := context.Background()
	dsp, stats := callbackRedis(t, 10), callbackRedis(t, 12)
	id := uuid.NewString()
	clicks, imps := "query-click:"+id, "query-imp:"+id
	defer dsp.Del(ctx, id)
	defer clearStats(ctx, stats, clicks, imps)
	if e := utils.WriteUUIDKeyToRedis(ctx, dsp, id, time.Hour); e != nil {
		t.Fatal(e)
	}
	notification := "https://dsp.test/win?x=one%2Ftwo&percent=%25&equals=a=b#fragment?keep&tail="
	path := filepath.Join(t.TempDir(), "outbox.db")
	box, e := outbox.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	captured := make(chan publisherBURLInput, 1)
	queue := newPublisherBURLQueue(ctx, box, dsp, nil, 1, 100, time.Second, 24*time.Hour, func(c context.Context, r publisherBURLInput) error { captured <- r; <-c.Done(); return c.Err() })
	mux := chi.NewRouter()
	mux.With(httpin.NewInput(admRequest{})).Get("/adm", func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(context.WithValue(r.Context(), publisherBurlKey{}, publisherBurlFunc(queue.enqueue)))
		getAdm(ctx, w, r, []*redis.Client{stats}, dsp, clicks, nil, "", nil, box, nil)
	})
	q := url.Values{"id": {id}, "f": {"0"}, "p": {"1"}, "url": {"https://advertiser.test/landing?click_id=original"}, "n": {notification}}
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest("GET", "/adm?"+q.Encode(), nil))
	if recorder.Code != 200 || !recorder.Flushed || !strings.Contains(recorder.Body.String(), "advertiser.test/landing") {
		t.Fatal("redirect lost", recorder.Code)
	}
	select {
	case input := <-captured:
		if input.Record.BURL != notification {
			t.Fatal("query decoded twice", input.Record.BURL)
		}
	case <-time.After(time.Second):
		t.Fatal("internal BURL not started")
	}
	queue.Close()
	if n, e := box.TaskCount(); e != nil || n != 1 {
		t.Fatal("unfinished task not durable", n, e)
	}
	if e = box.Close(); e != nil {
		t.Fatal(e)
	}
	box, e = outbox.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer box.Close()
	tasks, e := box.ClaimTasks(time.Now().Add(5*time.Minute), 1, time.Minute)
	if e != nil || len(tasks) != 1 {
		t.Fatal(tasks, e)
	}
	var payload publisherBURLPayload
	if e = json.Unmarshal(tasks[0].Payload, &payload); e != nil || payload.Record.BURL != notification {
		t.Fatal("NURL lost from durable payload", payload, e)
	}
	if e = box.FinishTask(tasks[0], time.Now().Add(-time.Second)); e != nil {
		t.Fatal(e)
	}
	notified := make(chan string, 1)
	client := &http.Client{Transport: callbackTransport(func(r *http.Request) (*http.Response, error) {
		notified <- r.URL.String()
		return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})}
	executor := newPublisherDSPExecutor(ctx, client, 1, 8)
	defer executor.Close()
	restarted := newPublisherBURLQueue(ctx, box, dsp, nil, 1, 100, time.Second, 24*time.Hour, func(c context.Context, r publisherBURLInput) error {
		return processBurl(c, r, client, []*redis.Client{stats}, imps, nil, "", nil, box, nil, executor)
	})
	defer restarted.Close()
	waitPublisher(t, func() bool { n, _ := box.TaskCount(); return n == 0 && dsp.HGet(ctx, id, "pub_burl_done").Val() == "1" })
	select {
	case got := <-notified:
		if got != notification {
			t.Fatal("external NURL damaged", got)
		}
	case <-time.After(time.Second):
		t.Fatal("restored task did not notify DSP")
	}
	if stats.SCard(ctx, imps).Val() != 1 || stats.SCard(ctx, clicks).Val() != 1 {
		t.Fatal("missing/duplicate effects")
	}
	if dsp.HExists(ctx, id, "pub_dsp_nurl").Val() {
		t.Fatal("NURL stored in Redis")
	}
}
