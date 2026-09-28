package auction

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

const (
	VideoFormatInstream  = "instream"
	VideoFormatOutstream = "outstream"
	VideoFormatPopup     = "video_popup"

	// The source VAST exists only between ADV and BidEngine. BidEngine replaces
	// this URL with the public exchange no-op Impression endpoint before the bid
	// is returned to the SSP.
	videoImpressionPlaceholder = "https://invalid.twinbid.local/video-impression"
)

// VideoCreativeMetadata is server-derived technical metadata for an uploaded
// VIDEO creative. These fields are compatibility facts, not campaign targeting.
type VideoCreativeMetadata struct {
	Mimes      []string `json:"mimes,omitempty"`
	Duration   int32    `json:"duration,omitempty"`
	Protocols  []int32  `json:"protocols,omitempty"`
	API        []int32  `json:"api,omitempty"`
	Attributes []int32  `json:"battr,omitempty"`
	Bitrate    int32    `json:"bitrate,omitempty"`
	Linearity  int32    `json:"linearity,omitempty"`
	Skippable  *bool    `json:"skippable,omitempty"`
	Width      int32    `json:"width,omitempty"`
	Height     int32    `json:"height,omitempty"`
	Codec      string   `json:"codec,omitempty"`
	FileSize   int64    `json:"file_size,omitempty"`
}

func normalizeVideoFormat(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case VideoFormatInstream:
		return VideoFormatInstream
	case VideoFormatOutstream, "outstream_standard", "outstream_slider":
		return VideoFormatOutstream
	case VideoFormatPopup:
		return VideoFormatPopup
	default:
		return ""
	}
}

// videoFormatMatchesImpression is the only advertiser-selectable VIDEO-specific
// eligibility check. OpenRTB 2.5 placement is authoritative. MyBid imp.ext.pl is
// normalized into placement in the SSP adapter before the request enters the
// internal gRPC pipeline; generic plcmt is deliberately not used here.
func videoFormatMatchesImpression(videoFormat string, imp *ortb.Imp) bool {
	if imp == nil || imp.GetVideo() == nil {
		return false
	}
	format := normalizeVideoFormat(videoFormat)
	if format == "" {
		return false
	}
	video := imp.GetVideo()
	if video.Placement != nil && video.GetPlacement() > 0 {
		switch video.GetPlacement() {
		case 1:
			return format == VideoFormatInstream
		case 2, 3, 4:
			return format == VideoFormatOutstream
		case 5:
			return format == VideoFormatPopup
		default:
			return false
		}
	}
	return false
}

func parseVideoCreativeMetadataJSONB(raw []byte) (*VideoCreativeMetadata, error) {
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "{}" {
		return &VideoCreativeMetadata{}, nil
	}
	var out VideoCreativeMetadata
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	for i := range out.Mimes {
		out.Mimes[i] = strings.ToLower(strings.TrimSpace(out.Mimes[i]))
	}
	out.Codec = strings.ToLower(strings.TrimSpace(out.Codec))
	return &out, nil
}

func cloneVideoCreativeMetadata(src *VideoCreativeMetadata) *VideoCreativeMetadata {
	if src == nil {
		return nil
	}
	out := *src
	out.Mimes = append([]string(nil), src.Mimes...)
	out.Protocols = append([]int32(nil), src.Protocols...)
	out.API = append([]int32(nil), src.API...)
	out.Attributes = append([]int32(nil), src.Attributes...)
	if src.Skippable != nil {
		v := *src.Skippable
		out.Skippable = &v
	}
	return &out
}

func int32SliceIntersects(a, b []int32) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	set := make(map[int32]struct{}, len(a))
	for _, v := range a {
		set[v] = struct{}{}
	}
	for _, v := range b {
		if _, ok := set[v]; ok {
			return true
		}
	}
	return false
}

func int32SliceContains(values []int32, want int32) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func stringSliceIntersectsFold(a, b []string) bool {
	set := make(map[string]struct{}, len(a))
	for _, v := range a {
		if v = strings.ToLower(strings.TrimSpace(v)); v != "" {
			set[v] = struct{}{}
		}
	}
	for _, v := range b {
		if _, ok := set[strings.ToLower(strings.TrimSpace(v))]; ok {
			return true
		}
	}
	return false
}

