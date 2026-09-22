package clickhouse_loader

import (
	"testing"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
)

func TestSplitOrtbBidResponsesExtractsPercenterTransportMetadata(t *testing.T) {
	input := map[string]string{
		"dsp-a": "200",
		constants.ADVRTBResponseStatsPrefix + "123": "204",
		constants.PercenterSegmentHashTransportKey:  " segment-hash ",
		constants.PercenterPointVersionTransportKey: "42",
	}
	normal, rtb, segment, point := splitOrtbBidResponses(input)
	if segment != "segment-hash" || point != 42 {
		t.Fatalf("metadata=(%q,%d)", segment, point)
	}
	if len(normal) != 1 || normal["dsp-a"] != "200" {
		t.Fatalf("normal responses=%#v", normal)
	}
	if len(rtb) != 1 || rtb["123"] != "204" {
		t.Fatalf("RTB responses=%#v", rtb)
	}
	if _, leaked := normal[constants.PercenterSegmentHashTransportKey]; leaked {
		t.Fatal("internal percenter metadata must not be encoded into normal bid responses")
	}
}
