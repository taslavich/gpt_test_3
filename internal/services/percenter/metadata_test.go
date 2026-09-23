package percenter

import (
	"testing"

	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

func TestSimpleMetadataRoundTripMergeAndStrip(t *testing.T) {
	source := &ortb.BidResponse{}
	AttachPercenterMetadata(source, "imp-1", "request-exact", "segment-1", 7)
	exact, segment, point := PercenterMetadata(source, "imp-1")
	if exact != "request-exact" || segment != "segment-1" || point != 7 {
		t.Fatalf("metadata=(%q,%q,%d)", exact, segment, point)
	}

	destination := &ortb.BidResponse{}
	MergeSimpleMetadata(destination, source)
	exact, segment, point = PercenterMetadata(destination, "imp-1")
	if exact != "request-exact" || segment != "segment-1" || point != 7 {
		t.Fatalf("merged metadata=(%q,%q,%d)", exact, segment, point)
	}

	StripSimpleMetadata(destination)
	if exact, segment, point := PercenterMetadata(destination, "imp-1"); exact != "" || segment != "" || point != 0 {
		t.Fatalf("metadata leaked after strip=(%q,%q,%d)", exact, segment, point)
	}
}

func TestAttachSimpleMetadataKeepsBackwardCompatibleExactAttribution(t *testing.T) {
	response := &ortb.BidResponse{}
	AttachSimpleMetadata(response, "imp", "segment", 5)
	exact, segment, point := PercenterMetadata(response, "imp")
	if exact != "segment" || segment != "segment" || point != 5 {
		t.Fatalf("metadata=(%q,%q,%d)", exact, segment, point)
	}
}
