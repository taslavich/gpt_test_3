package dspRouterWeb

import "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"

type IAllow interface {
	Allowed(bidRequest *ortb_V2_5.BidRequest) bool
}

type SiteIdBox struct {
	apply       bool
	isWhiteList bool
	siteIds     map[string]bool
}

func NewSiteIdBox(apply, isWhiteList bool, siteIds []string) *SiteIdBox {
	var newSiteIdMap map[string]bool
	if apply {
		newSiteIdMap = make(map[string]bool)
		for i := range siteIds {
			newSiteIdMap[siteIds[i]] = true
		}
	}

	return &SiteIdBox{
		siteIds:     newSiteIdMap,
		isWhiteList: isWhiteList,
		apply:       apply,
	}
}

func (s *SiteIdBox) Allowed(bidRequest *ortb_V2_5.BidRequest) bool {
	if s == nil {
		return true
	}

	if !s.apply {
		return true
	}

	siteId := bidRequest.Site.GetId()

	if s.isWhiteList {
		return s.siteIds[siteId]
	} else {
		return !s.siteIds[siteId]
	}
}

type FiltersBox struct {
	allowers []IAllow
}

func NewFiltersBox(allowers []IAllow) *FiltersBox {
	return &FiltersBox{
		allowers: allowers,
	}
}

func (f *FiltersBox) Allowed(bidRequest *ortb_V2_5.BidRequest) bool {
	if bidRequest == nil {
		return false
	}

	if f == nil {
		return true
	}

	for i := range f.allowers {
		if !f.allowers[i].Allowed(bidRequest) {
			return false
		}
	}
	return true
}
