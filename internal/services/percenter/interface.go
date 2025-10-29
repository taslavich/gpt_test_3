package percenter

import (
	"time"

	bidEngineGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/bidEngine"
)

var _ IPercenter = (*TPercenter)(nil)

type Bid struct {
	Price    float32 `json:"price"`
	DSPPrice float32 `json:"dsp_price"`
}

type Seatbid struct {
	Bid []Bid `json:"bid"`
}

type BidResponse struct {
	Seatbid []Seatbid `json:"Seatbid"`
}

type StatRecord struct {
	Domain        string    `ch:"domain"`
	Geo           string    `ch:"geo"`
	Ts5           time.Time `ch:"ts5"`
	TotalPrice    float32   `ch:"total_price"`
	TotalDSPPrice float32   `ch:"total_dsp_price"`
	Frofit        float32   `ch:"frofit"`
}

type GroupedData struct {
	Domain        string
	Geo           string
	Group         int
	TotalPrice    float32
	TotalDSPPrice float32
	Frofit        float32
}

type AggregatedData struct {
	Domain string
	Geo    string
	Frofit []float64
}

type TPercenter struct {
	addressOfBidEngine string
}

type IPercenter interface {
	GetGrpClient() (
		bidEngineGrpc.BidEngineServiceClient,
		func() error,
	)
}
