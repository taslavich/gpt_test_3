package orchestratorWeb_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	pb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/bidEngine"
	routerpb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/dspRouter"
	orchpb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/orchestrator"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/pubuuid"
	be "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/bidEngine/service"
	beweb "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/bidEngine/web"
	routerweb "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/dspRouter/web"
	orchweb "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/orchestrator/web"
	ssp "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/web"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"
	stats "gitlab.com/twinbid-exchange/RTB-exchange/internal/types/clickhouse"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
)

func TestPublisherFlagProtobufRoundTrip(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, message := range []proto.Message{
			&orchpb.OrchestratorRequest_V2_5{IsPublisher: enabled}, &routerpb.DspRouterRequest_V2_5{IsPublisher: enabled}, &routerpb.DspRouterResponse_V2_5{IsPublisher: enabled}, &pb.BidEngineRequest_V2_5{IsPublisher: enabled},
		} {
			t.Run(fmt.Sprintf("%T/%t", message, enabled), func(t *testing.T) {
				wire, e := proto.Marshal(message)
				if e != nil {
					t.Fatal(e)
				}
				clone := message.ProtoReflect().New().Interface()
				if e = proto.Unmarshal(wire, clone); e != nil {
					t.Fatal(e)
				}
				field := clone.ProtoReflect().Descriptor().Fields().ByName("is_publisher")
				wantNumber := 10
				if _, ok := message.(*pb.BidEngineRequest_V2_5); ok {
					wantNumber = 13
				}
				if field == nil || int(field.Number()) != wantNumber || clone.ProtoReflect().Get(field).Bool() != enabled {
					t.Fatal("wire flag lost", clone)
				}
			})
		}
	}
}

