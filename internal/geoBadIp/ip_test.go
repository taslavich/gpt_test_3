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
		fmt.Println(err)
	}

	ip := net.ParseIP("204.0.113.45")
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
		fmt.Println(err)
	}

	fmt.Println(rec.Country.ISOCode)
}
