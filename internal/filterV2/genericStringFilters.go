package filter

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"

	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type CampaignFiltersBoxes struct {
	Country    *FiltersBox
	Language   *FiltersBox
	DeviceType *FiltersBox
	Os         *FiltersBox
	Browser    *FiltersBox
	SiteID     *FiltersBox
	IP         *FiltersBox
}

func GetFiltersFromJSONB(raw []byte) (*Filters, error) {
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "{}" {
		return NewFilters(false, false, nil), nil
	}

	var filtersJson FiltersJson
	if err := json.Unmarshal(raw, &filtersJson); err != nil {
		return nil, err
	}

	apply := len(filtersJson.Objects) > 0

	return NewFilters(
		apply,
		filtersJson.IsWhiteList,
		filtersJson.Objects,
	), nil
}

func GetCampaignFiltersBoxesFromPostgres(
	ctx context.Context,
	db *sql.DB,
) (*CampaignFiltersBoxes, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT
			campaign_id,
			country,
			language,
			device_type,
			os,
			browser,
			site_id,
			ip
		FROM campaigns
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query campaign filters: %w", err)
	}
	defer rows.Close()

	countryAllowers := make(map[string]*Filters)
	languageAllowers := make(map[string]*Filters)
	deviceTypeAllowers := make(map[string]*Filters)
	osAllowers := make(map[string]*Filters)
	browserAllowers := make(map[string]*Filters)
	siteIDAllowers := make(map[string]*Filters)
	ipAllowers := make(map[string]*Filters)

	for rows.Next() {
		var campaignID string

		var countryRaw []byte
		var languageRaw []byte
		var deviceTypeRaw []byte
		var osRaw []byte
		var browserRaw []byte
		var siteIDRaw []byte
		var ipRaw []byte

		if err = rows.Scan(
			&campaignID,
			&countryRaw,
			&languageRaw,
			&deviceTypeRaw,
			&osRaw,
			&browserRaw,
			&siteIDRaw,
			&ipRaw,
		); err != nil {
			return nil, fmt.Errorf("failed to scan campaign filters row: %w", err)
		}

		countryAllowers[campaignID], err = GetFiltersFromJSONB(countryRaw)
		if err != nil {
			return nil, fmt.Errorf("failed to parse country filter for campaign_id=%s: %w", campaignID, err)
		}

		languageAllowers[campaignID], err = GetFiltersFromJSONB(languageRaw)
		if err != nil {
			return nil, fmt.Errorf("failed to parse language filter for campaign_id=%s: %w", campaignID, err)
		}

		deviceTypeAllowers[campaignID], err = GetFiltersFromJSONB(deviceTypeRaw)
		if err != nil {
			return nil, fmt.Errorf("failed to parse device_type filter for campaign_id=%s: %w", campaignID, err)
		}

		osAllowers[campaignID], err = GetFiltersFromJSONB(osRaw)
		if err != nil {
			return nil, fmt.Errorf("failed to parse os filter for campaign_id=%s: %w", campaignID, err)
		}

		browserAllowers[campaignID], err = GetFiltersFromJSONB(browserRaw)
		if err != nil {
			return nil, fmt.Errorf("failed to parse browser filter for campaign_id=%s: %w", campaignID, err)
		}

		siteIDAllowers[campaignID], err = GetFiltersFromJSONB(siteIDRaw)
		if err != nil {
			return nil, fmt.Errorf("failed to parse site_id filter for campaign_id=%s: %w", campaignID, err)
		}

		ipAllowers[campaignID], err = GetFiltersFromJSONB(ipRaw)
		if err != nil {
			return nil, fmt.Errorf("failed to parse ip filter for campaign_id=%s: %w", campaignID, err)
		}
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate campaign filters rows: %w", err)
	}

	return &CampaignFiltersBoxes{
		Country: &FiltersBox{
			Name:     "country",
			Allowers: countryAllowers,
		},
		Language: &FiltersBox{
			Name:     "language",
			Allowers: languageAllowers,
		},
		DeviceType: &FiltersBox{
			Name:     "device_type",
			Allowers: deviceTypeAllowers,
		},
		Os: &FiltersBox{
			Name:     "os",
			Allowers: osAllowers,
		},
		Browser: &FiltersBox{
			Name:     "browser",
			Allowers: browserAllowers,
		},
		SiteID: &FiltersBox{
			Name:     "site_id",
			Allowers: siteIDAllowers,
		},
		IP: &FiltersBox{
			Name:     "ip",
			Allowers: ipAllowers,
		},
	}, nil
}

