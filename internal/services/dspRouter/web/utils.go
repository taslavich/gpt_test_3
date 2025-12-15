package dspRouterWeb

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/yl2chen/cidranger"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
)

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

func InitSspHttpClients(configTimeouts config.MapStringToDuration) map[string]*http.Client {
	timeouts := make(map[string]*http.Client)
	for sspDomain, timeout := range configTimeouts {
		mcSspDomain := MC_PREFIX + sspDomain
		adlSspDomain := ADL_PREFIX + sspDomain

		client := NewFastHTTPClient(timeout)
		timeouts[mcSspDomain] = client
		timeouts[adlSspDomain] = client
	}

	timeouts[DEFAULT] = NewFastHTTPClient(150 * time.Millisecond)

	return timeouts
}

func getSspHttpClients(sspDomain string, clients map[string]*http.Client) *http.Client {
	client, ok := clients[sspDomain]
	if !ok {
		return clients[DEFAULT]
	}
	return client
}

func NewFastHTTPClient(timeout time.Duration) *http.Client {
	transport := &http.Transport{
		// ⚡ ДЛЯ 32 ЯДЕР И 250K СОЕДИНЕНИЙ:
		MaxIdleConns:        50000, // Было 500
		MaxIdleConnsPerHost: 10000, // Было 100
		MaxConnsPerHost:     0,     // БЕЗ ЛИМИТА

		// ⚡ Оптимизация для большего числа соединений
		IdleConnTimeout: 120 * time.Second,

		// ⚡ Больше буферы для high-throughput
		WriteBufferSize: 128 * 1024, // 128KB
		ReadBufferSize:  128 * 1024,

		// ⚡ Быстрее переподключения
		DialContext: (&net.Dialer{
			Timeout:   2 * time.Second, // Быстрее!
			KeepAlive: 120 * time.Second,
			DualStack: true,
		}).DialContext,

		// ⚡ Отключаем HTTP/2 - он создаёт один connection pool
		// Для RTB лучше много отдельных соединений
		ForceAttemptHTTP2: false,

		// ⚡ Включаем TCP FastOpen если поддерживается
		// (нужно проверить ядро)
	}

	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
		// ⚡ Отключаем редиректы в DSP роутере
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func DeletePrefix(domain string) string {
	if newDomain, ok := strings.CutPrefix(domain, ADL_PREFIX); ok {
		return newDomain
	}

	if newDomain, ok := strings.CutPrefix(domain, MC_PREFIX); ok {
		return newDomain
	}

	return ""
}
