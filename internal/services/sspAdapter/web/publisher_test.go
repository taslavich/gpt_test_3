package sppAdapterWeb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ggicci/httpin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	orchestratorProto "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/orchestrator"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/pubuuid"
	"google.golang.org/grpc"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestTrustedUUIDAdmission(t *testing.T) {
	addr := os.Getenv("PUB_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("PUB_TEST_REDIS_ADDR unset")
	}
	secret := strings.Repeat("x", 32)
	t.Setenv("PUB_INTERNAL_SECRET", secret)
	t.Setenv("PUB_UUID_FEEDS", "adult,mainstream")
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 13})
	defer client.Close()
	clients := []*redis.Client{client}
	id := uuid.NewString()
	stamp := fmt.Sprint(time.Now().Unix())
	reqID := "request"
	impID := "1"
	payload := &postBidPayloadV25{BidRequest: &ortb.BidRequest{Id: &reqID, Imp: []*ortb.Imp{{Id: &impID}}}}
	in := &postBidRequest_V2_5{Feed: "adult", Payload: payload}
	req := httptest.NewRequest("POST", "/bid_v_2_5?feed=adult", nil)
	req.Header.Set(pubuuid.Header, id)
	req.Header.Set(pubuuid.TimeHeader, stamp)
	sig := pubuuid.Sign(secret, id, in.Feed, stamp, payload.BidRequest)
	req.Header.Set(pubuuid.SignatureHeader, sig)
	if _, e := trustedPublisherUUID(context.Background(), req, in, "POP", clients); e == nil {
		t.Fatal("absent HASH accepted")
	}
	client.HSet(context.Background(), id, "pub_user_id", "owner")
	client.HSet(context.Background(), pubuuid.Marker(id), "state", "prepared", "signature", sig)
	defer client.Del(context.Background(), id, pubuuid.Marker(id))
	got, e := trustedPublisherUUID(context.Background(), req, in, "POP", clients)
	if e != nil || got != id {
		t.Fatal(got, e)
	}
	if _, e := trustedPublisherUUID(context.Background(), req, in, "POP", clients); e == nil {
		t.Fatal("replay")
	}
	if client.HGet(context.Background(), id, "pub_user_id").Val() != "owner" {
		t.Fatal("metadata touched")
	}
	in.Feed = "ordinary"
	if got, e := trustedPublisherUUID(context.Background(), req, in, "POP", clients); e != nil || got != "" {
		t.Fatal("ordinary SSP changed", got, e)
	}
	req.Header.Del(pubuuid.Header)
	if got, e := trustedPublisherUUID(context.Background(), req, in, "POP", clients); e != nil || got != "" {
		t.Fatal(got, e)
	}
	in.Feed = "adult"
	if _, e := trustedPublisherUUID(context.Background(), req, in, "POP", clients); e == nil {
		t.Fatal("unsigned pub feed")
	}
	req.Header.Set(pubuuid.Header, id)
	payload.Imp = append(payload.Imp, &ortb.Imp{Id: &impID})
	req.Header.Set(pubuuid.SignatureHeader, pubuuid.Sign(secret, id, in.Feed, stamp, payload.BidRequest))
	if _, e := trustedPublisherUUID(context.Background(), req, in, "POP", clients); e == nil {
		t.Fatal("two imps")
	}
	payload.Imp = payload.Imp[:1]
	// JSON-equivalent requests sign the same generated protobuf value.
	b, _ := json.Marshal(payload.BidRequest)
	var clone ortb.BidRequest
	json.Unmarshal(b, &clone)
	if pubuuid.Sign(secret, id, in.Feed, stamp, &clone) != sig {
		t.Fatal("canonical request changed")
	}
}

type publisherOrchestrator struct {
	orchestratorProto.OrchestratorServiceClient
	status int32
	id     string
	t      *testing.T
}

