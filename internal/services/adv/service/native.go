package auction

import (
	"bytes"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

type nativeRequestEnvelope struct {
	Native json.RawMessage `json:"native"`
}

type nativeRequest struct {
	Ver    string               `json:"ver"`
	Assets []nativeRequestAsset `json:"assets"`
}

type nativeRequestAsset struct {
	ID       json.RawMessage     `json:"id"`
	Required int                 `json:"required"`
	Title    *nativeRequestTitle `json:"title,omitempty"`
	Data     *nativeRequestData  `json:"data,omitempty"`
	Img      *nativeRequestImage `json:"img,omitempty"`
}

type nativeRequestTitle struct {
	Len int `json:"len"`
}

type nativeRequestData struct {
	Type int `json:"type"`
	Len  int `json:"len"`
}

type nativeRequestImage struct {
	Type int `json:"type"`
	W    int `json:"w"`
	H    int `json:"h"`
	WMin int `json:"wmin"`
	HMin int `json:"hmin"`
}

type nativeResponseEnvelope struct {
	Native nativeResponse `json:"native"`
}

type nativeResponse struct {
	Ver    string                `json:"ver"`
	Link   nativeResponseLink    `json:"link"`
	Assets []nativeResponseAsset `json:"assets"`
}

type nativeResponseLink struct {
	URL string `json:"url"`
}

type nativeResponseAsset struct {
	ID    json.RawMessage      `json:"id"`
	Title *nativeResponseTitle `json:"title,omitempty"`
	Data  *nativeResponseData  `json:"data,omitempty"`
	Img   *nativeResponseImage `json:"img,omitempty"`
}

type nativeResponseTitle struct {
	Text string `json:"text"`
}

type nativeResponseData struct {
	Value string `json:"value"`
}

type nativeResponseImage struct {
	URL string `json:"url"`
	W   int    `json:"w"`
	H   int    `json:"h"`
}

func parseNativeRequest(imp *ortb.Imp) (nativeRequest, bool) {
	if imp == nil || imp.GetNative() == nil {
		return nativeRequest{}, false
	}
	raw := strings.TrimSpace(imp.GetNative().GetRequest())
	if raw == "" {
		return nativeRequest{}, false
	}

	var envelope nativeRequestEnvelope
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return nativeRequest{}, false
	}

	payload := []byte(raw)
	if len(bytes.TrimSpace(envelope.Native)) > 0 && !bytes.Equal(bytes.TrimSpace(envelope.Native), []byte("null")) {
		payload = envelope.Native
	}

	var request nativeRequest
	if err := json.Unmarshal(payload, &request); err != nil || len(request.Assets) == 0 {
		return nativeRequest{}, false
	}
	for _, asset := range request.Assets {
		if !validNativeAssetID(asset.ID) {
			return nativeRequest{}, false
		}
	}
	return request, true
}

func buildNativeADM(imp *ortb.Imp, campaign *Campaign, creative *Creative, clickURL string) (string, bool) {
	if campaign == nil || creative == nil || strings.TrimSpace(clickURL) == "" {
		return "", false
	}
	format := normalizeFormat(campaign.Format)
	if format != constants.NAT && format != constants.IPP {
		return "", false
	}
	request, ok := parseNativeRequest(imp)
	if !ok {
		return "", false
	}

	response := nativeResponse{
		Ver:    strings.TrimSpace(request.Ver),
		Link:   nativeResponseLink{URL: clickURL},
		Assets: make([]nativeResponseAsset, 0, len(request.Assets)),
	}
	if response.Ver == "" && imp.GetNative() != nil {
		response.Ver = strings.TrimSpace(imp.GetNative().GetVer())
	}
	if response.Ver == "" {
		response.Ver = "1.2"
	}

	for _, requested := range request.Assets {
		asset, include, valid := buildNativeAsset(requested, campaign, creative)
		if !valid {
			return "", false
		}
		if include {
			response.Assets = append(response.Assets, asset)
		}
	}

	encoded, err := json.Marshal(nativeResponseEnvelope{Native: response})
	if err != nil {
		return "", false
	}
	return string(encoded), true
}

func buildNativeAsset(requested nativeRequestAsset, campaign *Campaign, creative *Creative) (nativeResponseAsset, bool, bool) {
	response := nativeResponseAsset{ID: append(json.RawMessage(nil), requested.ID...)}
	required := requested.Required == 1

	switch {
	case requested.Title != nil:
		value := strings.TrimSpace(creative.Title)
		if value == "" {
			return response, false, !required
		}
		response.Title = &nativeResponseTitle{Text: truncateNativeText(value, requested.Title.Len)}
		return response, true, true

	case requested.Data != nil:
		value := ""
		supported := true
		switch requested.Data.Type {
		case 1:
			value = strings.TrimSpace(campaign.BrandName)
		case 2:
			value = strings.TrimSpace(creative.Description)
		default:
			supported = false
		}
		if (!supported || value == "") && required {
			return response, false, false
		}
		response.Data = &nativeResponseData{Value: truncateNativeText(value, requested.Data.Len)}
		return response, true, true

	case requested.Img != nil:
		if !nativeImageEligible(creative, requested.Img) {
			return response, false, !required
		}
		response.Img = &nativeResponseImage{
			URL: strings.TrimSpace(creative.ImageURL),
			W:   creative.W,
			H:   creative.H,
		}
		return response, true, true

	default:
		return response, false, !required
	}
}

func nativeImageEligible(creative *Creative, request *nativeRequestImage) bool {
	if creative == nil || request == nil {
		return false
	}
	if strings.TrimSpace(creative.ImageURL) == "" || creative.W <= 0 || creative.H <= 0 {
		return false
	}
	if request.WMin > 0 && creative.W < request.WMin {
		return false
	}
	if request.HMin > 0 && creative.H < request.HMin {
		return false
	}
	return allowedNativeImageFormat(creative.FileFormat)
}

func allowedNativeImageFormat(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "image/jpg", "image/jpeg", "image/png", "image/gif", "jpg", "jpeg", "png", "gif":
		return true
	default:
		return false
	}
}

func truncateNativeText(value string, maxRunes int) string {
	if maxRunes <= 0 || utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxRunes])
}

func validNativeAssetID(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return false
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return false
	}
	switch value.(type) {
	case float64, string:
		return true
	default:
		return false
	}
}