// selectVASTVersion intentionally supports only InLine protocols for our own
// uploaded MP4. Wrapper-only inventory is therefore a no-bid.
func selectVASTVersion(protocols []int32) (version string, protocol int32, ok bool) {
	if int32SliceContains(protocols, 7) {
		return "4.0", 7, true
	}
	if int32SliceContains(protocols, 3) {
		return "3.0", 3, true
	}
	if int32SliceContains(protocols, 2) {
		return "2.0", 2, true
	}
	return "", 0, false
}

func validHTTPURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

func validHTTPSURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && u.Host != "" && u.Scheme == "https"
}

type videoFilterTrace struct {
	Matched          bool
	Reason           diagnosticReason
	RequestFormat    string
	CreativeFormat   string
	SelectedProtocol int32
	FormatCheck      string
	SourceCheck      string
	ProtocolCheck    string
	MimeCheck        string
	DurationCheck    string
	APICheck         string
	BattrCheck       string
	BitrateCheck     string
	LinearityCheck   string
	SkipCheck        string
	SizeCheck        string
}

func newVideoFilterTrace(creative *Creative, imp *ortb.Imp) videoFilterTrace {
	trace := videoFilterTrace{
		Reason:         diagNone,
		FormatCheck:    "not_checked",
		SourceCheck:    "not_checked",
		ProtocolCheck:  "not_checked",
		MimeCheck:      "not_checked",
		DurationCheck:  "not_checked",
		APICheck:       "not_checked",
		BattrCheck:     "not_checked",
		BitrateCheck:   "not_checked",
		LinearityCheck: "not_checked",
		SkipCheck:      "not_checked",
		SizeCheck:      "not_checked",
	}
	if creative != nil {
		trace.CreativeFormat = normalizeVideoFormat(creative.VideoFormat)
	}
	trace.RequestFormat = videoRequestFormat(imp)
	return trace
}

func videoRequestFormat(imp *ortb.Imp) string {
	if imp == nil || imp.GetVideo() == nil {
		return ""
	}
	video := imp.GetVideo()
	if video.Placement == nil || video.GetPlacement() <= 0 {
		return ""
	}
	switch video.GetPlacement() {
	case 1:
		return VideoFormatInstream
	case 2, 3, 4:
		return VideoFormatOutstream
	case 5:
		return VideoFormatPopup
	default:
		return fmt.Sprintf("placement_%d", video.GetPlacement())
	}
}

