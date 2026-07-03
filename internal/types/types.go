package types

type Ortb struct {
	UUID            string `json:"UUID"`
	EVENT_TIME      string `json:"EVENT_TIME"`
	TYPIC           string `json:"TYPIC"`
	FORMAT          string `json:"FORMAT"`
	SPP_DOMAIN      string `json:"SPP_DOMAIN"`
	GEO             string `json:"GEO"`
	CITY_ID         string `json:"CITY_ID"`
	CODE            string `json:"CODE"`
	BID_RESPONSES   string `json:"BID_RESPONSES"`
	IP              string `json:"IP"`
	IPV6            string `json:"IPV6"`
	LANG            string `json:"LANG"`
	BROWSER         string `json:"BROWSER"`
	BROWSER_VERSION string `json:"BROWSER_VERSION"`
	OS              string `json:"OS"`
	OS_VERSION      string `json:"OS_VERSION"`
	DEVICE          string `json:"DEVICE"`
	SITE_ID         string `json:"SITE_ID"`
	SITE_DOMAIN     string `json:"SITE_DOMAIN"`
	BID_FLOOR       string `json:"BID_FLOOR"`
	WIN_DSP_DOMAIN  string `json:"WIN_DSP_DOMAIN"`
	WIN_PRICE       string `json:"WIN_PRICE"`
	WIN_DSP_PRICE   string `json:"WIN_DSP_PRICE"`
	WIN_CID         string `json:"WIN_CID"`
	WIN_CRID        string `json:"WIN_CRID"`
	WIN_USER_ID     string `json:"WIN_USER_ID"`
}

type Clicks struct {
	CLICKS_UUID       string `json:"CLICKS_UUID"`
	ORTB_UUID         string `json:"ORTB_UUID"`
	EVENT_TIME_CLICKS string `json:"EVENT_TIME_CLICKS"`
	FORMAT            string `json:"FORMAT"`
}

type Impressions struct {
	IMPRESSIONS_UUID       string `json:"IMPRESSIONS_UUID"`
	ORTB_UUID              string `json:"ORTB_UUID"`
	EVENT_TIME_IMPRESSIONS string `json:"EVENT_TIME_IMPRESSIONS"`
	FORMAT                 string `json:"FORMAT"`
}

type Conversions struct {
	CONVERSIONS_UUID string `json:"CONVERSIONS_UUID"`
	PAYOUT           string `json:"PAYOUT"`
}

type PercentAndBidfloor struct {
	Percent  float32 `json:"percent"`
	Bidfloor bool    `json:"bidfloor"`
}