// Actual HTTP SSP admission, all three real gRPC service handlers and protobuf
// wire transport, followed by the real BidEngine auction/ADM builder.
func TestSignedPublisherFlagThroughEntireGRPCChain(t *testing.T) {
	addr := os.Getenv("PUB_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("PUB_TEST_REDIS_ADDR unset")
	}
	ctx := context.Background()
	secret := strings.Repeat("x", 32)
	t.Setenv("PUB_INTERNAL_SECRET", secret)
	t.Setenv("PUB_UUID_FEEDS", "publisher-feed")
	redisClient := func(db int) *redis.Client {
		c := redis.NewClient(&redis.Options{Addr: addr, DB: db})
		t.Cleanup(func() { c.Close() })
		return c
	}
	source, adm, burl := redisClient(7), redisClient(10), redisClient(11)
	set := "chain:" + uuid.NewString()
	defer source.Del(ctx, set)
	destination := "https://advertiser.test/landing?keep=a&b=two"
	notification := "https://dsp.test/win?keep=%2F&value=a=b#part"
	dsp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request ortb.BidRequest
		if e := json.NewDecoder(r.Body).Decode(&request); e != nil {
			t.Error(e)
			w.WriteHeader(400)
			return
		}
		imp := request.GetImp()[0].GetId()
		price := float32(2)
		json.NewEncoder(w).Encode(&ortb.BidResponse{Seatbid: []*ortb.SeatBid{{Bid: []*ortb.Bid{{Impid: &imp, Price: &price, Adm: &destination, Nurl: &notification}}}}})
	}))
	defer dsp.Close()
	routes := &routerweb.FormatRoutesV25{POP: routerweb.FormatRouteV25{MainstreamEndpoints: config.MapStringToString{dsp.URL: "dsp.test"}}}
	router := routerweb.NewServer(nil, nil, nil, routes, nil, time.Second, routerweb.InitSspHttpClients(routes.EndpointSets()...), nil, nil, nil, nil, nil, nil, nil, nil, nil)
	engine := beweb.NewServer(0, []*redis.Client{source}, adm, burl, time.Hour, set, func(c context.Context, r *pb.BidEngineRequest_V2_5, p float32, ids map[string]string, routes *types.FormatPercentRoutesV25, logged bool, typic, host string) (*ortb.BidResponse, stats.UuidImpBidResponse, []string, []string) {
		return be.GetWinnerBidInternalWithRoutes_V_2_5(c, r, p, ids, routes, logged, typic, host)
	}, nil, "adm.test", nil)
	listener := bufconn.Listen(1024 * 1024)
	defer listener.Close()
	conn, e := grpc.NewClient("passthrough:///publisher-chain", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Close()
	var mu sync.Mutex
	seen := map[string][]bool{}
	ids := []string{}
	interceptor := func(c context.Context, in any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		var flag bool
		switch r := in.(type) {
		case *orchpb.OrchestratorRequest_V2_5:
			flag = r.GetIsPublisher()
			mu.Lock()
			for _, id := range r.GetImpIdUuid() {
				ids = append(ids, id)
			}
			mu.Unlock()
		case *routerpb.DspRouterRequest_V2_5:
			flag = r.GetIsPublisher()
		case *pb.BidEngineRequest_V2_5:
			flag = r.GetIsPublisher()
		}
		mu.Lock()
		seen[info.FullMethod] = append(seen[info.FullMethod], flag)
		mu.Unlock()
		result, err := next(c, in)
		if r, ok := result.(*routerpb.DspRouterResponse_V2_5); ok && r.GetIsPublisher() != flag {
			t.Error("Router response flag lost")
		}
		return result, err
	}
	server := grpc.NewServer(grpc.UnaryInterceptor(interceptor))
	defer server.Stop()
	routerpb.RegisterDspRouterServiceServer(server, router)
	pb.RegisterBidEngineServiceServer(server, engine)
	orchpb.RegisterOrchestratorServiceServer(server, orchweb.NewServer(pb.NewBidEngineServiceClient(conn), routerpb.NewDspRouterServiceClient(conn), time.Second, time.Second))
	go server.Serve(listener)
	defer func() {
		for _, id := range ids {
			for _, c := range []*redis.Client{source, adm, burl} {
				c.Del(ctx, id, pubuuid.Marker(id))
			}
		}
	}()
	feedPath := filepath.Join(t.TempDir(), "feeds.json")
	if e = os.WriteFile(feedPath, []byte(`{"publisher-feed":"pub-ssp","ordinary-feed":"ordinary-ssp"}`), 0600); e != nil {
		t.Fatal(e)
	}
	feeds, e := ssp.NewFeedStore(feedPath)
	if e != nil {
		t.Fatal(e)
	}
	mux := chi.NewRouter()
	ssp.InitHttpRoutes(ctx, mux, []*redis.Client{source}, set, func(string) (bool, error) { return false, nil }, func(string) (string, uint32, error) { return "US", 0, nil }, orchpb.NewOrchestratorServiceClient(conn), 2*time.Second, &ssp.FormatFeedRoutesV25{POP: ssp.FormatFeedRouteV25{Mainstream: feeds}}, ssp.NewWorkStatus(true, true), &utils.SiteIdsAndDomains{}, map[string]string{"DEFAULT": "en"}, nil, "", ssp.NewIPLimitStore())
	for _, kind := range []string{"publisher", "ordinary", "bad-signature"} {
		t.Run(kind, func(t *testing.T) {
			requestID, imp, ip, ua, site, domain := uuid.NewString(), "1", "1.1.1.1", "Mozilla/5.0 desktop browser", "site", "example.test"
			request := &ortb.BidRequest{Id: &requestID, Imp: []*ortb.Imp{{Id: &imp}}, Site: &ortb.Site{Id: &site, Domain: &domain}, Device: &ortb.Device{Ip: &ip, Ua: &ua}}
			body, e := json.Marshal(request)
			if e != nil {
				t.Fatal(e)
			}
			feed := "publisher-feed"
			if kind == "ordinary" {
				feed = "ordinary-feed"
			}
			httpReq := httptest.NewRequest("POST", "/bid_v_2_5_mc?feed="+feed, strings.NewReader(string(body)))
			httpReq.Header.Set("Content-Type", "application/json")
			if kind != "ordinary" {
				id := uuid.NewString()
				stamp := fmt.Sprint(time.Now().Unix())
				signature := pubuuid.Sign(secret, id, feed, stamp, request)
				source.HSet(ctx, id, "pub_user_id", "owner", "ad_code_id", "code", "request_error", "0")
				source.HSet(ctx, pubuuid.Marker(id), "state", "prepared", "signature", signature)
				defer source.Del(ctx, id, pubuuid.Marker(id))
				httpReq.Header.Set(pubuuid.Header, id)
				httpReq.Header.Set(pubuuid.TimeHeader, stamp)
				if kind == "bad-signature" {
					signature = "invalid"
				}
				httpReq.Header.Set(pubuuid.SignatureHeader, signature)
			}
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, httpReq)
			if kind == "bad-signature" {
				if recorder.Code != 403 {
					t.Fatal("signature not rejected", recorder.Code, recorder.Body.String())
				}
				return
			}
			if recorder.Code != 200 {
				t.Fatal(recorder.Code, recorder.Body.String())
			}
			var response ortb.BidResponse
			if e = json.Unmarshal(recorder.Body.Bytes(), &response); e != nil {
				t.Fatal(e)
			}
			bid := response.GetSeatbid()[0].GetBid()[0]
			u, e := url.Parse(bid.GetAdm())
			if e != nil {
				t.Fatal(e)
			}
			if kind == "publisher" {
				if u.Query().Get("p") != "1" || u.Query().Get("n") != notification {
					t.Fatal("flag/NURL lost in full chain", u)
				}
			} else if u.Query().Has("p") || u.Query().Has("n") {
				t.Fatal("ordinary feed marked", u)
			}
			if u.Query().Get("url") != destination {
				t.Fatal("destination changed", u)
			}
		})
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 3 {
		t.Fatal("missing gRPC hop", seen)
	}
	for method, flags := range seen {
		if len(flags) != 2 || !flags[0] || flags[1] {
			t.Fatal("flag lost or invalid signature reached upstream", method, flags)
		}
	}
}

