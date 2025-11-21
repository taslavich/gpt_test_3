package dspRouterWeb

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/yl2chen/cidranger"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

var innerFilterMap = map[string]func(bidRequest *ortb_V2_5.BidRequest, ranger cidranger.Ranger) bool{
	"adl_dsp_hilltopads.com": func(bidRequest *ortb_V2_5.BidRequest, ranger cidranger.Ranger) bool {
		return AllowedSiteHilltop(bidRequest) // allowedUA(bidRequest.GetDevice().GetUa()) && isIPAllowed(bidRequest.GetDevice().GetIp(), ranger, bidRequest) &&
	},
	"mc_dsp_dao.ad": func(bidRequest *ortb_V2_5.BidRequest, ranger cidranger.Ranger) bool {
		return AllowedSiteDao(bidRequest) // && allowedUA(bidRequest.GetDevice().GetUa())
	},
	"adl_dsp_dao.ad": func(bidRequest *ortb_V2_5.BidRequest, ranger cidranger.Ranger) bool {
		return AllowedSiteDao(bidRequest) //&& allowedUA(bidRequest.GetDevice().GetUa())
	},
	/*
		"http://u625267.pophandler.net/rtb/?async=1&code_type=1&js=1&rtbRequest=1&sid=940499": func(bidRequest *ortb_V2_5.BidRequest, ranger cidranger.Ranger) bool {
			return !HasIpv6(bidRequest)
		},
		"http://pop.zog.link/bid-request?token=h6dKfdh544FHD83": func(bidRequest *ortb_V2_5.BidRequest, ranger cidranger.Ranger) bool {
			return AllowedSite(bidRequest) && allowedUA(bidRequest.GetDevice().GetUa())
		},
	*/
}

func AllowedSiteDao(bidRequest *ortb_V2_5.BidRequest) bool {
	if bidRequest.Site.GetId() == "" {
		return true
	}

	siteId := bidRequest.Site.GetId()

	blockedList := map[string]bool{
		"5028151":   true,
		"5026223":   true,
		"4948223":   true,
		"4948184":   true,
		"5028223":   true,
		"5028170":   true,
		"635585":    true,
		"4889149":   true,
		"4889163":   true,
		"5026118":   true,
		"4889164":   true,
		"4948133":   true,
		"5028133":   true,
		"4948163":   true,
		"5026163":   true,
		"5318285":   true,
		"4889562":   true,
		"5028562":   true,
		"5028149":   true,
		"4889121":   true,
		"4794562":   true,
		"4889170":   true,
		"4794163":   true,
		"4948972":   true,
		"5026170":   true,
		"5026562":   true,
		"5026133":   true,
		"5028157":   true,
		"3706708":   true,
		"6765":      true,
		"4948121":   true,
		"53351290":  true,
		"531426843": true,
		"1514881":   true,
		"1133":      true,
		"532006761": true,
		"532008162": true,
		"532006760": true,
		"532006815": true,
		"532006323": true,
		"532006321": true,
		"532006984": true,
		"53502320":  true,
		"53839256":  true,
		"366142":    true,
		"1449072":   true,
		"284407":    true,
		"201667":    true,
		"840":       true,
		"536042675": true,
		"531432573": true,
		"532006762": true,
		"531429670": true,
		"531436242": true,
		"532007766": true,
		"531430094": true,
		"536046031": true,
		"532008634": true,
		"532007151": true,
		"531430095": true,
		"531430092": true,
		"531443452": true,
		"536046623": true,
		"532006978": true,
		"536046673": true,
		"531430096": true,
		"531450109": true,
		"536046044": true,
		"531430097": true,
		"531434313": true,
		"536097192": true,
	}

	return !blockedList[siteId]
}

func ChangeSiteId(bidRequest *ortb_V2_5.BidRequest) {
	if bidRequest.Site.GetId() == "" {
		return
	}

	siteId := bidRequest.Site.GetId()

	whiteList := map[string]bool{
		"1467988": true,
		"1467987": true,
		"1457703": true,
		"1457702": true,
		"1461440": true,
		"6037953": true,
		"6092051": true,
		"1455125": true,
		"6092052": true,
		"2012434": true,
		"2012438": true,
		"1457725": true,
		"840":     true,
	}

	if res := whiteList[siteId]; res {
		newNum, err := strconv.Atoi(siteId)
		if err != nil {
			return
		}

		newId := strconv.Itoa(newNum * 2)
		bidRequest.Site.Id = &newId
	}
}

