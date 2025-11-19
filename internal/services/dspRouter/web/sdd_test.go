package dspRouterWeb

import (
	"fmt"
	"testing"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

func TestSiteGetIdSafety(t *testing.T) {
	var site *ortb_V2_5.Site
	_ = site.GetId() // Не должно паниковать
	fmt.Print("ffff")
}
