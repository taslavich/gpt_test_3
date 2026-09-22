package percenter

import (
	"testing"

	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

func TestSimpleMetadataRoundTripMergeAndStrip(t *testing.T) {
	source := &ortb.BidResponse{}
	AttachSimpleMetadata(source, "imp-1", "segment-1", 7)
	segment, point := SimpleMetadata(source, "imp-1")
	if segment != "segment-1" || point != 7 {
		t.Fatalf("metadata=(%q,%d)", segment, point)
	}

	destination := &ortb.BidResponse{}
	MergeSimpleMetadata(destination, source)
	segment, point = SimpleMetadata(destination, "imp-1")
	if segment != "segment-1" || point != 7 {
		t.Fatalf("merged metadata=(%q,%d)", segment, point)
	}

	StripSimpleMetadata(destination)
	if segment, point := SimpleMetadata(destination, "imp-1"); segment != "" || point != 0 {
		t.Fatalf("metadata leaked after strip=(%q,%d)", segment, point)
	}
}