func AllowedSiteHilltop(bidRequest *ortb_V2_5.BidRequest) bool {
	if bidRequest.Site.GetId() == "" {
		return true
	}

	siteId := bidRequest.Site.GetId()

	blockedList := map[string]bool{
		"888687":     true,
		"81880":      true,
		"888701":     true,
		"888685":     true,
		"888703":     true,
		"134812":     true,
		"123979":     true,
		"121603667":  true,
		"213781":     true,
		"2010283":    true,
		"1470301":    true,
		"1457558":    true,
		"2010614":    true,
		"162772":     true,
		"251659":     true,
		"888688":     true,
		"1463271":    true,
		"159487":     true,
		"158953":     true,
		"255112":     true,
		"350896":     true,
		"1332056814": true,
		"121654102":  true,
		"11017520":   true,
	}

	return !blockedList[siteId]
}

func Allowed(endpoint string, bidRequest *ortb_V2_5.BidRequest, ranger cidranger.Ranger) bool {
	val, ok := innerFilterMap[endpoint]

	if !ok {
		return true
	} else {
		return val(bidRequest, ranger)
	}
}

// allowedUA проверяет User-Agent по всем правилам блокировки
// Возвращает true если UA валиден, false если заблокирован
func allowedUA(ua string) bool {
	// Нормализуем UA
	normalizedUA := strings.TrimSpace(ua)

	// 1. Пустой UA или "-"
	if normalizedUA == "" || normalizedUA == "-" {
		return false
	}

	// 2. Google Search App (GSA) - БЛОКИРУЕМ всё с GSA
	if strings.Contains(normalizedUA, "GSA/") {
		return false
	}

	// 3. Подозрительные версии Chrome (например, 141.0.0.0 вместо 141.0.0.1)
	/*if hasSuspiciousChromeVersion(normalizedUA) {
		return false
	}

	if hasSuspiciousIOSBuild(normalizedUA) {
		return false
	}*/

	if hasMismatchedOSBrowser(normalizedUA) {
		return false
	}

	// 4. Автоматизация/скрипты
	automationPatterns := []string{
		`(?i)HeadlessChrome`, `(?i)PhantomJS`, `(?i)Selenium`,
		`(?i)Puppeteer`, `(?i)curl`, `(?i)wget`,
		`(?i)python-requests`, `(?i)Go-http-client`,
		`(?i)okhttp`, `(?i)libwww-perl`,
		`(?i)Apache-HttpClient`, `(?i)HttpClient`,
		`(?i)node\.js`,
	}

	for _, pattern := range automationPatterns {
		if matched, _ := regexp.MatchString(pattern, normalizedUA); matched {
			return false
		}
	}

	// 5. Боты/краулеры
	botPatterns := []string{
		`(?i)bot`, `(?i)crawler`, `(?i)spider`,
		`(?i)scrap`, `(?i)scan`, `(?i)checker`,
	}

	for _, pattern := range botPatterns {
		if matched, _ := regexp.MatchString(pattern, normalizedUA); matched {
			return false
		}
	}

	// 6. Версия браузера 0 или невалидная
	zeroVersionPatterns := []string{
		`Chrome/0`, `Firefox/0`, `Version/0`,
		`Chrome/0\.`, `Firefox/0\.`, `Version/0\.`,
	}

	for _, pattern := range zeroVersionPatterns {
		if strings.Contains(normalizedUA, pattern) {
			return false
		}
	}

	// 7. Длина UA
	if len(normalizedUA) < 20 {
		return false
	}
	if len(normalizedUA) > 512 {
		return false
	}

	// 8. Несогласованная комбинация ОС/устройства
	osMismatchPatterns := []string{
		`(?i)Windows NT.*Mobile`,
		`(?i)Macintosh;.*Mobile`,
	}

	for _, pattern := range osMismatchPatterns {
		if matched, _ := regexp.MatchString(pattern, normalizedUA); matched {
			return false
		}
	}

	// 9. Мобильный маркер без браузерных маркеров
	if hasMobileMarker(normalizedUA) && !hasBrowserMarker(normalizedUA) {
		return false
	}

	// 10. Старые версии браузеров
	if hasOldBrowser(normalizedUA) {
		return false
	}

	return true
}

// hasSuspiciousChromeVersion проверяет подозрительные версии Chrome
// Например: Chrome/141.0.0.0 (все нули в минорных версиях)
func hasSuspiciousChromeVersion(ua string) bool {
	// Ищем Chrome версии с .0.0.0 в конце
	suspiciousPatterns := []string{
		`Chrome/\d+\.0\.0\.0`,
		`Chrome/\d+\.0\.0\s`,
	}

	for _, pattern := range suspiciousPatterns {
		if matched, _ := regexp.MatchString(pattern, ua); matched {
			return true
		}
	}
	return false
}

// hasMobileMarker проверяет наличие мобильных маркеров
func hasMobileMarker(ua string) bool {
	mobilePatterns := []string{
		`(?i)Android`, `(?i)iPhone`, `(?I)iPad`, `(?i)Mobile`,
	}

	for _, pattern := range mobilePatterns {
		if matched, _ := regexp.MatchString(pattern, ua); matched {
			return true
		}
	}
	return false
}

