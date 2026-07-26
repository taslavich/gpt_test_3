package bidEngine

import (
	"html"
	"regexp"
	"strings"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	"google.golang.org/protobuf/proto"
)

var bannerAnchorHrefPattern = regexp.MustCompile(`(?is)(<a\b[^>]*\bhref\s*=\s*)(?:"([^"]*)"|'([^']*)')`)

func isADVBannerFormat(format string) bool {
	return strings.EqualFold(strings.TrimSpace(format), constants.BAN)
}

// finalizeADVBannerCallbacks keeps banner ADM as HTML.
//
// For an image banner, only the destination in the first <a href="..."> is
// replaced with the exchange /adm click wrapper. The image URL and the rest of
// the markup stay untouched. For iframe ADM there is no click redirect: the
// iframe markup is returned unchanged.
func finalizeADVBannerCallbacks(source *ortb.Bid, admDomain, globalID, sspDomain, format string) (*ortb.Bid, bool) {
	if source == nil || strings.TrimSpace(source.GetAdm()) == "" {
		return nil, false
	}
	finalBid, ok := proto.Clone(source).(*ortb.Bid)
	if !ok || finalBid == nil {
		return nil, false
	}
	finalBid.Nurl = nil
	finalBid.Burl = nil

	adm, ok := finalizeADVBannerADM(source.GetAdm(), admDomain, globalID, format)
	if !ok {
		return nil, false
	}
	finalBid.Adm = &adm

	nurl := utils.WrapADVNurlURL(admDomain, globalID, sspDomain, format)
	if strings.TrimSpace(nurl) == "" {
		return nil, false
	}
	finalBid.Nurl = &nurl

	burl := utils.WrapBurlURL(admDomain, globalID, format)
	if strings.TrimSpace(burl) == "" {
		return nil, false
	}
	finalBid.Burl = &burl
	return finalBid, true
}

func finalizeADVBannerADM(adm, admDomain, globalID, format string) (string, bool) {
	trimmed := strings.TrimSpace(adm)
	if trimmed == "" {
		return "", false
	}

	// Iframe banners remain raw HTML. The exchange does not rewrite iframe src
	// and therefore does not track clicks inside the iframe.
	if strings.Contains(strings.ToLower(trimmed), "<iframe") {
		return adm, true
	}

	match := bannerAnchorHrefPattern.FindStringSubmatchIndex(adm)
	if match == nil {
		return "", false
	}

	hrefStart, hrefEnd := match[4], match[5]
	quote := byte('"')
	if hrefStart < 0 || hrefEnd < 0 {
		hrefStart, hrefEnd = match[6], match[7]
		quote = '\''
	}
	if hrefStart < 0 || hrefEnd < 0 {
		return "", false
	}

	destination := strings.TrimSpace(html.UnescapeString(adm[hrefStart:hrefEnd]))
	if destination == "" {
		return "", false
	}
	wrapped := utils.WrapURL(admDomain, destination, globalID, format)
	if strings.TrimSpace(wrapped) == "" {
		return "", false
	}

	wrapped = html.EscapeString(wrapped)
	if quote == '\'' {
		wrapped = strings.ReplaceAll(wrapped, "&#39;", "&apos;")
	}
	return adm[:hrefStart] + wrapped + adm[hrefEnd:], true
}
