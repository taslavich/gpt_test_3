package percenter

import (
	"testing"

	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

func TestInternalMetadataRoundTripAndStrip(t *testing.T) {
	response := &ortb.BidResponse{}
	AttachInternalMetadata(response, "imp-1", "hash-1", 7)
	response.Ext.Values["public"] = "keep"

	hash, point := InternalMetadata(response, "imp-1")
	if hash != "hash-1" || point != 7 {
		t.Fatalf("metadata=(%q,%d), want (hash-1,7)", hash, point)
	}

	StripInternalMetadata(response)
	if got := response.GetExt().GetValues()["public"]; got != "keep" {
		t.Fatalf("public ext was removed: %q", got)
	}
	if hash, point := InternalMetadata(response, "imp-1"); hash != "" || point != 0 {
		t.Fatalf("internal metadata leaked after strip: (%q,%d)", hash, point)
	}
}

func TestMergeInternalMetadataPreservesDestination(t *testing.T) {
	source := &ortb.BidResponse{}
	AttachInternalMetadata(source, "imp-1", "hash-1", 3)
	destination := &ortb.BidResponse{Ext: &ortb.Ext{Values: map[string]string{"public": "x"}}}

	merged := MergeInternalMetadata(destination, source)
	if merged != destination {
		t.Fatal("merge replaced destination response")
	}
	if got := merged.GetExt().GetValues()["public"]; got != "x" {
		t.Fatalf("destination ext changed: %q", got)
	}
	hash, point := InternalMetadata(merged, "imp-1")
	if hash != "hash-1" || point != 3 {
		t.Fatalf("merged metadata=(%q,%d), want (hash-1,3)", hash, point)
	}
}

func TestInternalMetadataWithExact(t *testing.T) {
	response := &ortb.BidResponse{}
	AttachInternalMetadataWithExact(response, "imp-exact", "exact-hash", "effective-hash", 11)
	exactHash, effectiveHash, pointVersion := InternalMetadataWithExact(response, "imp-exact")
	if exactHash != "exact-hash" || effectiveHash != "effective-hash" || pointVersion != 11 {
		t.Fatalf("metadata exact=%q effective=%q point=%d", exactHash, effectiveHash, pointVersion)
	}
	StripInternalMetadata(response)
	if response.GetExt() != nil {
		t.Fatalf("internal metadata was not fully stripped: %+v", response.GetExt())
	}
}

func TestInternalMetadataPreservesKnownSegmentWithZeroPointVersion(t *testing.T) {
	response := &ortb.BidResponse{}
	AttachInternalMetadataWithExact(response, "imp-fallback", "exact-fallback", "effective-fallback", 0)

	exactHash, effectiveHash, pointVersion := InternalMetadataWithExact(response, "imp-fallback")
	if exactHash != "exact-fallback" || effectiveHash != "effective-fallback" || pointVersion != 0 {
		t.Fatalf("fallback metadata exact=%q effective=%q point=%d", exactHash, effectiveHash, pointVersion)
	}
	if got := response.GetExt().GetValues()[internalPointPrefix+"imp-fallback"]; got != "0" {
		t.Fatalf("raw point version=%q, want 0", got)
	}
}
