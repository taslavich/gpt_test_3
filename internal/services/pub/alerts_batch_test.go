package pub

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

func TestPublisherSystemAlertsBatchEveryTenMinutes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		var mu sync.Mutex
		var messages []string
		alerts := NewSystemAlerts("test-pub", func(_ context.Context, text string) error {
			mu.Lock()
			defer mu.Unlock()
			messages = append(messages, text)
			if len(messages) == 1 {
				return errors.New("bot unavailable")
			}
			return nil
		})
		snapshot := func() []string {
			mu.Lock()
			defer mu.Unlock()
			return append([]string(nil), messages...)
		}
		go alerts.Run(ctx)
		synctest.Wait()

		alerts.Record(AlertDelivery)
		alerts.recordRTBFailure(errors.New("test RTB failure"))
		time.Sleep(9 * time.Minute)
		alerts.Record(AlertDelivery)
		synctest.Wait()
		if messages := snapshot(); len(messages) != 0 {
			t.Fatalf("alerts sent before the first ten-minute window: %q", messages)
		}
		time.Sleep(time.Minute)
		synctest.Wait()
		if messages := snapshot(); len(messages) != 1 || !strings.Contains(messages[0], "Ad delivery preparation failed: 2") || !strings.Contains(messages[0], "RTB: Unknown failure; see pub logs ×1") {
			t.Fatalf("the first send must batch all events: %q", messages)
		}

		alerts.recordRTBFailure(errors.New("test RTB failure"))
		time.Sleep(9 * time.Minute)
		synctest.Wait()
		if messages := snapshot(); len(messages) != 1 {
			t.Fatalf("retry sent before the next window: %q", messages)
		}
		time.Sleep(time.Minute)
		synctest.Wait()
		if messages := snapshot(); len(messages) != 2 || !strings.Contains(messages[1], "Ad delivery preparation failed: 2") || !strings.Contains(messages[1], "RTB: Unknown failure; see pub logs ×2") {
			t.Fatalf("retry must include both pending and new events: %q", messages)
		}

		time.Sleep(20 * time.Minute)
		synctest.Wait()
		if messages := snapshot(); len(messages) != 2 {
			t.Fatalf("empty windows should not send messages: %q", messages)
		}
	})
}

func TestPubRTBAlertShowsCauseAndCount(t *testing.T) {
	var message string
	a := NewSystemAlerts("NLDW4-3-08-36", func(_ context.Context, text string) error {
		message = text
		return nil
	})
	for i := 0; i < 4; i++ {
		a.recordRTBFailure(classifyRTBHTTPFailure(503, "", "backend error").
			withUpstream("twinbidexchange.com:8086/bid_v_2_5_mc?feed=secret"))
	}
	a.flush(t.Context())
	const want = "[pub] NLDW4-3-08-36\nRTB HTTP 503: Service unavailable ×4 (/bid_v_2_5_mc)"
	if message != want {
		t.Fatalf("digest:\n%s\nwant:\n%s", message, want)
	}
	a.recordRTBFailure(classifyRTBHTTPFailure(503, "", "").withUpstream("rtb:8086/bid_v_2_5"))
	a.recordRTBFailure(classifyRTBHTTPFailure(503, "", "").withUpstream("rtb:8086/bid_v_2_5_mc"))
	a.flush(t.Context())
	if message != "[pub] NLDW4-3-08-36\nRTB HTTP 503: Service unavailable ×2" {
		t.Fatalf("digest must not attribute a shared count to one endpoint: %q", message)
	}
}
