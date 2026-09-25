package auction

import (
	"encoding/json"
	"encoding/xml"
	"io"
	"strings"

	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

const (
	VideoFormatInstream  = "instream"
	VideoFormatOutstream = "outstream"
	VideoFormatPopup     = "video_popup"
)

// VideoCreativeMetadata describes technical requirements/capabilities that are
// needed to decide whether a VIDEO creative can be served into imp.video.
// These fields are not advertiser targeting controls.
type VideoCreativeMetadata struct {
	Mimes      []string `json:"mimes,omitempty"`
	Duration   int32    `json:"duration,omitempty"`
	Protocols  []int32  `json:"protocols,omitempty"`
	API        []int32  `json:"api,omitempty"`
	Attributes []int32  `json:"battr,omitempty"`
	Bitrate    int32    `json:"bitrate,omitempty"`
	Linearity  int32    `json:"linearity,omitempty"`
	Skippable  *bool    `json:"skippable,omitempty"`
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
// eligibility check. OpenRTB 2.5 placement is primary; plcmt is a compatibility
// fallback only when placement is absent/0. If the request contains neither field we cannot
// prove that the inventory matches the selected VIDEO format, so it is rejected.
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
	// plcmt is retained as a compatibility fallback when placement is absent/0.
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

func validVASTXML(adm string) bool {
	decoder := xml.NewDecoder(strings.NewReader(strings.TrimSpace(adm)))
	rootSeen := false
	depth := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return rootSeen && depth == 0
		}
		if err != nil {
			return false
		}
		switch t := token.(type) {
		case xml.StartElement:
			if !rootSeen {
				if !strings.EqualFold(t.Name.Local, "VAST") {
					return false
				}
				rootSeen = true
			}
			depth++
		case xml.EndElement:
			depth--
			if depth < 0 {
				return false
			}
		}
	}
}

func videoCreativeMatchesImpression(creative *Creative, imp *ortb.Imp) (bool, diagnosticReason) {
	if imp == nil || imp.GetVideo() == nil {
		return false, diagVideoObjectMissing
	}
	if creative == nil || !videoFormatMatchesImpression(creative.VideoFormat, imp) {
		return false, diagVideoFormatMismatch
	}
	v := imp.GetVideo()
	if !validVASTXML(creative.ADMURL) {
		return false, diagVideoVASTInvalid
	}
	meta := creative.VideoMetadata
	if meta == nil {
		meta = &VideoCreativeMetadata{}
	}

	creativeMimes := append([]string(nil), meta.Mimes...)
	if len(creativeMimes) == 0 && strings.HasPrefix(strings.ToLower(strings.TrimSpace(creative.FileFormat)), "video/") {
		creativeMimes = []string{creative.FileFormat}
	}
	if len(v.GetMimes()) > 0 && (len(creativeMimes) == 0 || !stringSliceIntersectsFold(creativeMimes, v.GetMimes())) {
		return false, diagVideoMimeMismatch
	}
	if meta.Duration > 0 {
		if v.GetMinduration() > 0 && meta.Duration < v.GetMinduration() {
			return false, diagVideoDurationMismatch
		}
		if v.GetMaxduration() > 0 && meta.Duration > v.GetMaxduration() {
			return false, diagVideoDurationMismatch
		}
	}
	if len(meta.Protocols) > 0 && len(v.GetProtocols()) > 0 && !int32SliceIntersects(meta.Protocols, v.GetProtocols()) {
		return false, diagVideoProtocolMismatch
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
		if !sameAspect && v.GetBoxingallowed() != 1 {
			return false, diagVideoSizeMismatch
		}
	}
	return true, diagNone
}
