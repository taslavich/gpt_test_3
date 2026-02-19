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

type CidIdBox struct {
	Apply       bool
	IsWhiteList bool
	CidIds      map[string]bool
}

func NewCidIdBox(apply, isWhiteList bool, cidIds []string) *CidIdBox {
	var newCidIdMap map[string]bool
	if apply {
		newCidIdMap = make(map[string]bool)
		for i := range cidIds {
			newCidIdMap[cidIds[i]] = true
		}
	}

	return &CidIdBox{
		CidIds:      newCidIdMap,
		IsWhiteList: isWhiteList,
		Apply:       apply,
	}
}

func (s *CidIdBox) Allowed(bidResponse *ortb_V2_5.BidResponse) bool {
	if s == nil {
		return true
	}

	if !s.Apply {
		return true
	}

	if bidResponse == nil || bidResponse.Seatbid == nil {
		return true
	}

	for i := range bidResponse.Seatbid {
		seatbid := bidResponse.Seatbid[i]
		if seatbid == nil {
			continue
		}
		for j := range bidResponse.Seatbid[i].Bid {
			bid := bidResponse.Seatbid[i].Bid[j]
			if bid == nil {
				continue
			}

			cidId := bid.GetCid()
			var check bool
			if s.IsWhiteList {
				check = s.CidIds[cidId]
			} else {
				check = !s.CidIds[cidId]
			}

			if check == false {
				bidResponse.Seatbid[i].Bid[j] = nil
			}
		}
	}

	thereIs := false
	for i := range bidResponse.Seatbid {
		seatbid := bidResponse.Seatbid[i]
		if seatbid == nil {
			continue
		}
		for j := range bidResponse.Seatbid[i].Bid {
			bid := bidResponse.Seatbid[i].Bid[j]
			if bid == nil {
				continue
			}

			thereIs = true
		}
	}

	return thereIs
}

type CidIdBoxJson struct {
	Apply       bool     `json:"apply"`
	IsWhiteList bool     `json:"isWhiteList"`
	CidIds      []string `json:"cidIds"`
}

func getCidIdBox(cidIdBoxJson *CidIdBoxJson) *CidIdBox {
	if cidIdBoxJson == nil {
		return &CidIdBox{
			Apply:       false,
			IsWhiteList: false,
			CidIds:      map[string]bool{},
		}
	}

	return &CidIdBox{
		Apply:       cidIdBoxJson.Apply,
		IsWhiteList: cidIdBoxJson.IsWhiteList,
		CidIds:      SliceToMapTrue(cidIdBoxJson.CidIds),
	}
}

func FiltersCidJsonToFiltersCid(mapa FilterCidJsonBoxType) FilterCidBoxType {
	newMap := make(FilterCidBoxType)
	for sspKey, dsps := range mapa {

		sspKeys := utils.SplitAndTrimKeys(sspKey)
		for _, singleSspDomain := range sspKeys {
			if newMap[singleSspDomain] == nil {
				newMap[singleSspDomain] = make(map[string]*CidIdBox)
			}

			for dspKey, filterCid := range dsps {

				dspKeys := utils.SplitAndTrimKeys(dspKey)
				for _, singleDspDomain := range dspKeys {
					if newMap[singleSspDomain][singleDspDomain] == nil {
						newMap[singleSspDomain][singleDspDomain] = getCidIdBox(filterCid)
					}
				}
			}
		}

	}

	return newMap
}

type FilterCidJsonBoxType = map[string]map[string]*CidIdBoxJson
type FilterCidBoxType = map[string]map[string]*CidIdBox

func RewriteDspFiltersCidFileNextVer(
	filtersCidJson FilterCidJsonBoxType,
	filtersCidFilename string,
) (
	FilterCidBoxType,
	error,
) {
	fileData, err := json.MarshalIndent(filtersCidJson, "", "  ")
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to marshal data for file: %v", err)
	}

	err = os.WriteFile(filtersCidFilename, fileData, 0644)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to write file %s: %v", filtersCidFilename, err)
	}

	return FiltersCidJsonToFiltersCid(filtersCidJson), nil
}

func GetValueFomCidMap(bidResponse *ortb_V2_5.BidResponse, ssp, dsp string, valueMap FilterCidBoxType) bool {
	if valueMap == nil {
		return true
	}

	sspAll := []string{ssp, utils.ALL}
	dspAll := []string{dsp, utils.ALL}

	for i := range sspAll {
		if dspMap, ok := valueMap[sspAll[i]]; ok {
			for k := range dspAll {
				if mainObj, ok := dspMap[dspAll[k]]; ok {
					return mainObj.Allowed(bidResponse)
				}
			}
		}
	}

	return true
}

func InitCidSspDspMap(filename string) (FilterCidBoxType, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filename, err)
	}

	tmp := make(FilterCidJsonBoxType)

	err = json.Unmarshal(data, &tmp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSON from %s: %w", filename, err)
	}

	return FiltersCidJsonToFiltersCid(tmp), nil
}
