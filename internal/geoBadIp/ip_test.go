package geoBadIp

import (
	"fmt"
	"log"
	"net"
	"testing"
)

func TestDSPV25Rules(t *testing.T) {
	service, err := NewGeoIPService("GeoIP2_City.mmdb")
	if err != nil {
		t.Skipf("GeoIP fixture unavailable: %v", err)
	}

	ip := net.ParseIP("2a00:1450:4007:0812::200e")
	if ip == nil {
		log.Println(fmt.Errorf(
			"%w %s",
			BadIpFormatError,
			ip,
		))
	}

	var rec GeoIPRecord
	err = service.db.Lookup(ip, &rec)
	if err != nil {
		t.Skipf("GeoIP fixture unavailable: %v", err)
	}

	fmt.Printf("--|%s|--\n", rec.Country.ISOCode)
}
