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

func AttachInternalMetadata(response *ortb.BidResponse, impID, segmentHash string, pointVersion uint64) {
	AttachInternalMetadataWithExact(response, impID, "", segmentHash, pointVersion)
}

func AttachInternalMetadataWithExact(response *ortb.BidResponse, impID, exactSegmentHash, segmentHash string, pointVersion uint64) {
	if response == nil || strings.TrimSpace(impID) == "" || strings.TrimSpace(segmentHash) == "" {
		return
	}
	if response.Ext == nil {
		response.Ext = &ortb.Ext{Values: make(map[string]string)}
	}
	if response.Ext.Values == nil {
		response.Ext.Values = make(map[string]string)
	}
	if exactSegmentHash = strings.TrimSpace(exactSegmentHash); exactSegmentHash != "" {
		response.Ext.Values[internalExactSegmentPrefix+impID] = exactSegmentHash
	}
	response.Ext.Values[internalSegmentPrefix+impID] = segmentHash
	response.Ext.Values[internalPointPrefix+impID] = strconv.FormatUint(pointVersion, 10)
}

func InternalMetadata(response *ortb.BidResponse, impID string) (string, uint64) {
	_, segmentHash, pointVersion := InternalMetadataWithExact(response, impID)
	return segmentHash, pointVersion
}

func InternalMetadataWithExact(response *ortb.BidResponse, impID string) (string, string, uint64) {
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
	return exactSegmentHash, segmentHash, pointVersion
}

func MergeInternalMetadata(destination, source *ortb.BidResponse) *ortb.BidResponse {
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
		if isInternalMetadataKey(key) {
			destination.Ext.Values[key] = value
		}
	}
	return destination
}

func StripInternalMetadata(response *ortb.BidResponse) {
	if response == nil || response.Ext == nil || len(response.Ext.Values) == 0 {
		return
	}
	for key := range response.Ext.Values {
		if isInternalMetadataKey(key) {
			delete(response.Ext.Values, key)
		}
	}
	if len(response.Ext.Values) == 0 {
		response.Ext = nil
	}
}

func isInternalMetadataKey(key string) bool {
	return strings.HasPrefix(key, internalExactSegmentPrefix) || strings.HasPrefix(key, internalSegmentPrefix) || strings.HasPrefix(key, internalPointPrefix)
}
