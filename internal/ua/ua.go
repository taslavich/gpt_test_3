package ua

import "github.com/mileusna/useragent"

type UAFields struct {
	Browser        string `json:"browser"`
	BrowserVersion string `json:"browser_version"`
	OS             string `json:"os"`
	OSVersion      string `json:"os_version"`
	Device         string `json:"device"`
}

func normalizeDevice(ua useragent.UserAgent) string {
	switch {
	case ua.Bot:
		return "bot"
	case ua.Mobile:
		return "mobile"
	case ua.Tablet:
		return "tablet"
	case ua.Desktop:
		return "desktop"
	default:
		return "other"
	}
}

func ParseUA(rawUA string) UAFields {
	ua := useragent.Parse(rawUA)

	return UAFields{
		Browser:        ua.Name,
		BrowserVersion: ua.Version,
		OS:             ua.OS,
		OSVersion:      ua.OSVersion,
		Device:         normalizeDevice(ua),
	}
}
