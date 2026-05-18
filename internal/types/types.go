package types

type Ortb struct {
	UUID            string `json:"UUID"`
	EVENT_TIME      string `json:"EVENT_TIME"`
	TYPIC           string `json:"TYPIC"`
	FORMAT          string `json:"FORMAT"`
	SPP_DOMAIN      string `json:"SPP_DOMAIN"`
	GEO             string `json:"GEO"`
	CITY_ID         string `json:"CITY_ID"`
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
	UUID              string `json:"UUID"`
	EVENT_TIME_CLICKS string `json:"EVENT_TIME_CLICKS"`
}

type Impressions struct {
	UUID                   string `json:"UUID"`
	EVENT_TIME_IMPRESSIONS string `json:"EVENT_TIME_IMPRESSIONS"`
}

type PercentAndBidfloor struct {
	Percent  float32 `json:"percent"`
	Bidfloor bool    `json:"bidfloor"`
}

func HasDataOrtb(record Ortb) bool {
	return record.UUID != "" ||
		record.EVENT_TIME != "" ||
		record.TYPIC != "" ||
		record.FORMAT != "" ||
		record.SPP_DOMAIN != "" ||
		record.GEO != "" ||
		record.CITY_ID != "" ||
		record.BID_RESPONSES != "" ||
		record.IP != "" ||
		record.IPV6 != "" ||
		record.LANG != "" ||
		record.BROWSER != "" ||
		record.BROWSER_VERSION != "" ||
		record.OS != "" ||
		record.OS_VERSION != "" ||
		record.DEVICE != "" ||
		record.SITE_ID != "" ||
		record.SITE_DOMAIN != "" ||
		record.BID_FLOOR != "" ||
		record.WIN_DSP_DOMAIN != "" ||
		record.WIN_PRICE != "" ||
		record.WIN_DSP_PRICE != "" ||
		record.WIN_CID != "" ||
		record.WIN_CRID != "" ||
		record.WIN_USER_ID != ""
}

func HasDataClicks(record Clicks) bool {
	return record.UUID != "" ||
		record.EVENT_TIME_CLICKS != ""
}

func HasDataImpressions(record Impressions) bool {
	return record.UUID != "" ||
		record.EVENT_TIME_IMPRESSIONS != ""
}