func (o publisherOrchestrator) GetWinnerBid_V2_5(ctx context.Context, in *orchestratorProto.OrchestratorRequest_V2_5, opts ...grpc.CallOption) (*orchestratorProto.OrchestratorResponse_V2_5, error) {
	if in.ImpIdUuid["1"] != o.id || in.GetTypic() != "MAINSTREAM" || !in.GetIsPublisher() {
		o.t.Fatal("UUID/traffic changed", in)
	}
	if o.status == 500 {
		return nil, errors.New("upstream RPC failed")
	}
	res := &orchestratorProto.OrchestratorResponse_V2_5{Code: o.status, ImpIdUuidClone: in.ImpIdUuid}
	if o.status == 200 {
		imp := "1"
		adm := "https://adm.example.test/adm?url=https%3A%2F%2Fexample.test"
		res.BidResponse = &ortb.BidResponse{Seatbid: []*ortb.SeatBid{{Bid: []*ortb.Bid{{Impid: &imp, Adm: &adm}}}}}
	}
	return res, nil
}
func TestPublisherActualAdapterCommitPaths(t *testing.T) {
	addr := os.Getenv("PUB_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("PUB_TEST_REDIS_ADDR unset")
	}
	t.Setenv("PUB_INTERNAL_SECRET", strings.Repeat("x", 32))
	t.Setenv("PUB_UUID_FEEDS", "mainstream")
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 13})
	defer client.Close()
	feeds := &FeedStore{}
	feeds.runtime.Store(FeedMap{"mainstream": "pub-ssp"})
	for _, status := range []int32{200, 204, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			id := uuid.NewString()
			set := id + ":ready"
			defer client.Del(context.Background(), id, pubuuid.Marker(id), set)
			reqID, impID, ip, userAgent, siteID, domain := "r", "1", "1.1.1.1", "Mozilla/5.0 desktop browser", "site", "example.test"
			payload := &postBidPayloadV25{BidRequest: &ortb.BidRequest{Id: &reqID, Imp: []*ortb.Imp{{Id: &impID}}, Site: &ortb.Site{Id: &siteID, Domain: &domain}, Device: &ortb.Device{Ip: &ip, Ua: &userAgent}}}
			in := &postBidRequest_V2_5{Feed: "mainstream", Payload: payload}
			stamp := fmt.Sprint(time.Now().Unix())
			sig := pubuuid.Sign(os.Getenv("PUB_INTERNAL_SECRET"), id, in.Feed, stamp, payload.BidRequest)
			client.HSet(context.Background(), id, "pub_user_id", "owner", "request_error", "0")
			client.HSet(context.Background(), pubuuid.Marker(id), "state", "prepared", "signature", sig)
			req := httptest.NewRequest("POST", "/bid_v_2_5_mc?feed=mainstream", nil)
			req.Header.Set(pubuuid.Header, id)
			req.Header.Set(pubuuid.TimeHeader, stamp)
			req.Header.Set(pubuuid.SignatureHeader, sig)
			req = req.WithContext(context.WithValue(req.Context(), httpin.Input, in))
			w := httptest.NewRecorder()
			var counter uint64
			postBid_V2_5(context.Background(), w, req, []*redis.Client{client}, set, func(string) (bool, error) { return false, nil }, func(string) (string, uint32, error) { return "US", 0, nil }, publisherOrchestrator{status: status, id: id, t: t}, time.Second, feeds, &counter, "MAINSTREAM", "POP", &utils.SiteIdsAndDomains{}, map[string]string{"DEFAULT": "en"}, nil, "", NewIPLimitStore())
			if w.Code != int(status) {
				t.Fatal(w.Code, w.Body.String())
			}
			if n := client.SCard(context.Background(), set).Val(); n != 1 {
				t.Fatal("not one adapter commit", n)
			}
			if v := client.HGet(context.Background(), id, "CODE").Val(); v != fmt.Sprint(status) {
				t.Fatal("wrong status", v)
			}
			if v := client.HGet(context.Background(), id, "pub_user_id").Val(); v != "owner" {
				t.Fatal(v)
			}
		})
	}
}
