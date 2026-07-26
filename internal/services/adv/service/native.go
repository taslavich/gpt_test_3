package auction

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	request, reason := parseNativeRequestDetailed(imp)
	return request, reason == ""
}

func parseNativeRequestDetailed(imp *ortb.Imp) (nativeRequest, string) {
	if imp == nil {
		return nativeRequest{}, "impression_nil"
	}
	if imp.GetNative() == nil {
		return nativeRequest{}, "native_object_missing"
	}
	raw := strings.TrimSpace(imp.GetNative().GetRequest())
	if raw == "" {
		return nativeRequest{}, "native_request_empty"
	}

	var envelope nativeRequestEnvelope
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return nativeRequest{}, "native_request_json_invalid"
	}

	payload := []byte(raw)
	if len(bytes.TrimSpace(envelope.Native)) > 0 && !bytes.Equal(bytes.TrimSpace(envelope.Native), []byte("null")) {
		payload = envelope.Native
	}

	var request nativeRequest
	if err := json.Unmarshal(payload, &request); err != nil {
		return nativeRequest{}, "native_payload_json_invalid"
	}
	if len(request.Assets) == 0 {
		return nativeRequest{}, "native_assets_empty"
	}
	for index, asset := range request.Assets {
		if !validNativeAssetID(asset.ID) {
			return nativeRequest{}, fmt.Sprintf("native_asset_id_invalid_at_%d", index)
		}
	}
	return request, ""
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

type nativeRejectionDiagnostic struct {
	Reason        string
	AssetIndex    int
	AssetID       string
	AssetKind     string
	Required      bool
	DataType      int
	RequestedW    int
	RequestedH    int
	RequestedWMin int
	RequestedHMin int
}

func diagnoseNativeCreativeRejection(imp *ortb.Imp, campaign *Campaign, creative *Creative) nativeRejectionDiagnostic {
	diagnostic := nativeRejectionDiagnostic{AssetIndex: -1}
	if campaign == nil {
		diagnostic.Reason = "campaign_nil"
		return diagnostic
	}
	if creative == nil {
		diagnostic.Reason = "creative_nil"
		return diagnostic
	}
	if strings.TrimSpace(creative.ID) == "" {
		diagnostic.Reason = "creative_id_empty"
		return diagnostic
	}
	if strings.TrimSpace(creative.ADMURL) == "" {
		diagnostic.Reason = "adm_url_empty"
		return diagnostic
	}
	format := normalizeFormat(campaign.Format)
	if format != constants.NAT && format != constants.IPP {
		diagnostic.Reason = "campaign_format_not_native_or_ipp"
		return diagnostic
	}

	request, parseReason := parseNativeRequestDetailed(imp)
	if parseReason != "" {
		diagnostic.Reason = parseReason
		return diagnostic
	}

	for index, requested := range request.Assets {
		_, _, valid := buildNativeAsset(requested, campaign, creative)
		if valid {
			continue
		}

		diagnostic.AssetIndex = index
		diagnostic.AssetID = nativeAssetIDText(requested.ID)
		diagnostic.Required = requested.Required == 1

		switch {
		case requested.Title != nil:
			diagnostic.AssetKind = "title"
			diagnostic.Reason = "required_title_missing"

		case requested.Data != nil:
			diagnostic.AssetKind = "data"
			diagnostic.DataType = requested.Data.Type
			switch requested.Data.Type {
			case 1:
				diagnostic.Reason = "required_brand_name_missing"
			case 2:
				diagnostic.Reason = "required_description_missing"
			default:
				diagnostic.Reason = "required_data_type_unsupported"
			}

		case requested.Img != nil:
			diagnostic.AssetKind = "img"
			diagnostic.RequestedW = requested.Img.W
			diagnostic.RequestedH = requested.Img.H
			diagnostic.RequestedWMin = requested.Img.WMin
			diagnostic.RequestedHMin = requested.Img.HMin
			switch {
			case strings.TrimSpace(creative.ImageURL) == "":
				diagnostic.Reason = "required_image_url_missing"
			case creative.W <= 0 || creative.H <= 0:
				diagnostic.Reason = "required_image_dimensions_invalid"
			case requested.Img.WMin > 0 && creative.W < requested.Img.WMin:
				diagnostic.Reason = "required_image_width_below_wmin"
			case requested.Img.HMin > 0 && creative.H < requested.Img.HMin:
				diagnostic.Reason = "required_image_height_below_hmin"
			case !allowedNativeImageFormat(creative.FileFormat):
				diagnostic.Reason = "required_image_format_unsupported"
			default:
				diagnostic.Reason = "required_image_not_eligible"
			}

		default:
			diagnostic.AssetKind = "unsupported"
			diagnostic.Reason = "required_asset_type_unsupported"
		}
		return diagnostic
	}

	diagnostic.Reason = "native_adm_build_failed_unknown"
	return diagnostic
}

func nativeAssetIDText(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return ""
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return string(raw)
	}
	return fmt.Sprint(value)
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
