package kafka_loader

import (
	"testing"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	eventspb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/buffer"
	"google.golang.org/protobuf/proto"
)

func TestOrtbHMGetPercenterMetadataPositions(t *testing.T) {
	if got, want := len(ortbHMGetFields), 28; got != want {
		t.Fatalf("ortb HMGET field count=%d want=%d", got, want)
	}
	checks := map[int]string{
		25: constants.EXACT_SEGMENT_HASH_COLUMN,
		26: constants.SEGMENT_HASH_COLUMN,
		27: constants.PERCENTER_POINT_VERSION_COLUMN,
	}
	for index, want := range checks {
		if got := ortbHMGetFields[index]; got != want {
			t.Fatalf("ortb HMGET field[%d]=%q want=%q", index, got, want)
		}
	}
}

func TestBuildOrtbKafkaMessageCarriesExactAndEffectiveSegmentMetadata(t *testing.T) {
	values := make([]interface{}, 28)
	values[0] = "2026-09-05 12:00:00.000"
	values[1] = "banner"
	values[2] = "banner"
	values[3] = "ssp.example"
	values[4] = "US"
	values[6] = "200"
	values[22] = "campaign-1"
	values[25] = "exact-hash"
	values[26] = "effective-hash"
	values[27] = "42"

	message, ok, err := buildOrtbKafkaMessage(0, "uuid-1", values)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ORTB message unexpectedly considered empty")
	}
	var event eventspb.OrtbEvent
	if err := proto.Unmarshal(message.Value, &event); err != nil {
		t.Fatal(err)
	}
	if got := event.BidResponses[exactSegmentHashTransportKey]; got != "exact-hash" {
		t.Fatalf("exact hash=%q", got)
	}
	if got := event.BidResponses[segmentHashTransportKey]; got != "effective-hash" {
		t.Fatalf("effective hash=%q", got)
	}
	if got := event.BidResponses[pointVersionTransportKey]; got != "42" {
		t.Fatalf("point version=%q", got)
	}
}

func TestBuildOrtbKafkaMessageCarriesZeroPointVersionForKnownFallbackSegment(t *testing.T) {
	values := make([]interface{}, 28)
	values[0] = "2026-09-06 12:00:00.000"
	values[1] = "banner"
	values[2] = "banner"
	values[3] = "ssp.example"
	values[4] = "US"
	values[6] = "204"
	values[22] = "campaign-1"
	values[25] = "exact-fallback"
	values[26] = "effective-fallback"
	values[27] = "0"

	message, ok, err := buildOrtbKafkaMessage(0, "uuid-fallback", values)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ORTB fallback message unexpectedly considered empty")
	}
	var event eventspb.OrtbEvent
	if err := proto.Unmarshal(message.Value, &event); err != nil {
		t.Fatal(err)
	}
	if got := event.BidResponses[exactSegmentHashTransportKey]; got != "exact-fallback" {
		t.Fatalf("exact hash=%q", got)
	}
	if got := event.BidResponses[segmentHashTransportKey]; got != "effective-fallback" {
		t.Fatalf("effective hash=%q", got)
	}
	if got := event.BidResponses[pointVersionTransportKey]; got != "0" {
		t.Fatalf("point version=%q", got)
	}
}
