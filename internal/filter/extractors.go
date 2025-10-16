package filter

import (
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_4"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

// StatelessV24BidRequestExtractor - stateless экстрактор для ORTB v2.4
type StatelessV24BidRequestExtractor struct{}

func NewStatelessV24BidRequestExtractor() *StatelessV24BidRequestExtractor {
	return &StatelessV24BidRequestExtractor{}
}

func (e *StatelessV24BidRequestExtractor) ExtractFieldValue(field FieldType, req interface{}) FieldValue {
	bidReq := req.(*ortb_V2_4.BidRequest)

	switch field {
	case FieldBidFloor:
		return e.extractBidFloor(bidReq)
	case FieldAppID:
		return e.extractAppID(bidReq)
	case FieldSiteID:
		return e.extractSiteID(bidReq)
	case FieldDeviceIP:
		return e.extractDeviceIP(bidReq)
	case FieldBannerWidth:
		return e.extractBannerWidth(bidReq)
	case FieldBannerHeight:
		return e.extractBannerHeight(bidReq)
	case FieldDeviceCountry, FieldCountry:
		return e.extractDeviceCountry(bidReq)
	default:
		return FieldValue{}
	}
}

func (e *StatelessV24BidRequestExtractor) extractBidFloor(req *ortb_V2_4.BidRequest) FieldValue {
	if req.Imp == nil {
		return NewFloatValue(0)
	}
	for i := range req.Imp {
		if req.Imp[i] != nil && req.Imp[i].BidFloor != nil {
			return NewFloatValue(float64(*req.Imp[i].BidFloor))
		}
	}
	return NewFloatValue(0)
}

func (e *StatelessV24BidRequestExtractor) extractAppID(req *ortb_V2_4.BidRequest) FieldValue {
	if req.App != nil && req.App.Id != nil {
		return NewStringValue(*req.App.Id)
	}
	return NewStringValue("")
}

func (e *StatelessV24BidRequestExtractor) extractSiteID(req *ortb_V2_4.BidRequest) FieldValue {
	if req.Site != nil && req.Site.Id != nil {
		return NewStringValue(*req.Site.Id)
	}
	return NewStringValue("")
}

func (e *StatelessV24BidRequestExtractor) extractDeviceIP(req *ortb_V2_4.BidRequest) FieldValue {
	if req.Device != nil && req.Device.Ip != nil {
		return NewStringValue(*req.Device.Ip)
	}
	return NewStringValue("")
}

func (e *StatelessV24BidRequestExtractor) extractBannerWidth(req *ortb_V2_4.BidRequest) FieldValue {
	if req.Imp != nil {
		for i := range req.Imp {
			if req.Imp[i] != nil && req.Imp[i].Banner != nil && req.Imp[i].Banner.W != nil {
				return NewIntValue(int(*req.Imp[i].Banner.W))
			}
		}
	}
	return NewIntValue(0)
}

func (e *StatelessV24BidRequestExtractor) extractBannerHeight(req *ortb_V2_4.BidRequest) FieldValue {
	if req.Imp != nil {
		for i := range req.Imp {
			if req.Imp[i] != nil && req.Imp[i].Banner != nil && req.Imp[i].Banner.H != nil {
				return NewIntValue(int(*req.Imp[i].Banner.H))
			}
		}
	}
	return NewIntValue(0)
}

func (e *StatelessV24BidRequestExtractor) extractDeviceCountry(req *ortb_V2_4.BidRequest) FieldValue {
	if req.Device != nil && req.Device.Geo != nil && req.Device.Geo.Country != nil {
		return NewStringValue(*req.Device.Geo.Country)
	}
	return NewStringValue("")
}

// StatelessV25BidRequestExtractor - stateless экстрактор для ORTB v2.5
type StatelessV25BidRequestExtractor struct{}

func NewStatelessV25BidRequestExtractor() *StatelessV25BidRequestExtractor {
	return &StatelessV25BidRequestExtractor{}
}

func (e *StatelessV25BidRequestExtractor) ExtractFieldValue(field FieldType, req interface{}) FieldValue {
	bidReq := req.(*ortb_V2_5.BidRequest)

	switch field {
	case FieldBidFloor:
		return e.extractBidFloor(bidReq)
	case FieldDeviceIP:
		return e.extractDeviceIP(bidReq)
	case FieldBannerWidth:
		return e.extractBannerWidth(bidReq)
	case FieldBannerHeight:
		return e.extractBannerHeight(bidReq)
	case FieldDeviceCountry:
		return e.extractDeviceCountry(bidReq)
	case FieldSitePage:
		return e.extractSitePage(bidReq)
	case FieldSiteDomain:
		return e.extractSiteDomain(bidReq)
	case FieldSitePublisherID:
		return e.extractSitePublisherID(bidReq)
	case FieldDeviceUA:
		return e.extractDeviceUA(bidReq)
	case FieldDeviceLanguage:
		return e.extractDeviceLanguage(bidReq)
	case FieldUserID:
		return e.extractUserID(bidReq)
	case FieldUserKeywords:
		return e.extractUserKeywords(bidReq)
	case FieldAuctionType:
		return e.extractAuctionType(bidReq)
	case FieldTMax:
		return e.extractTMax(bidReq)
	case FieldImpTagID:
		return e.extractImpTagID(bidReq)
	case FieldImpSecure:
		return e.extractImpSecure(bidReq)
	case FieldImpInstl:
		return e.extractImpInstl(bidReq)
	case FieldBidRequestID:
		return e.extractBidRequestID(bidReq)
	case FieldBidRequestAt:
		return e.extractBidRequestAt(bidReq)
	case FieldBidRequestTMax:
		return e.extractBidRequestTMax(bidReq)
	case FieldBidRequestCur:
		return e.extractBidRequestCur(bidReq)
	case FieldBidRequestBCat:
		return e.extractBidRequestBCat(bidReq)
	case FieldImpID:
		return e.extractImpID(bidReq)
	case FieldImpBidFloorCur:
		return e.extractImpBidFloorCur(bidReq)
	case FieldBannerBType:
		return e.extractBannerBType(bidReq)
	case FieldBannerBAttr:
		return e.extractBannerBAttr(bidReq)
	case FieldBannerPos:
		return e.extractBannerPos(bidReq)
	case FieldBannerMimes:
		return e.extractBannerMimes(bidReq)
	case FieldBannerExpDir:
		return e.extractBannerExpDir(bidReq)
	case FieldBannerAPI:
		return e.extractBannerAPI(bidReq)
	case FieldBannerID:
		return e.extractBannerID(bidReq)
	case FieldGeoLat:
		return e.extractGeoLat(bidReq)
	case FieldGeoLon:
		return e.extractGeoLon(bidReq)
	case FieldGeoRegion:
		return e.extractGeoRegion(bidReq)
	case FieldGeoCity:
		return e.extractGeoCity(bidReq)
	case FieldGeoZip:
		return e.extractGeoZip(bidReq)
	case FieldSiteName:
		return e.extractSiteName(bidReq)
	case FieldSiteRef:
		return e.extractSiteRef(bidReq)
	case FieldSiteCat:
		return e.extractSiteCat(bidReq)
	case FieldUserBuyerUID:
		return e.extractUserBuyerUID(bidReq)
	case FieldNativeRequest:
		return e.extractNativeRequestV25(bidReq)
	case FieldNativeVer:
		return e.extractNativeVerV25(bidReq)
	case FieldSiteID:
		return e.extractSiteID(bidReq)
	default:
		return FieldValue{}
	}
}

func (e *StatelessV25BidRequestExtractor) extractSiteID(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Site == nil {
		return NewStringValue("")
	}
	if req.Site.Id != nil {
		return NewStringValue(*req.Site.Id)
	}
	return NewStringValue("") // для exists: пустая строка == нет значения
}

func (e *StatelessV25BidRequestExtractor) extractNativeRequestV25(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Imp != nil {
		for _, imp := range req.Imp {
			if imp != nil && imp.Native != nil {
				if imp.Native.Request != nil {
					return NewStringValue(*imp.Native.Request)
				}
			}
		}
	}
	return NewStringValue("")
}

func (e *StatelessV25BidRequestExtractor) extractNativeVerV25(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Imp != nil {
		for _, imp := range req.Imp {
			if imp != nil && imp.Native != nil && imp.Native.Ver != nil {
				return NewStringValue(*imp.Native.Ver)
			}
		}
	}
	return NewStringValue("")
}

func (e *StatelessV25BidRequestExtractor) extractBidFloor(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Imp == nil {
		return NewFloatValue(0)
	}
	for i := range req.Imp {
		if req.Imp[i] != nil && req.Imp[i].BidFloor != nil {
			return NewFloatValue(float64(*req.Imp[i].BidFloor))
		}
	}
	return NewFloatValue(0)
}

func (e *StatelessV25BidRequestExtractor) extractDeviceIP(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Device == nil {
		return NewStringValue("")
	}
	if req.Device.Ip != nil {
		return NewStringValue(*req.Device.Ip)
	}
	return NewStringValue("")
}

func (e *StatelessV25BidRequestExtractor) extractBannerWidth(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Imp != nil {
		for i := range req.Imp {
			if req.Imp[i] != nil && req.Imp[i].Banner != nil && req.Imp[i].Banner.W != nil {
				return NewIntValue(int(*req.Imp[i].Banner.W))
			}
		}
	}
	return NewIntValue(0)
}

func (e *StatelessV25BidRequestExtractor) extractBannerHeight(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Imp != nil {
		for i := range req.Imp {
			if req.Imp[i] != nil && req.Imp[i].Banner != nil && req.Imp[i].Banner.H != nil {
				return NewIntValue(int(*req.Imp[i].Banner.H))
			}
		}
	}
	return NewIntValue(0)
}

func (e *StatelessV25BidRequestExtractor) extractDeviceCountry(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Device != nil && req.Device.Geo != nil && req.Device.Geo.Country != nil {
		return NewStringValue(*req.Device.Geo.Country)
	}
	return NewStringValue("")
}

// ДОБАВЛЕНО: Методы извлечения для новых полей v2.4
func (e *StatelessV25BidRequestExtractor) extractSitePage(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Site != nil && req.Site.Page != nil {
		return NewStringValue(*req.Site.Page)
	}
	return NewStringValue("")
}

func (e *StatelessV25BidRequestExtractor) extractSiteDomain(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Site != nil && req.Site.Domain != nil {
		return NewStringValue(*req.Site.Domain)
	}
	return NewStringValue("")
}

func (e *StatelessV25BidRequestExtractor) extractSitePublisherID(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Site != nil && req.Site.Publisher != nil && req.Site.Publisher.Id != nil {
		return NewStringValue(*req.Site.Publisher.Id)
	}
	return NewStringValue("")
}

func (e *StatelessV25BidRequestExtractor) extractDeviceUA(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Device == nil {
		return NewStringValue("")
	}
	if req.Device.Ua != nil {
		return NewStringValue(*req.Device.Ua)
	}
	return NewStringValue("")
}

func (e *StatelessV25BidRequestExtractor) extractDeviceLanguage(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Device != nil && req.Device.Language != nil {
		return NewStringValue(*req.Device.Language)
	}
	return NewStringValue("")
}

func (e *StatelessV25BidRequestExtractor) extractUserID(req *ortb_V2_5.BidRequest) FieldValue {
	if req.User != nil && req.User.Id != nil {
		return NewStringValue(*req.User.Id)
	}
	return NewStringValue("")
}

func (e *StatelessV25BidRequestExtractor) extractUserKeywords(req *ortb_V2_5.BidRequest) FieldValue {
	if req.User != nil && req.User.Keywords != nil {
		return NewStringValue(*req.User.Keywords)
	}
	return NewStringValue("")
}

func (e *StatelessV25BidRequestExtractor) extractAuctionType(req *ortb_V2_5.BidRequest) FieldValue {
	if req.At != nil {
		return NewIntValue(int(*req.At))
	}
	return NewIntValue(0) // default value
}

func (e *StatelessV25BidRequestExtractor) extractTMax(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Tmax != nil {
		return NewIntValue(int(*req.Tmax))
	}
	return NewIntValue(0)
}

func (e *StatelessV25BidRequestExtractor) extractImpTagID(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Imp != nil {
		for i := range req.Imp {
			if req.Imp[i] != nil && req.Imp[i].Tagid != nil {
				return NewStringValue(*req.Imp[i].Tagid)
			}
		}
	}
	return NewStringValue("")
}

func (e *StatelessV25BidRequestExtractor) extractImpSecure(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Imp != nil {
		for i := range req.Imp {
			if req.Imp[i] != nil && req.Imp[i].Secure != nil {
				return NewIntValue(int(*req.Imp[i].Secure))
			}
		}
	}
	return NewIntValue(0)
}

func (e *StatelessV25BidRequestExtractor) extractImpInstl(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Imp != nil {
		for i := range req.Imp {
			if req.Imp[i] != nil && req.Imp[i].Instl != nil {
				return NewIntValue(int(*req.Imp[i].Instl))
			}
		}
	}
	return NewIntValue(0)
}

func (e *StatelessV25BidRequestExtractor) extractBidRequestID(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Id != nil {
		return NewStringValue(*req.Id)
	}
	return NewStringValue("")
}

func (e *StatelessV25BidRequestExtractor) extractBidRequestAt(req *ortb_V2_5.BidRequest) FieldValue {
	if req.At != nil {
		return NewIntValue(int(*req.At))
	}
	return NewIntValue(2) // default
}

func (e *StatelessV25BidRequestExtractor) extractBidRequestTMax(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Tmax != nil {
		return NewIntValue(int(*req.Tmax))
	}
	return NewIntValue(0)
}

func (e *StatelessV25BidRequestExtractor) extractBidRequestCur(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Cur != nil && len(req.Cur) > 0 {
		return NewStringValue(req.Cur[0]) // берем первую валюту
	}
	return NewStringValue("")
}

func (e *StatelessV25BidRequestExtractor) extractBidRequestBCat(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Bcat != nil && len(req.Bcat) > 0 {
		return NewStringValue(req.Bcat[0]) // берем первую категорию
	}
	return NewStringValue("")
}

func (e *StatelessV25BidRequestExtractor) extractImpID(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Imp != nil {
		for i := range req.Imp {
			if req.Imp[i] != nil && req.Imp[i].Id != nil {
				return NewStringValue(*req.Imp[i].Id)
			}
		}
	}
	return NewStringValue("")
}

func (e *StatelessV25BidRequestExtractor) extractImpBidFloorCur(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Imp != nil {
		for i := range req.Imp {
			if req.Imp[i] != nil && req.Imp[i].Bidfloorcur != nil {
				return NewStringValue(*req.Imp[i].Bidfloorcur)
			}
		}
	}
	return NewStringValue("USD") // default
}

func (e *StatelessV25BidRequestExtractor) extractBannerBType(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Imp != nil {
		for i := range req.Imp {
			if req.Imp[i] != nil && req.Imp[i].Banner != nil && req.Imp[i].Banner.Btype != nil && len(req.Imp[i].Banner.Btype) > 0 {
				return NewIntValue(int(req.Imp[i].Banner.Btype[0]))
			}
		}
	}
	return NewIntValue(0)
}

func (e *StatelessV25BidRequestExtractor) extractBannerBAttr(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Imp != nil {
		for i := range req.Imp {
			if req.Imp[i] != nil && req.Imp[i].Banner != nil && req.Imp[i].Banner.Battr != nil && len(req.Imp[i].Banner.Battr) > 0 {
				return NewIntValue(int(req.Imp[i].Banner.Battr[0]))
			}
		}
	}
	return NewIntValue(0)
}

func (e *StatelessV25BidRequestExtractor) extractBannerPos(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Imp != nil {
		for i := range req.Imp {
			if req.Imp[i] != nil && req.Imp[i].Banner != nil && req.Imp[i].Banner.Pos != nil {
				return NewIntValue(int(*req.Imp[i].Banner.Pos))
			}
		}
	}
	return NewIntValue(0)
}

func (e *StatelessV25BidRequestExtractor) extractBannerMimes(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Imp != nil {
		for i := range req.Imp {
			if req.Imp[i] != nil && req.Imp[i].Banner != nil && req.Imp[i].Banner.Mimes != nil && len(req.Imp[i].Banner.Mimes) > 0 {
				return NewStringValue(req.Imp[i].Banner.Mimes[0])
			}
		}
	}
	return NewStringValue("")
}

func (e *StatelessV25BidRequestExtractor) extractBannerExpDir(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Imp != nil {
		for i := range req.Imp {
			if req.Imp[i] != nil && req.Imp[i].Banner != nil && req.Imp[i].Banner.Expdir != nil && len(req.Imp[i].Banner.Expdir) > 0 {
				return NewIntValue(int(req.Imp[i].Banner.Expdir[0]))
			}
		}
	}
	return NewIntValue(0)
}

func (e *StatelessV25BidRequestExtractor) extractBannerAPI(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Imp != nil {
		for i := range req.Imp {
			if req.Imp[i] != nil && req.Imp[i].Banner != nil && req.Imp[i].Banner.Api != nil && len(req.Imp[i].Banner.Api) > 0 {
				return NewIntValue(int(req.Imp[i].Banner.Api[0]))
			}
		}
	}
	return NewIntValue(0)
}

func (e *StatelessV25BidRequestExtractor) extractBannerID(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Imp != nil {
		for i := range req.Imp {
			if req.Imp[i] != nil && req.Imp[i].Banner != nil && req.Imp[i].Banner.Id != nil {
				return NewStringValue(*req.Imp[i].Banner.Id)
			}
		}
	}
	return NewStringValue("")
}

func (e *StatelessV25BidRequestExtractor) extractGeoLat(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Device != nil && req.Device.Geo != nil && req.Device.Geo.Lat != nil {
		return NewFloatValue(float64(*req.Device.Geo.Lat))
	}
	return NewFloatValue(0)
}

func (e *StatelessV25BidRequestExtractor) extractGeoLon(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Device != nil && req.Device.Geo != nil && req.Device.Geo.Lon != nil {
		return NewFloatValue(float64(*req.Device.Geo.Lon))
	}
	return NewFloatValue(0)
}

func (e *StatelessV25BidRequestExtractor) extractGeoRegion(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Device != nil && req.Device.Geo != nil && req.Device.Geo.Region != nil {
		return NewStringValue(*req.Device.Geo.Region)
	}
	return NewStringValue("")
}

func (e *StatelessV25BidRequestExtractor) extractGeoCity(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Device != nil && req.Device.Geo != nil && req.Device.Geo.City != nil {
		return NewStringValue(*req.Device.Geo.City)
	}
	return NewStringValue("")
}

func (e *StatelessV25BidRequestExtractor) extractGeoZip(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Device != nil && req.Device.Geo != nil && req.Device.Geo.Zip != nil {
		return NewStringValue(*req.Device.Geo.Zip)
	}
	return NewStringValue("")
}

func (e *StatelessV25BidRequestExtractor) extractSiteName(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Site != nil && req.Site.Name != nil {
		return NewStringValue(*req.Site.Name)
	}
	return NewStringValue("")
}

func (e *StatelessV25BidRequestExtractor) extractSiteRef(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Site != nil && req.Site.Ref != nil {
		return NewStringValue(*req.Site.Ref)
	}
	return NewStringValue("")
}

func (e *StatelessV25BidRequestExtractor) extractSiteCat(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Site != nil && req.Site.Cat != nil && len(req.Site.Cat) > 0 {
		return NewStringValue(req.Site.Cat[0])
	}
	return NewStringValue("")
}

func (e *StatelessV25BidRequestExtractor) extractUserBuyerUID(req *ortb_V2_5.BidRequest) FieldValue {
	if req.User != nil && req.User.Buyeruid != nil {
		return NewStringValue(*req.User.Buyeruid)
	}
	return NewStringValue("")
}

type StatelessV24BidResponseExtractor struct{}

func NewStatelessV24BidResponseExtractor() *StatelessV24BidResponseExtractor {
	return &StatelessV24BidResponseExtractor{}
}

func (e *StatelessV24BidResponseExtractor) ExtractFieldValue(field FieldType, resp interface{}) FieldValue {
	bidResp := resp.(*ortb_V2_4.BidResponse)

	switch field {
	case FieldBidPrice:
		return e.extractBidPrice(bidResp)
	case FieldBidID:
		return e.extractBidID(bidResp)
	case FieldBidAdID:
		return e.extractBidAdID(bidResp)
	case FieldBidImpID:
		return e.extractBidImpID(bidResp)
	case FieldBidArray:
		return e.extractBidArray(bidResp)
	case FieldBidNurl:
		return e.extractBidNurl(bidResp)
	case FieldBidBurl:
		return e.extractBidBurl(bidResp)
	default:
		return FieldValue{}
	}
}

func (e *StatelessV24BidResponseExtractor) extractBidPrice(resp *ortb_V2_4.BidResponse) FieldValue {
	if resp != nil && resp.Seatbid != nil && resp.Seatbid.Bid != nil {
		for i := range resp.Seatbid.Bid {
			if resp.Seatbid.Bid[i].Price != nil {
				return NewFloatValue(float64(*resp.Seatbid.Bid[i].Price))
			}
		}
	}
	return NewFloatValue(0)
}

func (e *StatelessV24BidResponseExtractor) extractBidID(resp *ortb_V2_4.BidResponse) FieldValue {
	if resp != nil && resp.Seatbid != nil && resp.Seatbid.Bid != nil {
		for i := range resp.Seatbid.Bid {
			if resp.Seatbid.Bid[i].Id != nil {
				return NewStringValue(*resp.Seatbid.Bid[i].Id)
			}
		}
	}
	return NewStringValue("")
}

func (e *StatelessV24BidResponseExtractor) extractBidAdID(resp *ortb_V2_4.BidResponse) FieldValue {
	if resp == nil || resp.Seatbid == nil || resp.Seatbid.Bid == nil || len(resp.Seatbid.Bid) == 0 {
		return NewStringValue("")
	}

	// Требуем, чтобы у каждого элемента был непустой Adid
	for i := range resp.Seatbid.Bid {
		b := resp.Seatbid.Bid[i]
		if b == nil || b.Adid == nil || *b.Adid == "" {
			return NewStringValue("") // ломаем "exists", если у любого элемента нет adid
		}
	}

	// Все элементы ок — можно вернуть первый (для exists/equals этого достаточно)
	return NewStringValue(*resp.Seatbid.Bid[0].Adid)
}

func (e *StatelessV24BidResponseExtractor) extractBidImpID(resp *ortb_V2_4.BidResponse) FieldValue {
	if resp != nil && resp.Seatbid != nil && resp.Seatbid.Bid != nil {
		for i := range resp.Seatbid.Bid {
			if resp.Seatbid.Bid[i].Impid != nil {
				return NewStringValue(*resp.Seatbid.Bid[i].Impid)
			}
		}
	}
	return NewStringValue("")
}

func (e *StatelessV24BidResponseExtractor) extractBidArray(resp *ortb_V2_4.BidResponse) FieldValue {
	if resp != nil && resp.Seatbid != nil && resp.Seatbid.Bid != nil {
		return NewStringValue("exists")
	}
	return NewStringValue("")
}

func (e *StatelessV24BidResponseExtractor) extractBidNurl(resp *ortb_V2_4.BidResponse) FieldValue {
	if resp != nil && resp.Seatbid != nil && resp.Seatbid.Bid != nil {
		for i := range resp.Seatbid.Bid {
			if resp.Seatbid.Bid[i].Nurl != nil {
				return NewStringValue(*resp.Seatbid.Bid[i].Nurl)
			}
		}
	}
	return NewStringValue("")
}

func (e *StatelessV24BidResponseExtractor) extractBidBurl(resp *ortb_V2_4.BidResponse) FieldValue {
	if resp != nil && resp.Seatbid != nil && resp.Seatbid.Bid != nil {
		for i := range resp.Seatbid.Bid {
			if resp.Seatbid.Bid[i].Burl != nil {
				return NewStringValue(*resp.Seatbid.Bid[i].Burl)
			}
		}
	}
	return NewStringValue("")
}

// StatelessV25BidResponseExtractor - stateless экстрактор для BidResponse v2.5
type StatelessV25BidResponseExtractor struct{}

func NewStatelessV25BidResponseExtractor() *StatelessV25BidResponseExtractor {
	return &StatelessV25BidResponseExtractor{}
}

func (e *StatelessV25BidResponseExtractor) ExtractFieldValue(field FieldType, resp interface{}) FieldValue {
	bidResp := resp.(*ortb_V2_5.BidResponse)

	switch field {
	case FieldBidAdID:
		return e.extractBidField(bidResp, func(bid *ortb_V2_5.Bid) *string {
			return bid.Adid
		})
	case FieldBidNurl:
		return e.extractBidField(bidResp, func(bid *ortb_V2_5.Bid) *string {
			return bid.Nurl
		})
	case FieldBidAdm:
		return e.extractBidField(bidResp, func(bid *ortb_V2_5.Bid) *string {
			return bid.Adm
		})
	default:
		return FieldValue{}
	}
}

func (e *StatelessV25BidResponseExtractor) extractBidField(
	resp *ortb_V2_5.BidResponse,
	fieldExtractor func(*ortb_V2_5.Bid) *string,
) FieldValue {
	if resp == nil || resp.Seatbid == nil || len(resp.Seatbid) == 0 {
		return NewStringValue("")
	}

	// Проверяем, есть ли вообще любые биды
	hasAnyBids := false
	for _, seatbid := range resp.Seatbid {
		if seatbid.Bid != nil && len(seatbid.Bid) > 0 {
			hasAnyBids = true
			break
		}
	}

	if !hasAnyBids {
		return NewStringValue("")
	}

	// Проверяем, что у ВСЕХ бидов во ВСЕХ seatbid есть нужное поле
	for _, seatbid := range resp.Seatbid {
		if seatbid.Bid == nil {
			continue
		}
		for _, bid := range seatbid.Bid {
			if bid == nil {
				return NewStringValue("")
			}
			fieldValue := fieldExtractor(bid)
			if fieldValue == nil || *fieldValue == "" {
				return NewStringValue("") // ломаем "exists", если у любого элемента нет поля
			}
		}
	}

	// Все элементы ок — можно вернуть первый (для exists/equals этого достаточно)
	for _, seatbid := range resp.Seatbid {
		if seatbid.Bid != nil && len(seatbid.Bid) > 0 {
			fieldValue := fieldExtractor(seatbid.Bid[0])
			if fieldValue != nil {
				return NewStringValue(*fieldValue)
			}
		}
	}

	return NewStringValue("")
}
