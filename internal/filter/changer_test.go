package filter

import (
	"testing"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

func TestChangersBoxChanger_Change(t *testing.T) {
	strPtr := func(s string) *string { return &s }

	tests := []struct {
		name        string
		changerBox  *ChangersBoxChanger
		bidRequest  *ortb_V2_5.BidRequest
		domain      string
		wantChanged bool
		wantSiteID  *string
	}{
		{
			name:        "nil receiver",
			changerBox:  nil,
			bidRequest:  &ortb_V2_5.BidRequest{Site: &ortb_V2_5.Site{Id: strPtr("site123")}},
			domain:      "any",
			wantChanged: false,
		},
		{
			name:        "nil bidRequest",
			changerBox:  &ChangersBoxChanger{},
			bidRequest:  nil,
			domain:      "any",
			wantChanged: false,
		},
		{
			name: "domain exists and modifies",
			changerBox: &ChangersBoxChanger{
				Changers: map[string]*ChangersChanger{
					"test-domain": {
						Apply: true,
						SiteIdBoxes: map[string]*SiteIdBoxChanger{
							"site123": {ToChange: true, SiteId: "newID"},
						},
					},
				},
			},
			bidRequest:  &ortb_V2_5.BidRequest{Site: &ortb_V2_5.Site{Id: strPtr("site123")}},
			domain:      "test-domain",
			wantChanged: true,
			wantSiteID:  strPtr("newID"),
		},
		{
			name: "domain exists but ToChange=false",
			changerBox: &ChangersBoxChanger{
				Changers: map[string]*ChangersChanger{
					"test-domain": {
						Apply: true,
						SiteIdBoxes: map[string]*SiteIdBoxChanger{
							"site123": {ToChange: false, SiteId: "newID"},
						},
					},
				},
			},
			bidRequest:  &ortb_V2_5.BidRequest{Site: &ortb_V2_5.Site{Id: strPtr("site123")}},
			domain:      "test-domain",
			wantChanged: false,
		},
		{
			name: "falls back to ALL",
			changerBox: &ChangersBoxChanger{
				Changers: map[string]*ChangersChanger{
					"ALL": {
						Apply: true,
						SiteIdBoxes: map[string]*SiteIdBoxChanger{
							"site123": {ToChange: true, SiteId: "fromAll"},
						},
					},
				},
			},
			bidRequest:  &ortb_V2_5.BidRequest{Site: &ortb_V2_5.Site{Id: strPtr("site123")}},
			domain:      "non-existent",
			wantChanged: true,
			wantSiteID:  strPtr("fromAll"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, changed := tt.changerBox.Change(tt.bidRequest, tt.domain)

			if changed != tt.wantChanged {
				t.Errorf("got changed=%v, want %v", changed, tt.wantChanged)
			}

			if tt.wantChanged {
				if result == tt.bidRequest {
					t.Error("got same pointer, want new")
				}
				if *result.Site.Id != *tt.wantSiteID {
					t.Errorf("got Site.ID=%q, want %q", *result.Site.Id, *tt.wantSiteID)
				}
			} else if tt.bidRequest != nil && result != tt.bidRequest {
				t.Error("got different pointer, want original")
			}
		})
	}
}