type flagRouterClient struct {
	routerpb.DspRouterServiceClient
	response *routerpb.DspRouterResponse_V2_5
	request  *routerpb.DspRouterRequest_V2_5
}

func (c *flagRouterClient) GetBids_V2_5(_ context.Context, r *routerpb.DspRouterRequest_V2_5, _ ...grpc.CallOption) (*routerpb.DspRouterResponse_V2_5, error) {
	c.request = r
	return c.response, nil
}

type flagEngineClient struct {
	pb.BidEngineServiceClient
	request *pb.BidEngineRequest_V2_5
}

func (c *flagEngineClient) GetWinnerBid_V2_5(_ context.Context, r *pb.BidEngineRequest_V2_5, _ ...grpc.CallOption) (*pb.BidEngineResponse_V2_5, error) {
	c.request = r
	return &pb.BidEngineResponse_V2_5{Code: 204}, nil
}
func TestOrchestratorUsesReturnedRouterPublisherFlag(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		router := &flagRouterClient{response: &routerpb.DspRouterResponse_V2_5{IsPublisher: enabled}}
		engine := &flagEngineClient{}
		server := orchweb.NewServer(engine, router, time.Second, time.Second)
		_, e := server.GetWinnerBid_V2_5(context.Background(), &orchpb.OrchestratorRequest_V2_5{BidRequest: &ortb.BidRequest{}, IsPublisher: !enabled})
		if e != nil || router.request == nil || engine.request == nil {
			t.Fatal(e)
		}
		if router.request.GetIsPublisher() != !enabled || engine.request.GetIsPublisher() != enabled {
			t.Fatal("Orchestrator bypassed returned Router flag")
		}
	}
}
