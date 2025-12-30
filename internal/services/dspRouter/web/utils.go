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

func getSspTimeout(sspDomain string, configTimeouts config.MapStringToDuration) time.Duration {
	var timeout time.Duration
	timeout, ok := configTimeouts[DeletePrefix(sspDomain)]
	if !ok {
		timeout = 150 * time.Millisecond
	}

	return timeout
}

func InitSspHttpClients(
	dspEndpoints_v_2_5 config.MapStringToString,
	dspEndpoints_mainstream_v_2_5 config.MapStringToString,
) map[string]*http.Client {
	timeouts := make(map[string]*http.Client)
	for _, domain := range dspEndpoints_v_2_5 {
		if _, ok := timeouts[domain]; !ok {
			client := NewFastHTTPClient()
			timeouts[domain] = client
		}
	}

	for _, domain := range dspEndpoints_mainstream_v_2_5 {
		if _, ok := timeouts[domain]; !ok {
			client := NewFastHTTPClient()
			timeouts[domain] = client
		}
	}

	timeouts[DEFAULT] = NewFastHTTPClient()

	return timeouts
}

func getDspHttpClients(dspDomain string, clients map[string]*http.Client) *http.Client {
	client, ok := clients[dspDomain]
	if !ok {
		return clients[DEFAULT]
	}
	return client
}

func NewFastHTTPClient() *http.Client {
	transport := &http.Transport{
		// ⚡ ДЛЯ 32 ЯДЕР И 250K СОЕДИНЕНИЙ:
		MaxIdleConns:        30000, // Было 500
		MaxIdleConnsPerHost: 5000,  // Было 100
		MaxConnsPerHost:     0,     // БЕЗ ЛИМИТА

		// ⚡ Оптимизация для большего числа соединений
		IdleConnTimeout: 120 * time.Second,

		// ⚡ Больше буферы для high-throughput
		ReadBufferSize:  32 * 1024,
		WriteBufferSize: 32 * 1024,

		// ⚡ Быстрее переподключения
		DialContext: (&net.Dialer{
			Timeout:   1 * time.Second, // Быстрее!
			KeepAlive: 120 * time.Second,
			DualStack: true,
		}).DialContext,
	}

	return &http.Client{
		Transport: transport,
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
