package bidEngine

import (
	"encoding/json"
	"encoding/xml"
	"html"
	"strings"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/callback"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

// Keep distinct purposes even where the legacy DSP BURL delegates to NURL.
// External DSP BURL itself is deliberately not proxied (existing contract).
func callbackRecord(source *ortb.Bid, format, ssp string, localADV bool) callback.Record {
	r := callback.Record{Format: format, SSP: ssp, WinnerType: "dsp", NURL: source.GetNurl(), BURL: source.GetNurl()}
	if localADV {
		r.WinnerType = "adv"
		r.NURL = ""
		r.BURL = ""
		switch format {
		case "NAT", "IPP":
			var a struct {
				Native struct {
					Link struct {
						URL string `json:"url"`
					} `json:"link"`
				} `json:"native"`
			}
			if json.Unmarshal([]byte(source.GetAdm()), &a) == nil {
				r.Destination = a.Native.Link.URL
			}
		case "BAN":
			if !strings.Contains(strings.ToLower(source.GetAdm()), "<iframe") {
				m := bannerAnchorHrefPattern.FindStringSubmatch(source.GetAdm())
				if len(m) > 3 {
					r.Destination = html.UnescapeString(m[2] + m[3])
				}
			}
		case "VID":
			var a advVideoClickVAST
			if xml.Unmarshal([]byte(source.GetAdm()), &a) == nil {
				r.Destination = a.Ad.Inline.Creatives.Creative.Linear.VideoClicks.ClickThrough
			}
		default:
			r.Destination = source.GetAdm()
		}
	} else if shouldWrapDSPADM(format) {
		r.Destination = source.GetAdm()
	}
	if format == "POP" && strings.HasPrefix(strings.TrimSpace(r.Destination), "<") {
		var a struct {
			Pop struct {
				URL string `xml:"url"`
			} `xml:"popunderAd"`
		}
		if xml.Unmarshal([]byte(r.Destination), &a) == nil {
			r.Destination = a.Pop.URL
		}
	}
	return r
}
