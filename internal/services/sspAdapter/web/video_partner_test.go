package sppAdapterWeb

import (
	"encoding/json"
	"testing"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
)

func TestMyBidExtPLNormalization(t *testing.T) {
	var payload postBidPayloadV25
	if err := json.Unmarshal([]byte(`{
		"id":"req-1",
		"imp":[
			{"id":"i1","video":{"placement":0},"ext":{"pl":61}},
			{"id":"i2","video":{},"ext":{"pl":306}},
			{"id":"i3","video":{"placement":5},"ext":{"pl":61}},
			{"id":"i4","video":{"plcmt":1},"ext":{"pl":999}}
		]
	}`), &payload); err != nil {
		t.Fatal(err)
	}
	normalizePartnerVideoPlacement(&payload, "adl_mybid.com", constants.VID)
	imps := payload.GetImp()
	if got := imps[0].GetVideo().GetPlacement(); got != 1 {
		t.Fatalf("ext.pl=61 normalized to %d, want 1", got)
	}
	if got := imps[1].GetVideo().GetPlacement(); got != 3 {
		t.Fatalf("ext.pl=306 normalized to %d, want 3", got)
	}
	if got := imps[2].GetVideo().GetPlacement(); got != 5 {
		t.Fatalf("standard placement must win, got %d", got)
	}
	if imps[3].GetVideo().Placement != nil || imps[3].GetVideo().Plcmt != nil {
		t.Fatalf("unknown MyBid ext.pl must stay unclassified: %#v", imps[3].GetVideo())
	}
}

func TestPartnerExtPLDoesNotLeakToOtherSSPOrFormats(t *testing.T) {
	body := []byte(`{"id":"r","imp":[{"id":"i","video":{},"ext":{"pl":308}}]}`)
	for _, tc := range []struct{ ssp, format string }{{"other.example", constants.VID}, {"mc_mybid.com", constants.BAN}} {
		var payload postBidPayloadV25
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatal(err)
		}
		normalizePartnerVideoPlacement(&payload, tc.ssp, tc.format)
		if payload.GetImp()[0].GetVideo().Placement != nil {
			t.Fatalf("partner placement leaked for ssp=%q format=%q", tc.ssp, tc.format)
		}
	}
}
