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
		if messages := snapshot(); len(messages) != 1 || !strings.Contains(messages[0], "delivery=2") || !strings.Contains(messages[0], "RTB errors: 1") {
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
		if messages := snapshot(); len(messages) != 2 || !strings.Contains(messages[1], "delivery=2") || !strings.Contains(messages[1], "RTB errors: 2") {
			t.Fatalf("retry must include both pending and new events: %q", messages)
		}

		time.Sleep(20 * time.Minute)
		synctest.Wait()
		if messages := snapshot(); len(messages) != 2 {
			t.Fatalf("empty windows should not send messages: %q", messages)
		}
	})
}
