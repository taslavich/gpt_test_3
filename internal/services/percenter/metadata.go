package percenter

import (
	"strconv"
	"strings"

	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

const (
	internalSegmentPrefix = "__twinbid_percenter_segment:"
	internalPointPrefix   = "__twinbid_percenter_point:"
)

func AttachSimpleMetadata(response *ortb.BidResponse, impID, segmentHash string, pointVersion uint64) {
	if response == nil || strings.TrimSpace(impID) == "" || strings.TrimSpace(segmentHash) == "" || pointVersion == 0 {
		return
	}
	if response.Ext == nil {
		response.Ext = &ortb.Ext{Values: make(map[string]string)}
	}
	if response.Ext.Values == nil {
		response.Ext.Values = make(map[string]string)
	}
	response.Ext.Values[internalSegmentPrefix+impID] = strings.TrimSpace(segmentHash)
	response.Ext.Values[internalPointPrefix+impID] = strconv.FormatUint(pointVersion, 10)
}

func SimpleMetadata(response *ortb.BidResponse, impID string) (string, uint64) {
	if response == nil || response.GetExt() == nil || strings.TrimSpace(impID) == "" {
		return "", 0
	}
	values := response.GetExt().GetValues()
	segmentHash := strings.TrimSpace(values[internalSegmentPrefix+impID])
	if segmentHash == "" {
		return "", 0
	}
	pointVersion, err := strconv.ParseUint(strings.TrimSpace(values[internalPointPrefix+impID]), 10, 64)
	if err != nil || pointVersion == 0 {
		return "", 0
	}
	return segmentHash, pointVersion
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
		if strings.HasPrefix(key, internalSegmentPrefix) || strings.HasPrefix(key, internalPointPrefix) {
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
		if strings.HasPrefix(key, internalSegmentPrefix) || strings.HasPrefix(key, internalPointPrefix) {
			delete(response.Ext.Values, key)
		}
	}
	if len(response.Ext.Values) == 0 {
		response.Ext = nil
	}
}
