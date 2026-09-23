package percenter

import (
	"strconv"
	"strings"

	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

const (
	internalExactSegmentPrefix = "__twinbid_percenter_exact_segment:"
	internalSegmentPrefix      = "__twinbid_percenter_segment:"
	internalPointPrefix        = "__twinbid_percenter_point:"
)

// AttachSimpleMetadata is kept for existing call sites/tests. When only the
// final segment is known it is also a valid exact attribution value.
func AttachSimpleMetadata(response *ortb.BidResponse, impID, segmentHash string, pointVersion uint64) {
	AttachPercenterMetadata(response, impID, segmentHash, segmentHash, pointVersion)
}

// AttachPercenterMetadata stores the immutable request attribution used by the
// actual bid. The pre-campaign exact hash is separate from the final
// campaign-aware segment hash.
func AttachPercenterMetadata(response *ortb.BidResponse, impID, exactSegmentHash, segmentHash string, pointVersion uint64) {
	if response == nil || strings.TrimSpace(impID) == "" || strings.TrimSpace(segmentHash) == "" || pointVersion == 0 {
		return
	}
	if response.Ext == nil {
		response.Ext = &ortb.Ext{Values: make(map[string]string)}
	}
	if response.Ext.Values == nil {
		response.Ext.Values = make(map[string]string)
	}
	if exact := strings.TrimSpace(exactSegmentHash); exact != "" {
		response.Ext.Values[internalExactSegmentPrefix+impID] = exact
	}
	response.Ext.Values[internalSegmentPrefix+impID] = strings.TrimSpace(segmentHash)
	response.Ext.Values[internalPointPrefix+impID] = strconv.FormatUint(pointVersion, 10)
}

func SimpleMetadata(response *ortb.BidResponse, impID string) (string, uint64) {
	_, segmentHash, pointVersion := PercenterMetadata(response, impID)
	return segmentHash, pointVersion
}

func PercenterMetadata(response *ortb.BidResponse, impID string) (string, string, uint64) {
	if response == nil || response.GetExt() == nil || strings.TrimSpace(impID) == "" {
		return "", "", 0
	}
	values := response.GetExt().GetValues()
	exactSegmentHash := strings.TrimSpace(values[internalExactSegmentPrefix+impID])
	segmentHash := strings.TrimSpace(values[internalSegmentPrefix+impID])
	if segmentHash == "" {
		return exactSegmentHash, "", 0
	}
	pointVersion, err := strconv.ParseUint(strings.TrimSpace(values[internalPointPrefix+impID]), 10, 64)
	if err != nil || pointVersion == 0 {
		return exactSegmentHash, segmentHash, 0
	}
	if exactSegmentHash == "" {
		exactSegmentHash = segmentHash
	}
	return exactSegmentHash, segmentHash, pointVersion
}

func MergeSimpleMetadata(destination, source *ortb.BidResponse) *ortb.BidResponse {
	if source == nil || source.GetExt() == nil || len(source.GetExt().GetValues()) == 0 {
		return destination
	}
	if destination == nil {
		destination = &ortb.BidResponse{}
	}
	if destination.Ext == nil {
		destination.Ext = &ortb.Ext{Values: make(map[string]string)}
	}
	if destination.Ext.Values == nil {
		destination.Ext.Values = make(map[string]string)
	}
	for key, value := range source.GetExt().GetValues() {
		if strings.HasPrefix(key, internalExactSegmentPrefix) || strings.HasPrefix(key, internalSegmentPrefix) || strings.HasPrefix(key, internalPointPrefix) {
			destination.Ext.Values[key] = value
		}
	}
	return destination
}

func StripSimpleMetadata(response *ortb.BidResponse) {
	if response == nil || response.Ext == nil {
		return
	}
	for key := range response.Ext.Values {
		if strings.HasPrefix(key, internalExactSegmentPrefix) || strings.HasPrefix(key, internalSegmentPrefix) || strings.HasPrefix(key, internalPointPrefix) {
			delete(response.Ext.Values, key)
		}
	}
	if len(response.Ext.Values) == 0 {
		response.Ext = nil
	}
}
