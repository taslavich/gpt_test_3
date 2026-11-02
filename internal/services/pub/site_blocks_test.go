package pub

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	c "gitlab.com/twinbid-exchange/RTB-exchange/internal/pubcontract"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/pubuuid"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/ua"
)

func TestPublisherBlocksUseReadySiteIDAndPreserveTelemetry(t *testing.T) {
	for _, mode := range []string{"blocked", "other-code-owner", "unblocked", "other-site", "case-sensitive", "domain-not-ID", "old-policy-only", "wrong-epoch"} {
		t.Run(mode, func(t *testing.T) {
			s, code := fixture(t)
			ctx := context.Background()
			in := input(code)
			site := c.SiteID("example.test")
			zone := c.ZoneID(site, code.ID)
			field := site
			value := "1"
			want := 200
			switch mode {
			case "blocked":
				want = 403
			case "other-code-owner":
				code.ID = uuid.NewString()
				code.UserID = uuid.NewString()
				code.ShortCode = "tb_second"
				b, _ := json.Marshal(code)
				if err := s.Redis.HSet(ctx, c.CodeKey(code.ShortCode), "data", b).Err(); err != nil {
					t.Fatal(err)
				}
				in = input(code)
				zone = c.ZoneID(site, code.ID)
				want = 403
			case "unblocked":
				value = "0"
			case "other-site":
				field = c.SiteID("another.example.test")
			case "case-sensitive":
				field = strings.ToLower(site)
				if field == site {
					t.Fatal("case fixture must differ")
				}
			case "domain-not-ID":
				field = "example.test"
			case "old-policy-only":
				s.Redis.Del(ctx, c.SiteBlocksKey)
				s.Redis.HSet(ctx, c.Prefix+"{control}:blocks", "epoch", "test", site, "0")
				want = 503
			case "wrong-epoch":
				s.Redis.HSet(ctx, c.SiteBlocksKey, "epoch", "old")
				want = 503
			}
			if err := s.Redis.HSet(ctx, c.SiteBlocksKey, field, value).Err(); err != nil {
				t.Fatal(err)
			}
			var upstreamSite string
			s.HTTP.Transport = transport(func(r *http.Request) (*http.Response, error) {
				var request ortb.BidRequest
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Fatal(err)
				}
				upstreamSite = request.GetSite().GetId()
				// Simulate the existing SSP Adapter admission/ORTB commit. A 204
				// response alone does not commit Publisher's prewritten HASH.
				id := r.Header.Get(pubuuid.Header)
				if err := pubuuid.Claim(ctx, s.ORTB, id, r.Header.Get(pubuuid.SignatureHeader)); err != nil {
					t.Fatal(err)
				}
				if err := utils.WriteStatsOrtb(ctx, s.ORTB, id, true, "POP", "MAINSTREAM", "ssp", "1.1.1.1", "", "en", "US", 0, 204, ua.UAFields{}, upstreamSite, "example.test", 0); err != nil {
					t.Fatal(err)
				}
				if err := utils.AddUUIDToRedisSet(ctx, s.ORTB, s.Config.ORTBReadySet, id, true); err != nil {
					t.Fatal(err)
				}
				return &http.Response{StatusCode: 204, Body: http.NoBody}, nil
			})
			w := send(s, in)
			if w.Code != want {
				t.Fatal(mode, w.Code, w.Body.String())
			}
			if want == 200 {
				if upstreamSite != site || s.RTBCalls.Load() != 1 {
					t.Fatal("Site ID missing on RTB wire", upstreamSite)
				}
			} else if s.RTBCalls.Load() != 0 {
				t.Fatal("blocked/unready site reached auction")
			}
			ids, err := s.ORTB[0].SMembers(ctx, s.Config.ORTBReadySet).Result()
			if err != nil || len(ids) != 1 {
				t.Fatal(ids, err)
			}
			fields := s.ORTB[0].HGetAll(ctx, ids[0]).Val()
			if fields["SITE_ID"] != site || fields["zone_id"] != zone || !c.ValidShortID(site) || !c.ValidShortID(zone) {
				t.Fatal("short identity missing in ORTB telemetry", fields)
			}
		})
	}
}
