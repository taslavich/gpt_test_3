package dspRouterWeb

import (
	"net"
	"testing"

	"github.com/yl2chen/cidranger"
)

func TestAllowedUA(t *testing.T) {
	tests := []struct {
		name     string
		ua       string
		expected bool
	}{
		// Блокируемые UA
		{"Empty UA", "", false},
		{"Dash UA", "-", false},
		{"GSA UA", "Mozilla/5.0 (Linux; Android 10; SM-G973F) AppleWebKit/537.36 (KHTML, like Gecko) GSA/392.0.823086651 Mobile Safari/537.36", false},
		{"Headless Chrome", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) HeadlessChrome/91.0.4472.114 Safari/537.36", false},
		{"Python requests", "python-requests/2.25.1", false},
		{"Bot", "Googlebot/2.1", false},
		{"Suspicious Chrome version", "Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Mobile Safari/537.36", false},
		{"Short UA", "Chrome/1.0", false},
		{"Zero version", "Chrome/0.0.0.0", false},

		// Разрешаемые UA
		{"Normal Chrome Android", "Mozilla/5.0 (Linux; Android 10; SM-G973F) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.1 Mobile Safari/537.36", true},
		{"Normal Firefox", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:120.0) Gecko/20100101 Firefox/120.0", true},
		{"Normal Safari", "Mozilla/5.0 (iPhone; CPU iPhone OS 16_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.6 Mobile/15E148 Safari/604.1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := allowedUA(tt.ua)
			if result != tt.expected {
				t.Errorf("allowedUA(%q) = %v, want %v", tt.ua, result, tt.expected)
			}
		})
	}
}

func TestIsIPAllowed(t *testing.T) {
	// Создаем ranger с тестовыми CIDR
	ranger := cidranger.NewPCTrieRanger()

	// Добавляем запрещенные сети из ТЗ
	blockedNetworks := []string{
		"1.19.0.0/16",
		"1.32.128.0/18",
		"2.56.192.0/22",
		"2.57.122.0/24",
		"2.57.149.0/24",
		"2.57.232.0/22",
	}

	for _, network := range blockedNetworks {
		_, ipnet, err := net.ParseCIDR(network)
		if err != nil {
			t.Fatalf("Failed to parse network %s: %v", network, err)
		}
		ranger.Insert(cidranger.NewBasicRangerEntry(*ipnet))
	}

	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		// IPv6 - всегда разрешены
		{"IPv6 allowed", "2409:408c:beb9:31d1:7d37:92ff:fcb3:8541", true},
		{"IPv6 T-Mobile", "2607:fb91:1234:5678:abcd:ef12:3456:7890", true},

		// IPv4 в запрещенных сетях
		{"Blocked IP 1", "1.19.1.1", false},
		{"Blocked IP 2", "1.32.128.1", false},
		{"Blocked IP 3", "2.57.122.100", false},

		// IPv4 в разрешенных сетях
		{"Allowed IP 1", "8.8.8.8", true},
		{"Allowed IP 2", "192.168.1.1", true},
		{"Allowed IP 3", "10.0.0.1", true},

		// Невалидные IP
		{"Invalid IP", "invalid.ip.address", false},
		{"Empty IP", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isIPAllowed(tt.ip, ranger)
			if result != tt.expected {
				t.Errorf("isIPAllowed(%q) = %v, want %v", tt.ip, result, tt.expected)
			}
		})
	}
}