// evaluateVideoCreativeCompatibility is the single source of truth for VIDEO
// technical eligibility. The trace fields are observational only: the wrapper
// below preserves the historical bool/reason API used by auction selection.
func evaluateVideoCreativeCompatibility(creative *Creative, imp *ortb.Imp) videoFilterTrace {
	trace := newVideoFilterTrace(creative, imp)
	if imp == nil || imp.GetVideo() == nil {
		trace.Reason = diagVideoObjectMissing
		return trace
	}

	trace.FormatCheck = "fail"
	if creative == nil || !videoFormatMatchesImpression(creative.VideoFormat, imp) {
		trace.Reason = diagVideoFormatMismatch
		return trace
	}
	trace.FormatCheck = "pass"

	trace.SourceCheck = "fail"
	if !validHTTPURL(creative.ADMURL) || !validHTTPSURL(creative.ImageURL) || creative.VideoMetadata == nil || creative.VideoMetadata.Duration <= 0 || creative.W <= 0 || creative.H <= 0 {
		trace.Reason = diagVideoSourceInvalid
		return trace
	}
	trace.SourceCheck = "pass"

	v := imp.GetVideo()
	trace.ProtocolCheck = "fail"
	_, selectedProtocol, ok := selectVASTVersion(v.GetProtocols())
	trace.SelectedProtocol = selectedProtocol
	if !ok {
		trace.Reason = diagVideoProtocolMismatch
		return trace
	}
	// The request offers a supported InLine protocol. Creative protocol
	// compatibility is checked after MIME, preserving the historical rejection
	// order; until then the trace explicitly says only the request side passed.
	trace.ProtocolCheck = "request_pass"
	meta := creative.VideoMetadata

	creativeMimes := append([]string(nil), meta.Mimes...)
	if len(creativeMimes) == 0 && strings.HasPrefix(strings.ToLower(strings.TrimSpace(creative.FileFormat)), "video/") {
		creativeMimes = []string{creative.FileFormat}
	}
	trace.MimeCheck = "fail"
	// mimes is required by OpenRTB VIDEO. We only own video/mp4 creatives, so
	// absence of an allow-list cannot prove technical compatibility.
	if len(v.GetMimes()) == 0 || len(creativeMimes) == 0 || !stringSliceIntersectsFold(creativeMimes, v.GetMimes()) {
		trace.Reason = diagVideoMimeMismatch
		return trace
	}
	trace.MimeCheck = "pass"

	if len(meta.Protocols) > 0 && !int32SliceContains(meta.Protocols, selectedProtocol) {
		trace.Reason = diagVideoProtocolMismatch
		return trace
	}
	trace.ProtocolCheck = "pass"

	trace.DurationCheck = "pass"
	if meta.Duration > 0 {
		if v.GetMinduration() > 0 && meta.Duration < v.GetMinduration() {
			trace.DurationCheck = "fail"
			trace.Reason = diagVideoDurationMismatch
			return trace
		}
		if v.GetMaxduration() > 0 && meta.Duration > v.GetMaxduration() {
			trace.DurationCheck = "fail"
			trace.Reason = diagVideoDurationMismatch
			return trace
		}
	}

	trace.APICheck = "pass"
	if len(meta.API) > 0 && len(v.GetApi()) > 0 && !int32SliceIntersects(meta.API, v.GetApi()) {
		trace.APICheck = "fail"
		trace.Reason = diagVideoAPIMismatch
		return trace
	}

	trace.BattrCheck = "pass"
	if len(meta.Attributes) > 0 && int32SliceIntersects(meta.Attributes, v.GetBattr()) {
		trace.BattrCheck = "fail"
		trace.Reason = diagVideoBlockedAttribute
		return trace
	}

	trace.BitrateCheck = "pass"
	if meta.Bitrate > 0 {
		if v.GetMinbitrate() > 0 && meta.Bitrate < v.GetMinbitrate() {
			trace.BitrateCheck = "fail"
			trace.Reason = diagVideoBitrateMismatch
			return trace
		}
		if v.GetMaxbitrate() > 0 && meta.Bitrate > v.GetMaxbitrate() {
			trace.BitrateCheck = "fail"
			trace.Reason = diagVideoBitrateMismatch
			return trace
		}
	}

	trace.LinearityCheck = "pass"
	if meta.Linearity > 0 && v.GetLinearity() > 0 && meta.Linearity != v.GetLinearity() {
		trace.LinearityCheck = "fail"
		trace.Reason = diagVideoLinearityMismatch
		return trace
	}

	trace.SkipCheck = "pass"
	if meta.Skippable != nil && v.Skip != nil {
		want := v.GetSkip() == 1
		if *meta.Skippable != want {
			trace.SkipCheck = "fail"
			trace.Reason = diagVideoSkipMismatch
			return trace
		}
	}

	trace.SizeCheck = "pass"
	if creative.W > 0 && creative.H > 0 && v.GetW() > 0 && v.GetH() > 0 {
		sameAspect := int64(creative.W)*int64(v.GetH()) == int64(creative.H)*int64(v.GetW())
		// OpenRTB defaults boxingallowed to 1 when omitted. Reject an aspect
		// mismatch only when the request explicitly disables boxing.
		boxingAllowed := v.Boxingallowed == nil || v.GetBoxingallowed() == 1
		if !sameAspect && !boxingAllowed {
			trace.SizeCheck = "fail"
			trace.Reason = diagVideoSizeMismatch
			return trace
		}
	}

	trace.Matched = true
	trace.Reason = diagNone
	return trace
}

func videoCreativeMatchesImpression(creative *Creative, imp *ortb.Imp) (bool, diagnosticReason) {
	trace := evaluateVideoCreativeCompatibility(creative, imp)
	return trace.Matched, trace.Reason
}

type vastDocument struct {
	XMLName xml.Name `xml:"VAST"`
	Version string   `xml:"version,attr"`
	Ad      vastAd   `xml:"Ad"`
}

type vastAd struct {
	ID     string     `xml:"id,attr,omitempty"`
	Inline vastInline `xml:"InLine"`
}

