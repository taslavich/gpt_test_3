package types

type StatisticsRecord struct {
	UUID                string `json:"UUID"`
	TIMESTAMP           string `json:"TIMESTAMP"`
	TYPIC               string `json:"TYPIC"`
	FORMAT              string `json:"FORMAT"`
	SPP_DOMAIN          string `json:"SPP_DOMAIN"`
	BID_REQUEST         string `json:"BID_REQUEST"`
	GEO_COLUMN          string `json:"GEO_COLUMN"`
	CITY_ID_COLUMN      string `json:"CITY_ID_COLUMN"`
	BID_RESPONSES       string `json:"BID_RESPONSES"`
	BID_RESPONSE_WINNER string `json:"BID_RESPONSE_WINNER"`
	ADM_IP              string `json:"ADM_IP"`
	ADM                 string `json:"ADM"`
}

type PercentAndBidfloor struct {
	Percent  float32 `json:"percent"`
	Bidfloor bool    `json:"bidfloor"`
}
