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
	case FieldDeviceCountry:
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
	default:
		return FieldValue{}
	}
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
	if req.Device != nil && req.Device.Ip != nil {
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

// StatelessV24BidResponseExtractor - stateless экстрактор для BidResponse v2.4
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
	if resp != nil && resp.Seatbid != nil && resp.Seatbid.Bid != nil {
		for i := range resp.Seatbid.Bid {
			if resp.Seatbid.Bid[i].Adid != nil {
				return NewStringValue(*resp.Seatbid.Bid[i].Adid)
			}
		}
	}
	return NewStringValue("")
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

func (e *StatelessV25BidResponseExtractor) extractBidPrice(resp *ortb_V2_5.BidResponse) FieldValue {
	if resp != nil && resp.Seatbid != nil && resp.Seatbid.Bid != nil {
		for i := range resp.Seatbid.Bid {
			if resp.Seatbid.Bid[i].Price != nil {
				return NewFloatValue(float64(*resp.Seatbid.Bid[i].Price))
			}
		}
	}
	return NewFloatValue(0)
}

func (e *StatelessV25BidResponseExtractor) extractBidID(resp *ortb_V2_5.BidResponse) FieldValue {
	if resp != nil && resp.Seatbid != nil && resp.Seatbid.Bid != nil {
		for i := range resp.Seatbid.Bid {
			if resp.Seatbid.Bid[i].Id != nil {
				return NewStringValue(*resp.Seatbid.Bid[i].Id)
			}
		}
	}
	return NewStringValue("")
}

func (e *StatelessV25BidResponseExtractor) extractBidAdID(resp *ortb_V2_5.BidResponse) FieldValue {
	if resp != nil && resp.Seatbid != nil && resp.Seatbid.Bid != nil {
		for i := range resp.Seatbid.Bid {
			if resp.Seatbid.Bid[i].Adid != nil {
				return NewStringValue(*resp.Seatbid.Bid[i].Adid)
			}
		}
	}
	return NewStringValue("")
}

func (e *StatelessV25BidResponseExtractor) extractBidImpID(resp *ortb_V2_5.BidResponse) FieldValue {
	if resp != nil && resp.Seatbid != nil && resp.Seatbid.Bid != nil {
		for i := range resp.Seatbid.Bid {
			if resp.Seatbid.Bid[i].Impid != nil {
				return NewStringValue(*resp.Seatbid.Bid[i].Impid)
			}
		}
	}
	return NewStringValue("")
}

func (e *StatelessV25BidResponseExtractor) extractBidArray(resp *ortb_V2_5.BidResponse) FieldValue {
	if resp != nil && resp.Seatbid != nil && resp.Seatbid.Bid != nil {
		return NewStringValue("exists")
	}
	return NewStringValue("")
}

func (e *StatelessV25BidResponseExtractor) extractBidNurl(resp *ortb_V2_5.BidResponse) FieldValue {
	if resp != nil && resp.Seatbid != nil && resp.Seatbid.Bid != nil {
		for i := range resp.Seatbid.Bid {
			if resp.Seatbid.Bid[i].Nurl != nil {
				return NewStringValue(*resp.Seatbid.Bid[i].Nurl)
			}
		}
	}
	return NewStringValue("")
}

func (e *StatelessV25BidResponseExtractor) extractBidBurl(resp *ortb_V2_5.BidResponse) FieldValue {
	if resp != nil && resp.Seatbid != nil && resp.Seatbid.Bid != nil {
		for i := range resp.Seatbid.Bid {
			if resp.Seatbid.Bid[i].Burl != nil {
				return NewStringValue(*resp.Seatbid.Bid[i].Burl)
			}
		}
	}
	return NewStringValue("")
}
