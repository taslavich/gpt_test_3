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

type SiteIdBoxChanger struct {
	ToChange bool   `json:"toChange"`
	SiteId   string `json:"siteId"`
}

func NewSiteIdBoxChanger(ToChange bool, siteId string) *SiteIdBoxChanger {
	return &SiteIdBoxChanger{
		SiteId:   siteId,
		ToChange: ToChange,
	}
}

func (s *SiteIdBoxChanger) Change(bidRequest *ortb_V2_5.BidRequest) (*ortb_V2_5.BidRequest, bool) {
	if s == nil || !s.ToChange || bidRequest == nil || bidRequest.Site == nil {
		return bidRequest, false
	}

	newBidRequest := &ortb_V2_5.BidRequest{
		Id:     bidRequest.Id,
		At:     bidRequest.At,
		Imp:    bidRequest.Imp,
		Device: bidRequest.Device,
		User:   bidRequest.User,
		Tmax:   bidRequest.Tmax,
		Cur:    bidRequest.Cur,
		Bcat:   bidRequest.Bcat,
	}

	newBidRequest.Site = &ortb_V2_5.Site{
		Id:        &s.SiteId,
		Name:      bidRequest.Site.Name,
		Page:      bidRequest.Site.Page,
		Domain:    bidRequest.Site.Domain,
		Ref:       bidRequest.Site.Ref,
		Cat:       bidRequest.Site.Cat,
		Publisher: bidRequest.Site.Publisher,
	}

	return newBidRequest, true
}

//----------------------------------------------------------------

type ChangersChanger struct {
	SiteIdBoxes map[string]*SiteIdBoxChanger `json:"siteIdBoxes"`
	Apply       bool                         `json:"apply"`
}

// expandGroupedKeys is shared by the site.id and site.domain changer maps.
// Both accept comma-separated DSP names and site_id keys in their JSON.
func expandGroupedKeys[T any](grouped map[string]T) map[string]T {
	expanded := make(map[string]T)
	for key, value := range grouped {
		for _, name := range utils.SplitAndTrimKeys(key) {
			expanded[name] = value
		}
	}
	return expanded
}

func (f *ChangersChanger) ToMany() *ChangersChanger {
	return &ChangersChanger{
		Apply:       f.Apply,
		SiteIdBoxes: expandGroupedKeys(f.SiteIdBoxes),
	}
}

func (f *ChangersChanger) Change(bidRequest *ortb_V2_5.BidRequest) (*ortb_V2_5.BidRequest, bool) {
	if !f.Apply {
		return bidRequest, false
	}

	if siteIdBox, ok := f.SiteIdBoxes[bidRequest.Site.GetId()]; ok {
		return siteIdBox.Change(bidRequest)
	}

	if siteIdBox, ok := f.SiteIdBoxes["ALL"]; ok {
		return siteIdBox.Change(bidRequest)
	}

	return bidRequest, false
}

//-------------------------------------------------------------------

type ChangersBoxChanger struct {
	Changers map[string]*ChangersChanger
}

func NewChangersBoxChanger(filename string) (*ChangersBoxChanger, error) {

	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filename, err)
	}

	var tmp map[string]*ChangersChanger

	err = json.Unmarshal(data, &tmp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSON from %s: %w", filename, err)
	}

	return &ChangersBoxChanger{
		Changers: ToManyChangersMap(tmp),
	}, nil
}

func (f *ChangersBoxChanger) Change(bidRequest *ortb_V2_5.BidRequest, domain string) (*ortb_V2_5.BidRequest, bool) {
	if f == nil || bidRequest == nil || bidRequest.Site == nil || bidRequest.Site.Id == nil || len(f.Changers) == 0 {
		return bidRequest, false
	}

	filters, ok := f.Changers[domain]
	if ok {
		if filters.Apply {
			return filters.Change(bidRequest)
		}
	}

	filtersALL, ok := f.Changers["ALL"]
	if ok {
		if filtersALL.Apply {
			return filtersALL.Change(bidRequest)
		}
	}

	return bidRequest, false
}

//------------------------------------------------------------------

func RewriteChangersFile(
	changers ChangerType,
	changersFilename string,
) (
	ChangerType,
	error,
) {
	fmt.Println("In Rewrite")
	fileData, err := json.MarshalIndent(changers, "", "  ")
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to marshal data for file: %v", err)
	}
	fmt.Println("Finished Marshal")

	err = os.WriteFile(changersFilename, fileData, 0644)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to write file %s: %v", changersFilename, err)
	}
	fmt.Println("Finished Writing to file")

	return ToManyChangersMap(changers), nil
}

func ToManyChangersMap(changers map[string]*ChangersChanger) map[string]*ChangersChanger {
	grouped := make(map[string]*ChangersChanger, len(changers))
	for key, val := range changers {
		grouped[key] = val.ToMany()
	}
	fmt.Println("Finished ToManyChangersMap")
	return expandGroupedKeys(grouped)
}

type ChangerType map[string]*ChangersChanger