type vastInline struct {
	AdSystem   vastAdSystem  `xml:"AdSystem"`
	AdTitle    string        `xml:"AdTitle"`
	Impression string        `xml:"Impression"`
	Creatives  vastCreatives `xml:"Creatives"`
}

type vastAdSystem struct {
	Version string `xml:"version,attr,omitempty"`
	Value   string `xml:",chardata"`
}

type vastCreatives struct {
	Creative vastCreative `xml:"Creative"`
}

type vastCreative struct {
	Linear vastLinear `xml:"Linear"`
}

type vastUniversalAdID struct {
	IDRegistry string `xml:"idRegistry,attr"`
	IDValue    string `xml:"idValue,attr"`
	Value      string `xml:",chardata"`
}

type vastLinear struct {
	Duration    string          `xml:"Duration"`
	VideoClicks vastVideoClicks `xml:"VideoClicks"`
	MediaFiles  vastMediaFiles  `xml:"MediaFiles"`
}

type vastVideoClicks struct {
	ClickThrough vastCDATAValue `xml:"ClickThrough"`
}

type vastCDATAValue struct {
	Value string `xml:",cdata"`
}

type vastMediaFiles struct {
	MediaFile vastMediaFile `xml:"MediaFile"`
}

type vastMediaFile struct {
	Delivery string `xml:"delivery,attr"`
	Type     string `xml:"type,attr"`
	Width    int    `xml:"width,attr"`
	Height   int    `xml:"height,attr"`
	Bitrate  int32  `xml:"bitrate,attr,omitempty"`
	Codec    string `xml:"codec,attr,omitempty"`
	URL      string `xml:",cdata"`
}

// VAST 4 uses a namespace and a different schema order than VAST 2/3. Keep a
// dedicated representation so VAST 2/3 compatibility is not changed while the
// protocol=7 response validates against the IAB VAST 4 schema.
type vast4Document struct {
	XMLName xml.Name `xml:"VAST"`
	XMLNS   string   `xml:"xmlns,attr"`
	Version string   `xml:"version,attr"`
	Ad      vast4Ad  `xml:"Ad"`
}

type vast4Ad struct {
	ID     string      `xml:"id,attr,omitempty"`
	Inline vast4Inline `xml:"InLine"`
}

type vast4Inline struct {
	AdSystem   vastAdSystem   `xml:"AdSystem"`
	Impression string         `xml:"Impression"`
	AdTitle    string         `xml:"AdTitle"`
	Creatives  vast4Creatives `xml:"Creatives"`
}

type vast4Creatives struct {
	Creative vast4Creative `xml:"Creative"`
}

type vast4Creative struct {
	UniversalAdID vastUniversalAdID `xml:"UniversalAdId"`
	Linear        vast4Linear       `xml:"Linear"`
}

type vast4Linear struct {
	Duration    string          `xml:"Duration"`
	MediaFiles  vastMediaFiles  `xml:"MediaFiles"`
	VideoClicks vastVideoClicks `xml:"VideoClicks"`
}

