package filter

import (
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

// StatelessV25BidRequestExtractor - stateless экстрактор для ORTB v2.5
type StatelessV25BidRequestExtractor struct{}

func NewStatelessV25BidRequestExtractor() *StatelessV25BidRequestExtractor {
	return &StatelessV25BidRequestExtractor{}
}

func (e *StatelessV25BidRequestExtractor) ExtractFieldValue(field FieldType, req interface{}) FieldValue {
	bidReq := req.(*ortb_V2_5.BidRequest)

	switch field {
	case FieldBidRequestID:
		return e.extractBidRequestID(bidReq)
	case FieldDeviceIP:
		return e.extractDeviceIP(bidReq)
	case FieldSitePage:
		return e.extractSitePage(bidReq)
	case FieldSiteDomain:
		return e.extractSiteDomain(bidReq)
	case FieldDeviceUA:
		return e.extractDeviceUA(bidReq)
	case FieldDeviceLanguage:
		return e.extractDeviceLanguage(bidReq)
	case FieldUserID:
		return e.extractUserID(bidReq)
	case FieldAuctionType:
		return e.extractAuctionType(bidReq)
	case FieldImpID:
		return e.extractImpID(bidReq)
	case FieldSiteID:
		return e.extractSiteID(bidReq)
	case FieldBidRequestCur:
		return e.extractBidRequestCur(bidReq)
	default:
		return FieldValue{}
	}
}

func (e *StatelessV25BidRequestExtractor) extractBidRequestCur(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Cur != nil {
		return NewStringValue(*req.Id)
	}
	return NewStringValue("")
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

func (e *StatelessV25BidRequestExtractor) extractDeviceIP(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Device == nil {
		return NewStringValue("")
	}
	if req.Device.Ip != nil {
		return NewStringValue(*req.Device.Ip)
	}
	return NewStringValue("")
}

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

func (e *StatelessV25BidRequestExtractor) extractAuctionType(req *ortb_V2_5.BidRequest) FieldValue {
	if req.At != nil {
		return NewIntValue(int(*req.At))
	}
	return NewIntValue(0) // default value
}

func (e *StatelessV25BidRequestExtractor) extractBidRequestID(req *ortb_V2_5.BidRequest) FieldValue {
	if req.Id != nil {
		return NewStringValue(*req.Id)
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
