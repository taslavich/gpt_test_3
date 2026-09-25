package auction

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/url"
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
// eligibility check. OpenRTB 2.5 placement is primary; plcmt remains a generic
// compatibility fallback. MyBid imp.ext.pl is normalized into placement in the
// SSP adapter before the request enters the internal gRPC pipeline.
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
	if video.Plcmt != nil && video.GetPlcmt() > 0 {
		switch video.GetPlcmt() {
		case 1:
			return format == VideoFormatInstream
		case 2, 4:
			return format == VideoFormatOutstream
		case 3:
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

func videoCreativeMatchesImpression(creative *Creative, imp *ortb.Imp) (bool, diagnosticReason) {
	if imp == nil || imp.GetVideo() == nil {
		return false, diagVideoObjectMissing
	}
	if creative == nil || !videoFormatMatchesImpression(creative.VideoFormat, imp) {
		return false, diagVideoFormatMismatch
	}
	if !validHTTPURL(creative.ADMURL) || !validHTTPSURL(creative.ImageURL) || creative.VideoMetadata == nil || creative.VideoMetadata.Duration <= 0 || creative.W <= 0 || creative.H <= 0 {
		return false, diagVideoSourceInvalid
	}
	v := imp.GetVideo()
	_, selectedProtocol, ok := selectVASTVersion(v.GetProtocols())
	if !ok {
		return false, diagVideoProtocolMismatch
	}
	meta := creative.VideoMetadata
	if meta == nil {
		meta = &VideoCreativeMetadata{}
	}

	creativeMimes := append([]string(nil), meta.Mimes...)
	if len(creativeMimes) == 0 && strings.HasPrefix(strings.ToLower(strings.TrimSpace(creative.FileFormat)), "video/") {
		creativeMimes = []string{creative.FileFormat}
	}
	// mimes is required by OpenRTB VIDEO. We only own video/mp4 creatives, so
	// absence of an allow-list cannot prove technical compatibility.
	if len(v.GetMimes()) == 0 || len(creativeMimes) == 0 || !stringSliceIntersectsFold(creativeMimes, v.GetMimes()) {
		return false, diagVideoMimeMismatch
	}
	if len(meta.Protocols) > 0 && !int32SliceContains(meta.Protocols, selectedProtocol) {
		return false, diagVideoProtocolMismatch
	}
	if meta.Duration > 0 {
		if v.GetMinduration() > 0 && meta.Duration < v.GetMinduration() {
			return false, diagVideoDurationMismatch
		}
		if v.GetMaxduration() > 0 && meta.Duration > v.GetMaxduration() {
			return false, diagVideoDurationMismatch
		}
	}
	if len(meta.API) > 0 && len(v.GetApi()) > 0 && !int32SliceIntersects(meta.API, v.GetApi()) {
		return false, diagVideoAPIMismatch
	}
	if len(meta.Attributes) > 0 && int32SliceIntersects(meta.Attributes, v.GetBattr()) {
		return false, diagVideoBlockedAttribute
	}
	if meta.Bitrate > 0 {
		if v.GetMinbitrate() > 0 && meta.Bitrate < v.GetMinbitrate() {
			return false, diagVideoBitrateMismatch
		}
		if v.GetMaxbitrate() > 0 && meta.Bitrate > v.GetMaxbitrate() {
			return false, diagVideoBitrateMismatch
		}
	}
	if meta.Linearity > 0 && v.GetLinearity() > 0 && meta.Linearity != v.GetLinearity() {
		return false, diagVideoLinearityMismatch
	}
	if meta.Skippable != nil && v.Skip != nil {
		want := v.GetSkip() == 1
		if *meta.Skippable != want {
			return false, diagVideoSkipMismatch
		}
	}
	if creative.W > 0 && creative.H > 0 && v.GetW() > 0 && v.GetH() > 0 {
		sameAspect := int64(creative.W)*int64(v.GetH()) == int64(creative.H)*int64(v.GetW())
		// OpenRTB defaults boxingallowed to 1 when omitted. Reject an aspect
		// mismatch only when the request explicitly disables boxing.
		boxingAllowed := v.Boxingallowed == nil || v.GetBoxingallowed() == 1
		if !sameAspect && !boxingAllowed {
			return false, diagVideoSizeMismatch
		}
	}
	return true, diagNone
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
	UniversalAdID *vastUniversalAdID `xml:"UniversalAdId,omitempty"`
	Linear        vastLinear         `xml:"Linear"`
}

type vastUniversalAdID struct {
	IDRegistry string `xml:"idRegistry,attr"`
	Value      string `xml:",chardata"`
}

type vastLinear struct {
	Duration    string          `xml:"Duration"`
	VideoClicks vastVideoClicks `xml:"VideoClicks"`
	MediaFiles  vastMediaFiles  `xml:"MediaFiles"`
}

type vastVideoClicks struct {
	ClickThrough string `xml:"ClickThrough"`
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
	URL      string `xml:",chardata"`
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
	linear := vastLinear{
		Duration:    formatVASTDuration(meta.Duration),
		VideoClicks: vastVideoClicks{ClickThrough: clickURL},
		MediaFiles: vastMediaFiles{MediaFile: vastMediaFile{
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
		}},
	}
	creativeNode := vastCreative{Linear: linear}
	if version == "4.0" {
		// UniversalAdId is required by the VAST 4 schema. We use our stable
		// creative UUID under TwinBid's own registry namespace.
		creativeNode.UniversalAdID = &vastUniversalAdID{IDRegistry: "twinbidexchange.com", Value: creative.ID}
	}
	doc := vastDocument{
		Version: version,
		Ad: vastAd{ID: creative.ID, Inline: vastInline{
			AdSystem:   vastAdSystem{Version: "1.0", Value: "TwinBid"},
			AdTitle:    title,
			Impression: videoImpressionPlaceholder,
			Creatives:  vastCreatives{Creative: creativeNode},
		}},
	}
	payload, err := xml.Marshal(doc)
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