func buildOwnVideoVAST(imp *ortb.Imp, creative *Creative, clickURL string) (string, bool) {
	if imp == nil || imp.GetVideo() == nil || creative == nil || creative.VideoMetadata == nil {
		return "", false
	}
	version, _, ok := selectVASTVersion(imp.GetVideo().GetProtocols())
	if !ok || !validHTTPURL(clickURL) || !validHTTPSURL(creative.ImageURL) {
		return "", false
	}
	meta := creative.VideoMetadata
	if meta.Duration <= 0 || creative.W <= 0 || creative.H <= 0 {
		return "", false
	}
	title := strings.TrimSpace(creative.CreativeName)
	if title == "" {
		title = strings.TrimSpace(creative.ID)
	}
	if title == "" {
		title = "TwinBid Video"
	}
	mediaFiles := vastMediaFiles{MediaFile: vastMediaFile{
		Delivery: "progressive",
		Type:     "video/mp4",
		Width:    creative.W,
		Height:   creative.H,
		Bitrate:  meta.Bitrate,
		// ffprobe codec_name is stored for compatibility/debugging but is
		// not necessarily an RFC 6381 codec string, so do not mislabel it
		// in VAST's optional codec attribute.
		Codec: "",
		URL:   creative.ImageURL,
	}}
	videoClicks := vastVideoClicks{ClickThrough: vastCDATAValue{Value: clickURL}}
	duration := formatVASTDuration(meta.Duration)

	var payload []byte
	var err error
	if version == "4.0" {
		id := strings.TrimSpace(creative.ID)
		if id == "" {
			id = "unknown"
		}
		doc := vast4Document{
			XMLNS:   "http://www.iab.com/VAST",
			Version: version,
			Ad: vast4Ad{ID: creative.ID, Inline: vast4Inline{
				AdSystem:   vastAdSystem{Version: "1.0", Value: "TwinBid"},
				Impression: videoImpressionPlaceholder,
				AdTitle:    title,
				Creatives: vast4Creatives{Creative: vast4Creative{
					UniversalAdID: vastUniversalAdID{IDRegistry: "twinbidexchange.com", IDValue: id, Value: id},
					Linear: vast4Linear{
						Duration: duration, MediaFiles: mediaFiles, VideoClicks: videoClicks,
					},
				}},
			}},
		}
		payload, err = xml.Marshal(doc)
	} else {
		doc := vastDocument{
			Version: version,
			Ad: vastAd{ID: creative.ID, Inline: vastInline{
				AdSystem:   vastAdSystem{Version: "1.0", Value: "TwinBid"},
				AdTitle:    title,
				Impression: videoImpressionPlaceholder,
				Creatives: vastCreatives{Creative: vastCreative{Linear: vastLinear{
					Duration: duration, VideoClicks: videoClicks, MediaFiles: mediaFiles,
				}}},
			}},
		}
		payload, err = xml.Marshal(doc)
	}
	if err != nil {
		return "", false
	}
	return xml.Header + string(payload), true
}

func formatVASTDuration(seconds int32) string {
	if seconds < 0 {
		seconds = 0
	}
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	secs := seconds % 60
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, secs)
}

func optionalBoolLogValue(value *bool) string {
	if value == nil {
		return "<unset>"
	}
	return strconv.FormatBool(*value)
}

