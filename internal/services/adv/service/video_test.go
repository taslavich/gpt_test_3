package auction

import (
	"testing"

	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

func i32(v int32) *int32 { return &v }

func TestVideoFormatMatchesImpression(t *testing.T) {
	tests := []struct {
		name      string
		format    string
		placement *int32
		plcmt     *int32
		want      bool
	}{
		{name: "instream modern", format: VideoFormatInstream, plcmt: i32(1), want: true},
		{name: "instream legacy", format: VideoFormatInstream, placement: i32(1), want: true},
		{name: "outstream accompanying content", format: VideoFormatOutstream, plcmt: i32(2), want: true},
		{name: "outstream standalone", format: VideoFormatOutstream, plcmt: i32(4), want: true},
		{name: "outstream legacy in article", format: VideoFormatOutstream, placement: i32(3), want: true},
		{name: "popup modern", format: VideoFormatPopup, plcmt: i32(3), want: true},
		{name: "popup legacy", format: VideoFormatPopup, placement: i32(5), want: true},
		{name: "wrong format", format: VideoFormatInstream, plcmt: i32(3), want: false},
		{name: "modern field has priority", format: VideoFormatPopup, placement: i32(5), plcmt: i32(1), want: false},
		{name: "missing classification", format: VideoFormatOutstream, want: false},
		{name: "legacy frontend alias", format: "outstream_slider", placement: i32(4), want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			imp := &ortb.Imp{Video: &ortb.Video{Placement: test.placement, Plcmt: test.plcmt}}
			if got := videoFormatMatchesImpression(test.format, imp); got != test.want {
				t.Fatalf("videoFormatMatchesImpression(%q)=%v want=%v", test.format, got, test.want)
			}
		})
	}
}

func TestVideoCreativeCompatibility(t *testing.T) {
	skippable := true
	creative := &Creative{
		ID: "v1", ADMURL: `<?xml version="1.0"?><VAST version="3.0"></VAST>`, FileFormat: "video/mp4", W: 1920, H: 1080,
		VideoFormat:   VideoFormatInstream,
		VideoMetadata: &VideoCreativeMetadata{Mimes: []string{"video/mp4"}, Duration: 20, Protocols: []int32{2, 3, 5, 6}, API: []int32{2}, Bitrate: 1200, Linearity: 1, Skippable: &skippable},
	}
	imp := &ortb.Imp{Video: &ortb.Video{
		Mimes: []string{"video/mp4", "video/webm"}, Minduration: i32(5), Maxduration: i32(30),
		Protocols: []int32{2, 5}, W: i32(400), H: i32(225), Linearity: i32(1),
		Minbitrate: i32(300), Maxbitrate: i32(9600), Api: []int32{2}, Skip: i32(1), Plcmt: i32(1),
		// These low-level request attributes are deliberately not advertiser filters.
		Startdelay: i32(-1), Playbackmethod: []int32{4}, Pos: i32(7),
	}}
	if ok, reason := videoCreativeMatchesImpression(creative, imp); !ok || reason != diagNone {
		t.Fatalf("compatible VIDEO creative rejected: reason=%v", reason)
	}
	imp.Video.Maxduration = i32(10)
	if ok, reason := videoCreativeMatchesImpression(creative, imp); ok || reason != diagVideoDurationMismatch {
		t.Fatalf("duration mismatch not detected: ok=%v reason=%v", ok, reason)
	}
}

func TestVideoCreativeFormatMismatch(t *testing.T) {
	creative := &Creative{
		ID: "v1", ADMURL: `<?xml version="1.0"?><VAST version="3.0"></VAST>`, FileFormat: "video/mp4",
		VideoFormat: VideoFormatInstream,
	}
	imp := &ortb.Imp{Video: &ortb.Video{Mimes: []string{"video/mp4"}, Plcmt: i32(3)}}
	if ok, reason := videoCreativeMatchesImpression(creative, imp); ok || reason != diagVideoFormatMismatch {
		t.Fatalf("VIDEO format mismatch not detected: ok=%v reason=%v", ok, reason)
	}
}

func TestVideoCreativeRejectsNonVAST(t *testing.T) {
	creative := &Creative{ID: "v1", ADMURL: "https://example.test/vast", FileFormat: "video/mp4", VideoFormat: VideoFormatInstream}
	imp := &ortb.Imp{Video: &ortb.Video{Mimes: []string{"video/mp4"}, Plcmt: i32(1)}}
	if ok, reason := videoCreativeMatchesImpression(creative, imp); ok || reason != diagVideoVASTInvalid {
		t.Fatalf("non-VAST ADM accepted: ok=%v reason=%v", ok, reason)
	}
}

func TestParseVideoMetadataCabinetContract(t *testing.T) {
	metadata, err := parseVideoCreativeMetadataJSONB([]byte(`{
		"mimes":["video/mp4"],
		"duration":20,
		"protocols":[2,3,5,6],
		"api":[2],
		"battr":[8,9],
		"bitrate":1200,
		"linearity":1,
		"skippable":true
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if metadata == nil || metadata.Duration != 20 || metadata.Bitrate != 1200 || metadata.Linearity != 1 {
		t.Fatalf("unexpected cabinet VIDEO metadata: %#v", metadata)
	}
	if len(metadata.Mimes) != 1 || metadata.Mimes[0] != "video/mp4" || len(metadata.Protocols) != 4 || len(metadata.Attributes) != 2 {
		t.Fatalf("cabinet VIDEO metadata lists not parsed: %#v", metadata)
	}
	if metadata.Skippable == nil || !*metadata.Skippable {
		t.Fatalf("skippable flag not parsed: %#v", metadata)
	}
}