const CommonFilterDomainKey = "ALL"

type Filters struct {
	Apply       bool            `json:"apply"`
	IsWhiteList bool            `json:"isWhiteList"`
	Objects     map[string]bool `json:"-"`
}

type FiltersJson struct {
	IsWhiteList bool     `json:"isWhiteList"`
	Objects     []string `json:"objects"`
}

type FiltersJsonBox = map[string]*FiltersJson

type FiltersBox struct {
	Name     string
	Allowers map[string]*Filters
}

func NewFilters(apply, isWhiteList bool, objects []string) *Filters {
	var objectsMap map[string]bool
	if apply {
		objectsMap = SliceToMapTrue(objects)
	}

	return &Filters{
		Apply:       apply,
		IsWhiteList: isWhiteList,
		Objects:     objectsMap,
	}
}

func (s *Filters) Allowed(object string) bool {
	if s == nil {
		return true
	}

	if !s.Apply {
		return true
	}

	if s.IsWhiteList {
		return s.Objects[object]
	}

	return !s.Objects[object]
}

func (f *FiltersBox) Allowed(object string, campaignID string, all bool) bool {
	if f == nil {
		return true
	}

	if all {
		if filtersALL, ok := f.Allowers[CommonFilterDomainKey]; ok {
			return filtersALL.Allowed(object)
		}
		return true
	}

	if filters, ok := f.Allowers[campaignID]; ok {
		return filters.Allowed(object)
	}

	return true
}

func getFilters(filtersJson *FiltersJson) *Filters {
	if filtersJson == nil {
		return &Filters{
			Apply:       false,
			IsWhiteList: false,
			Objects:     map[string]bool{},
		}
	}

	return &Filters{
		Apply:       true,
		IsWhiteList: filtersJson.IsWhiteList,
		Objects:     SliceToMapTrue(filtersJson.Objects),
	}
}

func FiltersJsonToFilters(mapa FiltersJsonBox) map[string]*Filters {
	newMap := make(map[string]*Filters)

	for domain, filters := range mapa {
		domains := utils.SplitAndTrimKeys(domain)

		for _, singleDomain := range domains {
			newMap[singleDomain] = getFilters(filters)
		}
	}

	return newMap
}

func FiltersToFiltersJson(mapa map[string]*Filters) map[string]*FiltersJson {
	newMap := make(map[string]*FiltersJson)

	for domain, filters := range mapa {
		domains := utils.SplitAndTrimKeys(domain)

		for _, singleDomain := range domains {
			if filters == nil {
				newMap[singleDomain] = &FiltersJson{
					IsWhiteList: false,
					Objects:     []string{},
				}
				continue
			}

			newMap[singleDomain] = &FiltersJson{
				IsWhiteList: filters.IsWhiteList,
				Objects:     MapKeysToSlice(filters.Objects),
			}
		}
	}

	return newMap
}

func RewriteDspFiltersFileNextVer(
	filtersJson FiltersJsonBox,
	filtersFilename string,
	name ...string,
) (
	map[string]*Filters,
	error,
) {
	fileData, err := json.MarshalIndent(filtersJson, "", "  ")
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to marshal %s filters data for file: %v", filterName(name...), err)
	}

	err = os.WriteFile(filtersFilename, fileData, 0644)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to write %s filters file %s: %v", filterName(name...), filtersFilename, err)
	}

	return FiltersJsonToFilters(filtersJson), nil
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

func filterName(name ...string) string {
	if len(name) == 0 || name[0] == "" {
		return "string"
	}

	return name[0]
}
