package auction

import (
	"fmt"
	"net"
	"strings"

	"github.com/oschwald/maxminddb-golang"
)

const twinBidVPNDatabaseType = "TwinBid-VPN-Traffic-IPv4"

type VPNClassifier interface {
	IsVPN(ip string) (bool, error)
}

type vpnRecord struct {
	VPN bool `maxminddb:"vpn"`
}

type VPNStore struct {
	db *maxminddb.Reader
}

func NewVPNStore(path string) (*VPNStore, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("VPN MMDB path is empty")
	}
	reader, err := maxminddb.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open VPN MMDB %q: %w", path, err)
	}
	if reader.Metadata.DatabaseType != twinBidVPNDatabaseType {
		_ = reader.Close()
		return nil, fmt.Errorf("unexpected VPN MMDB database_type %q, want %q", reader.Metadata.DatabaseType, twinBidVPNDatabaseType)
	}
	if reader.Metadata.IPVersion != 4 {
		_ = reader.Close()
		return nil, fmt.Errorf("unexpected VPN MMDB ip_version %d, want 4", reader.Metadata.IPVersion)
	}
	return &VPNStore{db: reader}, nil
}

func (s *VPNStore) Close() {
	if s != nil && s.db != nil {
		_ = s.db.Close()
	}
}

// IsVPN performs one in-memory MaxMind-tree lookup for an IPv4 address.
// The TwinBid VPN MMDB stores vpn=true on every prefix that must be excluded
// when a campaign has block_vpn=true. Empty IP and IPv6 do not match this
// IPv4-only database and therefore return false.
func (s *VPNStore) IsVPN(ipValue string) (bool, error) {
	if s == nil || s.db == nil {
		return false, fmt.Errorf("VPN MMDB is not initialized")
	}
	ipValue = strings.TrimSpace(ipValue)
	if ipValue == "" {
		return false, nil
	}
	ip := net.ParseIP(ipValue)
	if ip == nil {
		return false, fmt.Errorf("invalid device.ip %q", ipValue)
	}
	ipv4 := ip.To4()
	if ipv4 == nil {
		return false, nil
	}
	var record vpnRecord
	if err := s.db.Lookup(ipv4, &record); err != nil {
		return false, fmt.Errorf("lookup VPN MMDB for %s: %w", ipv4.String(), err)
	}
	return record.VPN, nil
}
