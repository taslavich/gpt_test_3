package config

import (
	"log"
	"testing"
)

func TestMain(t *testing.T) {

	adid := "clickadilla.com=http://pop.zog.link/bid-request?token=h6dKfdh544FHD83,kadam.net=http://pop-48702.daortb.com/api/rtb-pops/item?sourceId=59738&api-key=xvKZ-_oewvADCb2RR0W6bgp_EdLEKCLj"
	testAdm := MapStringToString{}
	err := testAdm.SetValue(adid)
	if err != nil {
		log.Fatalf("PIZDEC")
	}

	for k, v := range testAdm {
		log.Printf("--%s--", k)
		log.Printf("--%s--", v)
	}
}
