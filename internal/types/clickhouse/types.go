package clickhouse_types

type BidResponse struct {
	Id      *string `json:"id,omitempty"`
	Seatbid []*SeatBid
	Error   string
}

type SeatBid struct {
	Bid []*Bid `json:"bid,omitempty"`
}

type Bid struct {
	DspDomain *string  `json:"dsp_domain,omitempty"`
	Id        *string  `json:"id,omitempty"`
	Impid     *string  `json:"impid,omitempty"`
	Price     *float32 `json:"price,omitempty"`
	DspPrice  *float32 `json:"dsp_price,omitempty"`
	Adid      *string  `json:"adid,omitempty"`
	Cid       *string  `json:"cid,omitempty"`
	Crid      *string  `json:"crid,omitempty"`
}
