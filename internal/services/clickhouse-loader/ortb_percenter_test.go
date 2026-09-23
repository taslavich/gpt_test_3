package clickhouse_loader

import (
	"testing"

	"github.com/google/uuid"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
)

func TestSplitOrtbBidResponsesExtractsPercenterTransportMetadata(t *testing.T) {
	input := map[string]string{
		"dsp-a": "200",
		constants.ADVRTBResponseStatsPrefix + "123":     "204",
		constants.PercenterExactSegmentHashTransportKey: " request-exact ",
		constants.PercenterSegmentHashTransportKey:      " segment-hash ",
		constants.PercenterPointVersionTransportKey:     "42",
	}
	normal, rtb, exact, segment, point := splitOrtbBidResponses(input)
	if exact != "request-exact" || segment != "segment-hash" || point != 42 {
		t.Fatalf("metadata=(%q,%q,%d)", exact, segment, point)
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
	if _, leaked := normal[constants.PercenterExactSegmentHashTransportKey]; leaked {
		t.Fatal("internal exact segment metadata must not be encoded into normal bid responses")
	}
}

func TestLogicalOrtbEventIDMatchesExistingCanonicalUUIDDefault(t *testing.T) {
	raw := "AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA"
	parsed, err := uuid.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := logicalOrtbEventID(raw, parsed), parsed.String(); got != want {
		t.Fatalf("logical event id=%q want canonical %q", got, want)
	}
	if got := logicalOrtbEventID("not-a-uuid", uuid.Nil); got != "not-a-uuid" {
		t.Fatalf("malformed raw ids must stay distinct instead of collapsing to uuid.Nil, got %q", got)
	}
}

func TestLogicalOrtbEventIDCollapsesKafkaReplay(t *testing.T) {
	ids := []string{
		"11111111-1111-1111-1111-111111111111",
		"22222222-2222-2222-2222-222222222222",
		"33333333-3333-3333-3333-333333333333",
		"44444444-4444-4444-4444-444444444444",
		"33333333-3333-3333-3333-333333333333", // replay of one real impression/request
	}
	logical := make(map[string]struct{})
	for _, raw := range ids {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		logical[logicalOrtbEventID(raw, parsed)] = struct{}{}
	}
	if len(logical) != 4 {
		t.Fatalf("4 logical ORTB events + one replay must remain 4, got %d", len(logical))
	}
}
