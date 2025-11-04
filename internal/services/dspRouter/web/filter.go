package dspRouterWeb

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/yl2chen/cidranger"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

var innerFilterMap = map[string]func(bidRequest *ortb_V2_5.BidRequest) bool{
	"http://ortbtwinbidexadlt.hilltopadsfeed.com/ask": func(bidRequest *ortb_V2_5.BidRequest) bool {
		return allowedUA(bidRequest.GetDevice().GetUa()) && i
	},
}

// ValidateUA проверяет User-Agent по всем правилам блокировки
// Возвращает nil если UA валиден, error если заблокирован
func allowedUA(ua string) bool {
	// Нормализуем UA
	normalizedUA := strings.TrimSpace(ua)

	// 1. Пустой UA или "-"
	if normalizedUA == "" || normalizedUA == "-" {
		return false
	}

	// 2. Автоматизация/скрипты
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

	// 3. Боты/краулеры
	botPatterns := []string{
		`(?i)bot`, `(?i)crawler`, `(?i)spider`,
		`(?i)scrap`, `(?i)scan`, `(?i)checker`,
	}

	for _, pattern := range botPatterns {
		if matched, _ := regexp.MatchString(pattern, normalizedUA); matched {
			return false
		}
	}

	// 4. Версия браузера 0 или невалидная
	zeroVersionPatterns := []string{
		`Chrome/0`, `Firefox/0`, `Version/0`,
		`Chrome/0\.`, `Firefox/0\.`, `Version/0\.`,
	}

	for _, pattern := range zeroVersionPatterns {
		if strings.Contains(normalizedUA, pattern) {
			return false
		}
	}

	// 5. Длина UA
	if len(normalizedUA) < 20 {
		return false
	}
	if len(normalizedUA) > 512 {
		return false
	}

	// 6. Несогласованная комбинация ОС/устройства
	osMismatchPatterns := []string{
		`(?i)Windows NT.*Mobile`,
		`(?i)Macintosh;.*Mobile`,
	}

	for _, pattern := range osMismatchPatterns {
		if matched, _ := regexp.MatchString(pattern, normalizedUA); matched {
			return false
		}
	}

	// 7. Мобильный маркер без браузерных маркеров
	if hasMobileMarker(normalizedUA) && !hasBrowserMarker(normalizedUA) {
		return false
	}

	// 8. Старые версии браузеров
	if hasOldBrowser(normalizedUA) {
		return false
	}

	return true
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

func (s *Server) LoadNetset(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("open netset file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	loadedCount := 0
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Пропускаем пустые строки и комментарии
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Парсим CIDR
		_, network, err := net.ParseCIDR(line)
		if err != nil {
			log.Printf("Line %d: invalid network %s: %v", lineNum, line, err)
			continue
		}

		s.ranger.Insert(cidranger.NewBasicRangerEntry(*network))
		loadedCount++
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read netset file: %w", err)
	}

	log.Printf("Loaded %d networks from %s", loadedCount, filename)
	return nil
}

func (s *Server) isIPAllowed(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		log.Printf("invalid IP address: %s", ipStr)
		return false
	}

	blocked, err := s.ranger.Contains(ip)
	if err != nil {
		log.Printf("IP lookup error for %s: %v", ipStr, err)
		return false
	}

	return !blocked
}
