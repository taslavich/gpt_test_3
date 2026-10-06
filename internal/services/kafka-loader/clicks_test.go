package kafka_loader

import (
	"testing"
	"time"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	eventspb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/buffer"
	"google.golang.org/protobuf/proto"
)

func TestBuildClickKafkaMessageCarriesIframeStatus(t *testing.T) {
	for _, tc := range []struct {
		name       string
		redisValue string
		want       int32
	}{
		{name: "unknown", redisValue: "", want: clickIframeStatusUnknown},
		{name: "no iframe", redisValue: constants.FALSE, want: clickIframeStatusNo},
		{name: "iframe", redisValue: constants.TRUE, want: clickIframeStatusYes},
	} {
		t.Run(tc.name, func(t *testing.T) {
			msg, shouldSend, err := buildClickKafkaMessage(0, "11111111-1111-1111-1111-111111111111", []interface{}{
				"22222222-2222-2222-2222-222222222222",
				"2026-10-06 01:02:03.004",
				"POP",
				tc.redisValue,
			})
			if err != nil {
				t.Fatal(err)
			}
			if !shouldSend {
				t.Fatal("click unexpectedly skipped")
			}

			var got eventspb.ClickEvent
			if err := proto.Unmarshal(msg.Value, &got); err != nil {
				t.Fatal(err)
			}
			if got.IframeStatus != tc.want {
				t.Fatalf("IframeStatus=%d want %d", got.IframeStatus, tc.want)
			}
		})
	}
}

func TestShouldDeferRecentClickUntilIframeReportHasChanceToArrive(t *testing.T) {
	recent := time.Now().UTC().Add(-250 * time.Millisecond).Format("2006-01-02 15:04:05.000")
	old := time.Now().UTC().Add(-clickIframeReportGrace - time.Second).Format("2006-01-02 15:04:05.000")

	if !shouldDeferClickForIframeReport(0, "click", []interface{}{"ortb", recent, "POP", nil}) {
		t.Fatal("recent click without iframe report must be deferred")
	}
	if shouldDeferClickForIframeReport(0, "click", []interface{}{"ortb", recent, "POP", constants.TRUE}) {
		t.Fatal("known iframe status must not be deferred")
	}
	if shouldDeferClickForIframeReport(0, "click", []interface{}{"ortb", old, "POP", nil}) {
		t.Fatal("old click without report must proceed as unknown")
	}
}
