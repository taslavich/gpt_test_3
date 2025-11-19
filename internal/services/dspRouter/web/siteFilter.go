package dspRouterWeb

import "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"

type SiteIdBox struct {
	apply       bool
	isWhiteList bool
	siteIds     map[string]bool
}

func NewSiteIdBox(siteIds []string) *SiteIdBox {
	var newSiteIdMap = make(map[string]bool)
	for i := range siteIds {
		newSiteIdMap[siteIds[i]] = true
	}

	return &SiteIdBox{}
}

/*if bidRequest.Site == nil || bidRequest.Site.Id == nil{
	return false
}

if bidRequest.Site.Id == nil {
	return false
}

if bidRequest.Site.GetId() == "" || bidRequest.Site.GetId() == " " {
	return false
}

siteId := bidRequest.Site.GetId()
whiteList := map[string]bool{
	"137710":     true,
}

return whiteList[siteId]*/

func (s *SiteIdBox) applyFilter(bidRequest *ortb_V2_5.BidRequest) bool {
	if !s.apply {
		return !s.isWhiteList
	}

	if bidRequest.Site == nil {
		return false
	}

	if bidRequest.Site.Id == nil {
		return false
	}

	if bidRequest.Site.GetId() == "" || bidRequest.Site.GetId() == " " {
		return false
	}

	siteId := bidRequest.Site.GetId()

	whiteList := map[string]bool{
		"137710":  true,
		"395785":  true,
		"6097432": true,
	}

	return whiteList[siteId]
}

type filtersBox struct {
}

func allowedUnic(bidRequest *ortb_V2_5.BidRequest) bool {
	return true
}
