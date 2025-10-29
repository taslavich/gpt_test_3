package bidEngine

import (
	"log"
	"net/url"
	"testing"
)

func TestMain(t *testing.T) {

	addr := "http://twinbidexchange.com:8086/adm"
	encode := url.QueryEscape(addr)

	log.Println(encode)
	gotAdrr, err := url.QueryUnescape(encode)
	if err != nil {
		log.Fatalf("PIZDEC 2")
	}

	if addr == gotAdrr {
		log.Println("OK")
	}
}
