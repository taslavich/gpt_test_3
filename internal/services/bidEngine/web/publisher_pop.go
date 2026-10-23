package bidEngineWeb

import (
	"context"
	"fmt"
	"log"
	"net/url"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	pb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/bidEngine"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/pubuuid"
	shard "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/redis"
	stats "gitlab.com/twinbid-exchange/RTB-exchange/internal/types/clickhouse"
)

// Only authenticated Publisher traffic already attributed in ORTB receives p=1.
// This does not participate in the auction or reject a finalized winner.
func (s *Server) markPublisherPOP(ctx context.Context, req *pb.BidEngineRequest_V2_5, response *ortb.BidResponse, wins stats.UuidImpBidResponse) {
	if req.GetFormat() != constants.POP {
		return
	}
	for _, seat := range response.GetSeatbid() {
		for _, bid := range seat.GetBid() {
			if bid == nil {
				continue
			}
			id := req.GetImpIdUuid()[bid.GetImpid()]
			client, _, err := shard.SelectShard(s.redisClients, id)
			if err != nil {
				log.Printf("Publisher POP attribution uuid=%s: %v", id, err)
				continue
			}
			pipe := client.Pipeline()
			fields := pipe.HMGet(ctx, id, "pub_user_id", "ad_code_id", "SPP_DOMAIN", "FORMAT")
			state := pipe.HGet(ctx, pubuuid.Marker(id), "state")
			// The signed feed token in prewritten ORTB is not the mapped SSP domain.
			// Its claimed marker authenticates Publisher origin independently of advertiser user_id.
			_, _ = pipe.Exec(ctx)
			values, e := fields.Result()
			if e != nil {
				log.Printf("Publisher POP attribution uuid=%s: %v", id, e)
				continue
			}
			text := func(i int) string { v, _ := values[i].(string); return v }
			if text(0) == "" || text(1) == "" || text(2) == "" || text(3) != constants.POP || state.Val() != "adapter" {
				continue
			}
			adm, e := url.Parse(bid.GetAdm())
			if e != nil {
				log.Printf("Publisher POP ADM uuid=%s: %v", id, e)
				continue
			}
			q := adm.Query()
			q.Set("p", "1")
			adm.RawQuery = q.Encode()
			marked := adm.String()
			bid.Adm = &marked
			notification := ""
			if burl, e := url.Parse(bid.GetBurl()); e == nil {
				notification = burl.Query().Get("url")
			}
			kind := "dsp"
			if win := wins[id]; win != nil && win.WinDspDomain != nil && *win.WinDspDomain == "adv" {
				kind = "adv"
			}
			if kind == "adv" {
				continue
			}
			var storeErr error
			if notification == "" {
				storeErr = fmt.Errorf("Publisher DSP NURL missing uuid=%s", id)
			} else if s.redisAdmClient == nil {
				storeErr = fmt.Errorf("Publisher DSP ADM Redis unavailable uuid=%s", id)
			} else {
				// Augment ONLY the UUID hash already authorized above by
				// WriteUUIDKeyToRedis. Never create a key or change its TTL.
				var saved int
				saved, storeErr = s.redisAdmClient.Eval(ctx, `if redis.call('HGET',KEYS[1],'uuid')~=ARGV[1] then return 0 end
                  redis.call('HSET',KEYS[1],'pub_dsp_nurl',ARGV[2]);return 1`, []string{id}, id, notification).Int()
				if storeErr == nil && saved != 1 {
					storeErr = fmt.Errorf("Publisher DSP UUID hash unavailable uuid=%s", id)
				}
			}
			if storeErr != nil {
				log.Printf("Publisher DSP NURL: %v", storeErr)
				if s.redisWriteErrorMonitor != nil {
					s.redisWriteErrorMonitor.RecordForURL(storeErr, req.SspUrl)
				}
			}
		}
	}
}
