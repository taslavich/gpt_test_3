package dspRouterWeb

import (
	"net"
	"net/http"
	"time"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
)

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
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second, // Уменьшаем с 30s до 5s
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,

		// Пул соединений - ОБЯЗАТЕЛЬНО оставить!
		MaxIdleConns:        500,
		MaxIdleConnsPerHost: 100,
		MaxConnsPerHost:     200,
		IdleConnTimeout:     30 * time.Second, // ОБЯЗАТЕЛЬНО оставить!

		DisableCompression: true,
		ForceAttemptHTTP2:  false,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   timeout, // Главный таймаут
	}
}
