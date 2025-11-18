package dspRouterWeb

import (
	"log"
	"net"

	"github.com/yl2chen/cidranger"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

var innerFilterMap = map[string]func(bidRequest *ortb_V2_5.BidRequest, ranger cidranger.Ranger) bool{
	"http://ortbtwinbidexadlt.hilltopadsfeed.com/ask": func(bidRequest *ortb_V2_5.BidRequest, ranger cidranger.Ranger) bool {
		return AllowedSite(bidRequest) // allowedUA(bidRequest.GetDevice().GetUa()) && isIPAllowed(bidRequest.GetDevice().GetIp(), ranger, bidRequest) &&
	},
	/*"http://pop-48702.daortb.com/api/rtb-pops/item?sourceId=59738&api-key=xvKZ-_oewvADCb2RR0W6bgp_EdLEKCLj": func(bidRequest *ortb_V2_5.BidRequest, ranger cidranger.Ranger) bool {
		return AllowedSite(bidRequest) && allowedUA(bidRequest.GetDevice().GetUa())
	},
	"http://u625267.pophandler.net/rtb/?async=1&code_type=1&js=1&rtbRequest=1&sid=940499": func(bidRequest *ortb_V2_5.BidRequest, ranger cidranger.Ranger) bool {
		return !HasIpv6(bidRequest)
	},
	"http://pop.zog.link/bid-request?token=h6dKfdh544FHD83": func(bidRequest *ortb_V2_5.BidRequest, ranger cidranger.Ranger) bool {
		return AllowedSite(bidRequest) && allowedUA(bidRequest.GetDevice().GetUa())
	},*/
}

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

func AllowedSite(bidRequest *ortb_V2_5.BidRequest) bool {
	if bidRequest.Site == nil || bidRequest.Site.Id == nil {
		return false
	}

	if bidRequest.Site.GetId() == "" || bidRequest.Site.GetId() == " " {
		return false
	}

	siteId := bidRequest.Site.GetId()

	whiteList := map[string]bool{
		"348160":     true,
		"1470468":    true,
		"6068230":    true,
		"2055":       true,
		"408582":     true,
		"1462282":    true,
		"399380":     true,
		"1463316":    true,
		"402454":     true,
		"6091798":    true,
		"8216":       true,
		"1437721":    true,
		"1462301":    true,
		"6091805":    true,
		"89123":      true,
		"6059044":    true,
		"1463333":    true,
		"1463334":    true,
		"54324":      true,
		"1465400":    true,
		"835642":     true,
		"835646":     true,
		"835648":     true,
		"8260":       true,
		"2012230":    true,
		"2009161":    true,
		"412748":     true,
		"6098004":    true,
		"466008":     true,
		"1449051":    true,
		"1455198":    true,
		"675938":     true,
		"1425519":    true,
		"6089841":    true,
		"2011253":    true,
		"1425528":    true,
		"6086779":    true,
		"2010235":    true,
		"466044":     true,
		"2008199":    true,
		"6067335":    true,
		"6091914":    true,
		"524430":     true,
		"493714":     true,
		"84117":      true,
		"6084763":    true,
		"6084764":    true,
		"817308":     true,
		"1442972":    true,
		"6082720":    true,
		"1440931":    true,
		"1413285":    true,
		"22695":      true,
		"6093991":    true,
		"6098088":    true,
		"2012330":    true,
		"1468588":    true,
		"289962":     true,
		"6097070":    true,
		"6054067":    true,
		"310452":     true,
		"489656":     true,
		"1464505":    true,
		"21688":      true,
		"21689":      true,
		"1419456":    true,
		"1419457":    true,
		"1468609":    true,
		"80067":      true,
		"1419460":    true,
		"1464517":    true,
		"487620":     true,
		"1419461":    true,
		"53446":      true,
		"6063301":    true,
		"1469640":    true,
		"2012363":    true,
		"6093004":    true,
		"1453275":    true,
		"2009313":    true,
		"2009319":    true,
		"2009320":    true,
		"1445096":    true,
		"2011377":    true,
		"6025461":    true,
		"378104":     true,
		"785660":     true,
		"6098176":    true,
		"1466625":    true,
		"836866":     true,
		"1466627":    true,
		"2009348":    true,
		"2309":       true,
		"505104":     true,
		"2009364":    true,
		"2012436":    true,
		"2012437":    true,
		"6071576":    true,
		"836890":     true,
		"6061338":    true,
		"6066462":    true,
		"1468704":    true,
		"1468705":    true,
		"1468706":    true,
		"62753":      true,
		"6094114":    true,
		"58674":      true,
		"2009398":    true,
		"2012471":    true,
		"1423672":    true,
		"6083896":    true,
		"6083897":    true,
		"6094137":    true,
		"1469756":    true,
		"1423679":    true,
		"1470784":    true,
		"1452354":    true,
		"2009414":    true,
		"1466696":    true,
		"395594":     true,
		"1470794":    true,
		"6073677":    true,
		"467278":     true,
		"609616":     true,
		"447824":     true,
		"2009426":    true,
		"6089046":    true,
		"478558":     true,
		"6089055":    true,
		"467296":     true,
		"6089057":    true,
		"22882":      true,
		"354660":     true,
		"6068591":    true,
		"6086005":    true,
		"6068602":    true,
		"1424763":    true,
		"487804":     true,
		"25982":      true,
		"818560":     true,
		"1462657":    true,
		"532866":     true,
		"6098308":    true,
		"1453445":    true,
		"1452424":    true,
		"1462665":    true,
		"1462666":    true,
		"49547":      true,
		"1428873":    true,
		"44426":      true,
		"1404301":    true,
		"26003":      true,
		"1457557":    true,
		"2012568":    true,
		"484760":     true,
		"1444251":    true,
		"6097308":    true,
		"1464735":    true,
		"1408416":    true,
		"6072740":    true,
		"6076843":    true,
		"1428917":    true,
		"1468854":    true,
		"2011577":    true,
		"2011578":    true,
		"2011579":    true,
		"58812":      true,
		"2011581":    true,
		"6097342":    true,
		"6097341":    true,
		"2011584":    true,
		"6097343":    true,
		"2011582":    true,
		"6097345":    true,
		"1411524":    true,
		"1411525":    true,
		"1411526":    true,
		"1462727":    true,
		"1466824":    true,
		"1411527":    true,
		"1411530":    true,
		"1411531":    true,
		"342476":     true,
		"1468877":    true,
		"1458637":    true,
		"1427919":    true,
		"829904":     true,
		"1411532":    true,
		"837068":     true,
		"785876":     true,
		"6098390":    true,
		"6072793":    true,
		"1438172":    true,
		"2008542":    true,
		"2008543":    true,
		"2008544":    true,
		"2008545":    true,
		"2005474":    true,
		"2009571":    true,
		"2008547":    true,
		"508394":     true,
		"1460715":    true,
		"6067698":    true,
		"1427959":    true,
		"77305":      true,
		"1420796":    true,
		"1462781":    true,
		"1420797":    true,
		"1462784":    true,
		"6075904":    true,
		"387586":     true,
		"1434115":    true,
		"1462789":    true,
		"1462791":    true,
		"306696":     true,
		"6064651":    true,
		"64013":      true,
		"6059535":    true,
		"6091280":    true,
		"1455632":    true,
		"1439254":    true,
		"6085148":    true,
		"20008":      true,
		"20009":      true,
		"2008624":    true,
		"1403444":    true,
		"1413688":    true,
		"1457727":    true,
		"1464896":    true,
		"503362":     true,
		"1457739":    true,
		"6091343":    true,
		"1467984":    true,
		"1457745":    true,
		"6068818":    true,
		"329302":     true,
		"1464924":    true,
		"573022":     true,
		"25183":      true,
		"6068837":    true,
		"6068839":    true,
		"6060649":    true,
		"6066794":    true,
		"6097519":    true,
		"825968":     true,
		"25199":      true,
		"825970":     true,
		"1464952":    true,
		"6097529":    true,
		"2005626":    true,
		"6060666":    true,
		"1442432":    true,
		"1447553":    true,
		"6084240":    true,
		"6063770":    true,
		"6093467":    true,
		"497306":     true,
		"836256":     true,
		"6098594":    true,
		"31398":      true,
		"2011817":    true,
		"2011818":    true,
		"1454762":    true,
		"7854":       true,
		"1461935":    true,
		"1452722":    true,
		"812722":     true,
		"92861":      true,
		"1331016388": true,
		"6095561":    true,
		"401104":     true,
		"1461968":    true,
		"5849":       true,
		"5851":       true,
		"5852":       true,
		"5853":       true,
		"5854":       true,
		"6093540":    true,
		"6093543":    true,
		"1425129":    true,
		"2009835":    true,
		"6080235":    true,
		"2008821":    true,
		"2011895":    true,
		"2011897":    true,
		"1453824":    true,
		"1449733":    true,
		"1469208":    true,
		"1439514":    true,
		"1453851":    true,
		"528154":     true,
		"67357":      true,
		"6092573":    true,
		"1446689":    true,
		"6094628":    true,
		"6094630":    true,
		"288552":     true,
		"6092589":    true,
		"518958":     true,
		"1453870":    true,
		"1409841":    true,
		"6089522":    true,
		"1439539":    true,
		"1439542":    true,
		"2010936":    true,
		"1439544":    true,
		"1465146":    true,
		"1439550":    true,
		"2008895":    true,
		"839":        true,
		"534344":     true,
		"528208":     true,
		"6062931":    true,
		"1421139":    true,
		"1443667":    true,
		"1470291":    true,
		"1467223":    true,
		"1471320":    true,
		"1465186":    true,
		"93027":      true,
		"2012005":    true,
		"2008941":    true,
		"1437556":    true,
		"2012024":    true,
		"6071163":    true,
		"121542524":  true,
		"1432446":    true,
		"466814":     true,
		"528256":     true,
		"1466241":    true,
		"293766":     true,
		"6075271":    true,
		"1428361":    true,
		"6066062":    true,
		"6092687":    true,
		"1467280":    true,
		"1467281":    true,
		"1470351":    true,
		"2008977":    true,
		"1453973":    true,
		"528278":     true,
		"1459095":    true,
		"1457057":    true,
		"1463202":    true,
		"6091686":    true,
		"1457063":    true,
		"415654":     true,
		"6061994":    true,
		"1461165":    true,
		"40879":      true,
		"6093766":    true,
		"299976":     true,
		"1437645":    true,
		"1466318":    true,
		"395214":     true,
		"6085584":    true,
		"410578":     true,
		"2012118":    true,
		"90073":      true,
		"6066138":    true,
		"6066141":    true,
		"2012128":    true,
		"2012129":    true,
		"2012131":    true,
		"2012133":    true,
		"85989":      true,
		"1456104":    true,
		"1427443":    true,
		"484340":     true,
		"1414137":    true,
		"1449978":    true,
	}

	return whiteList[siteId]
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
/*func allowedUA(ua string) bool {
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
	if hasSuspiciousChromeVersion(normalizedUA) {
		return false
	}

	if hasSuspiciousIOSBuild(normalizedUA) {
		return false
	}

	if hasMismatchedOSBrowser(normalizedUA) {
		return false
	}

	// 4. Автоматизация/скрипты
	automationPatterns := []string{
		`(?i)HeadlessChrome`, `(?i)PhantomJS`, `(?i)Selenium`: true,
		`(?i)Puppeteer`, `(?i)curl`, `(?i)wget`: true,
		`(?i)python-requests`, `(?i)Go-http-client`: true,
		`(?i)okhttp`, `(?i)libwww-perl`: true,
		`(?i)Apache-HttpClient`, `(?i)HttpClient`: true,
		`(?i)node\.js`,
	}

	for _, pattern := range automationPatterns {
		if matched, _ := regexp.MatchString(pattern, normalizedUA); matched {
			return false
		}
	}

	// 5. Боты/краулеры
	botPatterns := []string{
		`(?i)bot`, `(?i)crawler`, `(?i)spider`: true,
		`(?i)scrap`, `(?i)scan`, `(?i)checker`,
	}

	for _, pattern := range botPatterns {
		if matched, _ := regexp.MatchString(pattern, normalizedUA); matched {
			return false
		}
	}

	// 6. Версия браузера 0 или невалидная
	zeroVersionPatterns := []string{
		`Chrome/0`, `Firefox/0`, `Version/0`: true,
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
		`(?i)Windows NT.*Mobile`: true,
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
		`Chrome/\d+\.0\.0\.0`: true,
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
		`(?i)Safari`, `(?i)Chrome`, `(?i)Firefox`, `(?i)Edge`: true,
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
