package constants

const (
	NEGATIVE_BIDFLOOR = -1
	ZERO_BIDFLOOR     = 0
)

const (
	EVENT_TIME_COLUMN      = "EVENT_TIME"
	TYPIC_COLUMN           = "TYPIC"
	FORMAT_COLUMN          = "FORMAT"
	SPP_DOMAIN_COLUMN      = "SPP_DOMAIN"
	GEO_COLUMN             = "GEO"
	CITY_ID_COLUMN         = "CITY_ID"
	CODE_COLUMN            = "CODE"
	BID_RESPONSES_COLUMN   = "BID_RESPONSES"
	IP_COLUMN              = "IP"
	IPV6_COLUMN            = "IPV6"
	LANG_COLUMN            = "LANG"
	BROWSER_COLUMN         = "BROWSER"
	BROWSER_VERSION_COLUMN = "BROWSER_VERSION"
	OS_COLUMN              = "OS"
	OS_VERSION_COLUMN      = "OS_VERSION"
	DEVICE_COLUMN          = "DEVICE"
	SITE_ID_COLUMN         = "SITE_ID"
	SITE_DOMAIN_COLUMN     = "SITE_DOMAIN"
	BID_FLOOR_COLUMN       = "BID_FLOOR"
	WIN_DSP_DOMAIN_COLUMN  = "WIN_DSP_DOMAIN"
	WIN_PRICE_COLUMN       = "WIN_PRICE"
	WIN_DSP_PRICE_COLUMN   = "WIN_DSP_PRICE"
	WIN_CID_COLUMN         = "WIN_CID"
	WIN_CRID_COLUMN        = "WIN_CRID"
	WIN_USER_ID_COLUMN     = "WIN_USER_ID"
)

const (
	EVENT_TIME_CLICKS_COLUMN = "EVENT_TIME_CLICKS"
)

const (
	EVENT_TIME_IMPRESSIONS_COLUMN = "EVENT_TIME_IMPRESSIONS"
)

const (
	UUID = "UUID"
)

const (
	FALSE = "0"
	TRUE  = "1"
)

const (
	IPP string = "IPP"
	POP string = "POP"
	BAN string = "BAN"
	NAT string = "NAT"
)

const (
	BOOLEAN_FALSE = false
	BOOLEAN_TRUE  = true
)

var FormatToCodes = map[string]string{
	POP: "0",
	BAN: "1",
	NAT: "2",
	IPP: "3",
}

var CodeToFormat = map[string]string{
	"0": POP,
	"1": BAN,
	"2": NAT,
	"3": IPP,
}
