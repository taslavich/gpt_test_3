package bidEngineWeb

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	pb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/bidEngine"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	stats "gitlab.com/twinbid-exchange/RTB-exchange/internal/types/clickhouse"
)

func TestPublisherDSPNURLOnlyExtendsExistingADMHashWithoutTTLChange(t *testing.T) {
	for _, kind := range []string{"dsp", "adv", "ordinary", "missing-uuid"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			id, imp := uuid.NewString(), "1"
			req := &pb.BidEngineRequest_V2_5{ImpIdUuid: map[string]string{imp: id}, Format: "POP", SspDomain: "mapped-domain"}
			server := &Server{}
			configureCallbackServer(t, server, req)
			if kind != "missing-uuid" {
				if e := utils.WriteUUIDKeyToRedis(ctx, server.redisAdmClient, id, 31*time.Second); e != nil {
					t.Fatal(e)
				}
			}
			before := server.redisAdmClient.Do(ctx, "PEXPIRETIME", id).Val()
			source := server.redisClients[0]
			source.HSet(ctx, id, "pub_user_id", "owner", "ad_code_id", "code", "SPP_DOMAIN", "feed-token", "FORMAT", "POP")
			if kind != "ordinary" {
				source.HSet(ctx, id+":pub-uuid", "state", "adapter")
			}
			defer source.Del(ctx, id+":pub-uuid")
			destination, nurl := "https://advertiser.test/landing", "http://127.0.0.1/dsp/win?keep=%2F&x=two"
			adm, burl := utils.WrapURL("adm.test", destination, id, "POP"), utils.WrapBurlNurlURL("adm.test", nurl, id, "POP")
			bid := &ortb.Bid{Impid: &imp, Adm: &adm, Burl: &burl}
			response := &ortb.BidResponse{Seatbid: []*ortb.SeatBid{{Bid: []*ortb.Bid{bid}}}}
			wins := stats.UuidImpBidResponse{}
			if kind == "adv" {
				domain := "adv"
				wins[id] = &stats.Bid{WinDspDomain: &domain}
			}
			server.markPublisherPOP(ctx, req, response, wins)
			after := server.redisAdmClient.Do(ctx, "PEXPIRETIME", id).Val()
			if before != after {
				t.Fatal("UUID TTL changed", before, after)
			}
			fields := server.redisAdmClient.HGetAll(ctx, id).Val()
			if kind == "dsp" {
				if fields["uuid"] != id || fields["pub_dsp_nurl"] != nurl || len(fields) != 2 {
					t.Fatal(fields)
				}
			} else if fields["pub_dsp_nurl"] != "" {
				t.Fatal("NURL written for wrong winner/missing key", fields)
			}
			if kind == "missing-uuid" && len(fields) != 0 {
				t.Fatal("new UUID key created")
			}
			if server.redisBurlClient.Exists(ctx, id).Val() != 0 {
				t.Fatal("BURL DB used for DSP NURL")
			}
			if source.HGet(ctx, id, "pub_dsp_nurl").Val() != "" {
				t.Fatal("ORTB used for DSP NURL storage")
			}
		})
	}
}

type failDSPNURLWrite struct{}

func (failDSPNURLWrite) DialHook(next redis.DialHook) redis.DialHook { return next }
func (failDSPNURLWrite) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (failDSPNURLWrite) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if cmd.Name() == "eval" && strings.Contains(cmd.Args()[1].(string), "pub_dsp_nurl") {
			return errors.New("injected NURL write failure")
		}
		return next(ctx, cmd)
	}
}
