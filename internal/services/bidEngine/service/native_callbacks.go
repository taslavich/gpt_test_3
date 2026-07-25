package bidEngine

import (
	"encoding/json"
	"strings"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	"google.golang.org/protobuf/proto"
)

func isADVNativeFormat(format string) bool {
	switch strings.ToUpper(strings.TrimSpace(format)) {
	case constants.NAT, constants.IPP:
		return true
	default:
		return false
	}
}

func finalizeADVNativeCallbacks(source *ortb.Bid, admDomain, globalID, format string) (*ortb.Bid, bool) {
	if source == nil || strings.TrimSpace(source.GetAdm()) == "" {
		return nil, false
	}
	finalBid, ok := proto.Clone(source).(*ortb.Bid)
	if !ok || finalBid == nil {
		return nil, false
	}
	finalBid.Nurl = nil
	finalBid.Burl = nil

	adm, ok := wrapNativeLinkURL(source.GetAdm(), admDomain, globalID, format)
	if !ok {
		return nil, false
	}
	finalBid.Adm = &adm

	burl := utils.WrapBurlURL(admDomain, globalID, format)
	if strings.TrimSpace(burl) == "" {
		return nil, false
	}
	finalBid.Burl = &burl
	return finalBid, true
}

func wrapNativeLinkURL(adm, admDomain, globalID, format string) (string, bool) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal([]byte(adm), &envelope); err != nil {
		return "", false
	}
	nativeRaw, ok := envelope["native"]
	if !ok {
		return "", false
	}

	var nativeObject map[string]json.RawMessage
	if err := json.Unmarshal(nativeRaw, &nativeObject); err != nil {
		return "", false
	}
	linkRaw, ok := nativeObject["link"]
	if !ok {
		return "", false
	}

	var linkObject map[string]json.RawMessage
	if err := json.Unmarshal(linkRaw, &linkObject); err != nil {
		return "", false
	}
	var destination string
	if err := json.Unmarshal(linkObject["url"], &destination); err != nil || strings.TrimSpace(destination) == "" {
		return "", false
	}

	wrapped := utils.WrapURL(admDomain, destination, globalID, format)
	if strings.TrimSpace(wrapped) == "" {
		return "", false
	}
	wrappedRaw, err := json.Marshal(wrapped)
	if err != nil {
		return "", false
	}
	linkObject["url"] = wrappedRaw
	linkRaw, err = json.Marshal(linkObject)
	if err != nil {
		return "", false
	}
	nativeObject["link"] = linkRaw
	nativeRaw, err = json.Marshal(nativeObject)
	if err != nil {
		return "", false
	}
	envelope["native"] = nativeRaw
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return "", false
	}
	return string(encoded), true
}