// hasBrowserMarker проверяет наличие браузерных маркеров
func hasBrowserMarker(ua string) bool {
	browserPatterns := []string{
		`(?i)Safari`, `(?i)Chrome`, `(?i)Firefox`, `(?i)Edge`,
		`(?i)AppleWebKit`, `(?i)Gecko`,
	}

	for _, pattern := range browserPatterns {
		if matched, _ := regexp.MatchString(pattern, ua); matched {
			return true
		}
	}
	return false
}

// hasOldBrowser проверяет устаревшие версии браузеров
func hasOldBrowser(ua string) bool {
	// Chrome < 70
	if chromeVersion := extractVersion(ua, `Chrome/(\d+)`); chromeVersion > 0 && chromeVersion < 70 {
		return true
	}

	// Firefox < 60
	if firefoxVersion := extractVersion(ua, `Firefox/(\d+)`); firefoxVersion > 0 && firefoxVersion < 60 {
		return true
	}

	// Safari < 10
	if safariVersion := extractVersion(ua, `Version/(\d+)`); safariVersion > 0 && safariVersion < 10 {
		return true
	}

	return false
}

// hasSuspiciousIOSBuild проверяет подозрительные iOS build tokens
// Например: Mobile/15E148 с новыми версиями iOS
func hasSuspiciousIOSBuild(ua string) bool {
	// Проверяем старый build token с новыми версиями iOS
	if strings.Contains(ua, "Mobile/15E148") {
		// Если iOS версия >= 18, а build token старый - подозрительно
		iosVersion := extractVersion(ua, `iPhone OS (\d+)`)
		if iosVersion >= 18 {
			return true
		}
	}
	return false
}

// hasMismatchedOSBrowser проверяет несоответствия версий ОС и браузеров
func hasMismatchedOSBrowser(ua string) bool {
	// Проверяем Android + Chrome несоответствия
	if strings.Contains(ua, "Android") {
		androidVersion := extractVersion(ua, `Android (\d+)`)
		chromeVersion := extractVersion(ua, `Chrome/(\d+)`)

		// Слишком новый Chrome на старом Android
		if androidVersion > 0 && chromeVersion > 0 {
			if androidVersion <= 8 && chromeVersion >= 120 {
				return true
			}
			if androidVersion <= 9 && chromeVersion >= 130 {
				return true
			}
		}
	}

	// Проверяем iOS + Safari несоответствия
	if strings.Contains(ua, "iPhone OS") || strings.Contains(ua, "CPU OS") {
		iosVersion := extractVersion(ua, `OS (\d+)`)
		safariVersion := extractVersion(ua, `Version/(\d+)`)

		// Слишком новый Safari на старом iOS
		if iosVersion > 0 && safariVersion > 0 {
			if iosVersion <= 12 && safariVersion >= 15 {
				return true
			}
			if iosVersion <= 14 && safariVersion >= 18 {
				return true
			}
		}
	}

	if strings.Contains(ua, "Macintosh") && strings.Contains(ua, "Safari") {
		macVersion := extractVersion(ua, `Mac OS X (\d+)_(\d+)`)
		safariVersion := extractVersion(ua, `Version/(\d+)`)

		// Если Safari версия значительно новее macOS
		if macVersion > 0 && safariVersion > 0 {
			// macOS 10.15 + Safari 26 = подозрительно
			if macVersion <= 10 && safariVersion >= 20 {
				return true
			}
		}
	}

	return false
}

// extractVersion извлекает версию браузера из UA
func extractVersion(ua, pattern string) int {
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(ua)
	if len(matches) > 1 {
		if version, err := strconv.Atoi(matches[1]); err == nil {
			return version
		}
	}
	return 0
}

/*
func HasIpv6(bidRequest *ortb_V2_5.BidRequest) bool {
	if bidRequest.Device != nil {
		bidRequest.Device.Ipv6 = nil

		ip := net.ParseIP(bidRequest.Device.GetIp())
		if ip == nil {
			log.Printf("invalid IP address: %s", bidRequest.Device.GetIp())
			return false
		}

		if ip.To4() == nil {
			return true
		}
	}

	return false
}
*/

/*
func isIPAllowed(ipStr string, ranger cidranger.Ranger, bidRequest *ortb_V2_5.BidRequest) bool {
	if bidRequest.Device != nil {
		bidRequest.Device.Ipv6 = nil

		ip := net.ParseIP(bidRequest.Device.GetIp())
		if ip == nil {
			log.Printf("invalid IP address: %s", bidRequest.Device.GetIp())
			return false
		}

		if ip.To4() == nil {
			return false
		}

		blocked, err := ranger.Contains(ip)
		if err != nil {
			log.Printf("IP lookup error for %s: %v", ipStr, err)
			return false
		}

		return !blocked
	}

	return false
}*/