func logVideoFilterTrace(
	logf debugLogFunc,
	requestID, impID string,
	campaign *Campaign,
	creative *Creative,
	imp *ortb.Imp,
) {
	if logf == nil {
		return
	}

	trace := evaluateVideoCreativeCompatibility(creative, imp)
	rejectReason := ""
	if !trace.Matched {
		rejectReason = diagnosticReasonName(trace.Reason)
	}

	campaignID := ""
	userID := ""
	if campaign != nil {
		campaignID = strings.TrimSpace(campaign.ID)
		userID = strings.TrimSpace(campaign.UserID)
	}

	creativeID := ""
	creativeFormatRaw := ""
	creativeFileFormat := ""
	creativeW := 0
	creativeH := 0
	creativeADMValid := false
	creativeMediaURLValid := false
	metadataPresent := false
	creativeMimes := []string(nil)
	creativeDuration := int32(0)
	creativeProtocols := []int32(nil)
	creativeAPI := []int32(nil)
	creativeBattr := []int32(nil)
	creativeBitrate := int32(0)
	creativeLinearity := int32(0)
	creativeSkippable := "<unset>"
	creativeCodec := ""
	creativeFileSize := int64(0)
	if creative != nil {
		creativeID = strings.TrimSpace(creative.ID)
		creativeFormatRaw = strings.TrimSpace(creative.VideoFormat)
		creativeFileFormat = strings.TrimSpace(creative.FileFormat)
		creativeW = creative.W
		creativeH = creative.H
		creativeADMValid = validHTTPURL(creative.ADMURL)
		creativeMediaURLValid = validHTTPSURL(creative.ImageURL)
		if creative.VideoMetadata != nil {
			metadataPresent = true
			meta := creative.VideoMetadata
			creativeMimes = meta.Mimes
			creativeDuration = meta.Duration
			creativeProtocols = meta.Protocols
			creativeAPI = meta.API
			creativeBattr = meta.Attributes
			creativeBitrate = meta.Bitrate
			creativeLinearity = meta.Linearity
			creativeSkippable = optionalBoolLogValue(meta.Skippable)
			creativeCodec = meta.Codec
			creativeFileSize = meta.FileSize
		}
	}

	requestVideoPresent := imp != nil && imp.GetVideo() != nil
	requestPlacementSet := false
	requestPlacement := int32(0)
	requestPlcmtSet := false
	requestPlcmt := int32(0)
	requestMimes := []string(nil)
	requestMinDuration := int32(0)
	requestMaxDuration := int32(0)
	requestProtocols := []int32(nil)
	requestAPI := []int32(nil)
	requestBattr := []int32(nil)
	requestMinBitrate := int32(0)
	requestMaxBitrate := int32(0)
	requestLinearity := int32(0)
	requestSkipSet := false
	requestSkip := int32(0)
	requestW := int32(0)
	requestH := int32(0)
	requestBoxingSet := false
	requestBoxingAllowed := int32(0)
	if requestVideoPresent {
		video := imp.GetVideo()
		requestPlacementSet = video.Placement != nil
		requestPlacement = video.GetPlacement()
		requestPlcmtSet = video.Plcmt != nil
		requestPlcmt = video.GetPlcmt()
		requestMimes = video.GetMimes()
		requestMinDuration = video.GetMinduration()
		requestMaxDuration = video.GetMaxduration()
		requestProtocols = video.GetProtocols()
		requestAPI = video.GetApi()
		requestBattr = video.GetBattr()
		requestMinBitrate = video.GetMinbitrate()
		requestMaxBitrate = video.GetMaxbitrate()
		requestLinearity = video.GetLinearity()
		requestSkipSet = video.Skip != nil
		requestSkip = video.GetSkip()
		requestW = video.GetW()
		requestH = video.GetH()
		requestBoxingSet = video.Boxingallowed != nil
		requestBoxingAllowed = video.GetBoxingallowed()
	}

	logf(
		"[ADV][VIDEO_FILTER] request_id=%q imp_id=%q campaign_id=%q user_id=%q creative_id=%q matched=%t reject_reason=%q request_video_present=%t request_placement_set=%t request_placement=%d request_plcmt_set=%t request_plcmt=%d request_format=%q request_mimes=%q request_minduration=%d request_maxduration=%d request_protocols=%v selected_protocol=%d request_api=%v request_battr=%v request_minbitrate=%d request_maxbitrate=%d request_linearity=%d request_skip_set=%t request_skip=%d request_width=%d request_height=%d request_boxingallowed_set=%t request_boxingallowed=%d creative_video_format_raw=%q creative_video_format_normalized=%q creative_file_format=%q creative_width=%d creative_height=%d creative_adm_valid=%t creative_media_url_valid=%t metadata_present=%t creative_mimes=%q creative_duration=%d creative_protocols=%v creative_api=%v creative_battr=%v creative_bitrate=%d creative_linearity=%d creative_skippable=%q creative_codec=%q creative_file_size=%d format_check=%q source_check=%q protocol_check=%q mime_check=%q duration_check=%q api_check=%q battr_check=%q bitrate_check=%q linearity_check=%q skip_check=%q size_check=%q",
		requestID,
		impID,
		campaignID,
		userID,
		creativeID,
		trace.Matched,
		rejectReason,
		requestVideoPresent,
		requestPlacementSet,
		requestPlacement,
		requestPlcmtSet,
		requestPlcmt,
		trace.RequestFormat,
		strings.Join(requestMimes, ","),
		requestMinDuration,
		requestMaxDuration,
		requestProtocols,
		trace.SelectedProtocol,
		requestAPI,
		requestBattr,
		requestMinBitrate,
		requestMaxBitrate,
		requestLinearity,
		requestSkipSet,
		requestSkip,
		requestW,
		requestH,
		requestBoxingSet,
		requestBoxingAllowed,
		creativeFormatRaw,
		trace.CreativeFormat,
		creativeFileFormat,
		creativeW,
		creativeH,
		creativeADMValid,
		creativeMediaURLValid,
		metadataPresent,
		strings.Join(creativeMimes, ","),
		creativeDuration,
		creativeProtocols,
		creativeAPI,
		creativeBattr,
		creativeBitrate,
		creativeLinearity,
		creativeSkippable,
		creativeCodec,
		creativeFileSize,
		trace.FormatCheck,
		trace.SourceCheck,
		trace.ProtocolCheck,
		trace.MimeCheck,
		trace.DurationCheck,
		trace.APICheck,
		trace.BattrCheck,
		trace.BitrateCheck,
		trace.LinearityCheck,
		trace.SkipCheck,
		trace.SizeCheck,
	)
}
