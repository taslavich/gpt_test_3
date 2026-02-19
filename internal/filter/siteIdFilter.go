package filter

import (
	"encoding/json"
	"fmt"
	"os"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type SiteIdBox struct {
	Apply       bool
	IsWhiteList bool
	SiteIds     map[string]bool
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
		SiteIds:     newSiteIdMap,
		IsWhiteList: isWhiteList,
		Apply:       apply,
	}
}

func (s *SiteIdBox) Allowed(bidRequest *ortb_V2_5.BidRequest) bool {
	if s == nil {
		return true
	}

	if !s.Apply {
		return true
	}

	siteId := bidRequest.Site.GetId()

	if s.IsWhiteList {
		return s.SiteIds[siteId]
	} else {
		return !s.SiteIds[siteId]
	}
}

type Filters struct {
	SiteId *SiteIdBox `json:"siteId"`
}

type FiltersBox struct {
	Allowers map[string]*Filters
}

func NewFiltersBox(filename string) (*FiltersBox, error) {

	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filename, err)
	}

	var tmp FiltersJsonBox

	err = json.Unmarshal(data, &tmp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSON from %s: %w", filename, err)
	}

	return &FiltersBox{
		Allowers: FiltersJsonToFilters(tmp),
	}, nil
}

func (f *FiltersBox) Allowed(bidRequest *ortb_V2_5.BidRequest, domain string) bool {
	if bidRequest == nil {
		return false
	}

	if f == nil {
		return true
	}

	filters, ok := f.Allowers[domain]
	if ok {
		if !filters.SiteId.Allowed(bidRequest) {
			return false
		}
	}

	filtersALL, ok := f.Allowers["ALL"]
	if ok {
		if !filtersALL.SiteId.Allowed(bidRequest) {
			return false
		}
	}

	return true
}

type SiteIdBoxJson struct {
	Apply       bool     `json:"apply"`
	IsWhiteList bool     `json:"isWhiteList"`
	SiteIds     []string `json:"siteIds"`
}

func getSiteIdBox(siteIdBoxJson *SiteIdBoxJson) *SiteIdBox {
	if siteIdBoxJson == nil {
		return &SiteIdBox{
			Apply:       false,
			IsWhiteList: false,
			SiteIds:     map[string]bool{},
		}
	}

	return &SiteIdBox{
		Apply:       siteIdBoxJson.Apply,
		IsWhiteList: siteIdBoxJson.IsWhiteList,
		SiteIds:     SliceToMapTrue(siteIdBoxJson.SiteIds),
	}
}

type FiltersJson struct {
	SiteId *SiteIdBoxJson `json:"siteId"`
}

func FiltersJsonToFilters(mapa FiltersJsonBox) map[string]*Filters {
	newMap := make(map[string]*Filters)
	for domain, filters := range mapa {
		domains := utils.SplitAndTrimKeys(domain)

		for _, singleDomain := range domains {
			newMap[singleDomain] = &Filters{
				SiteId: getSiteIdBox(filters.SiteId),
			}
		}
	}

	return newMap
}

func FiltersToFiltersJson(mapa map[string]*Filters) map[string]*FiltersJson {
	newMap := make(map[string]*FiltersJson)
	for domain, filters := range mapa {
		domains := utils.SplitAndTrimKeys(domain)

		for _, singleDomain := range domains {
			newMap[singleDomain] = &FiltersJson{
				SiteId: &SiteIdBoxJson{
					Apply:       filters.SiteId.Apply,
					IsWhiteList: filters.SiteId.IsWhiteList,
					SiteIds:     MapKeysToSlice(filters.SiteId.SiteIds),
				},
			}
		}
	}
	return newMap
}

func MapKeysToSlice[K comparable, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func SliceToMapTrue[K comparable](slice []K) map[K]bool {
	m := make(map[K]bool, len(slice))
	for _, item := range slice {
		m[item] = true
	}
	return m
}

type FiltersJsonBox = map[string]*FiltersJson

func RewriteDspFiltersFileNextVer(
	filtersJson FiltersJsonBox,
	filtersFilename string,
) (
	map[string]*Filters,
	error,
) {
	fileData, err := json.MarshalIndent(filtersJson, "", "  ")
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to marshal data for file: %v", err)
	}

	err = os.WriteFile(filtersFilename, fileData, 0644)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to write file %s: %v", filtersFilename, err)
	}

	return FiltersJsonToFilters(filtersJson), nil
}
