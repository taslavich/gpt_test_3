package kafka_loader

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	events "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/buffer"
	c "gitlab.com/twinbid-exchange/RTB-exchange/internal/pubcontract"
	pub "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/pub"
	"google.golang.org/protobuf/proto"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPublisherFieldsTravelThroughExistingOrtbMessage(t *testing.T) {
	values := make([]interface{}, 36)
	values[25] = "advertiser"
	values[29] = "publisher"
	values[30] = "code"
	values[31] = "zone"
	values[32] = "request"
	values[33] = "https://example.test"
	values[34] = "1"
	values[35] = "frequency"
	msg, ok, e := buildOrtbKafkaMessage(0, "00000000-0000-0000-0000-000000000001", values)
	if e != nil || !ok {
		t.Fatal(e)
	}
	var v events.OrtbEvent
	if e = proto.Unmarshal(msg.Value, &v); e != nil {
		t.Fatal(e)
	}
	if v.WinUserId != "advertiser" || v.PubUserId != "publisher" || v.AdCodeId != "code" || v.ZoneId != "zone" || !v.RequestError || v.ErrorReason != "frequency" {
		t.Fatalf("lost fields: %+v", &v)
	}
}
func TestOldOrtbMessageStillAccepted(t *testing.T) {
	values := make([]interface{}, 29)
	values[25] = "advertiser"
	msg, ok, e := buildOrtbKafkaMessage(0, "00000000-0000-0000-0000-000000000001", values)
	if e != nil || !ok {
		t.Fatal(e)
	}
	var v events.OrtbEvent
	proto.Unmarshal(msg.Value, &v)
	if v.PubUserId != "" || v.RequestError || v.WinUserId != "advertiser" {
		t.Fatal(&v)
	}
}

func TestRealRedisPublisherErrorFeedsExistingMessage(t *testing.T) {
	addr := os.Getenv("PUB_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("PUB_TEST_REDIS_ADDR unset")
	}
	store := redis.NewClient(&redis.Options{Addr: addr, DB: 11})
	defer store.Close()
	secret := strings.Repeat("x", 32)
	cfg := pub.Config{PublicURL: "https://pub.example.test", InternalSecret: secret, DeliverySecret: secret, RTBAdultURL: "https://rtb.example.test/bid_v_2_5", RTBMainstreamURL: "https://rtb.example.test/bid_v_2_5_mc", AdultFeed: "adult", MainstreamFeed: "mainstream", CallbackOrigins: "https://adm.example.test", RedisAddrs: addr, ORTBAddrs: addr, ORTBDB: 12, ORTBReadySet: "publisher-test-ready", MaxInflight: 4, RTBTimeout: time.Second, TelemetryMaxBytes: 1024, ConfirmedOpenEnabled: true}
	s, e := pub.NewServer(cfg, store, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer s.CloseORTB()
	code := c.Code{ID: uuid.NewString(), UserID: uuid.NewString(), ShortCode: "tb_" + uuid.NewString(), Name: "test", Format: "popunder", TrafficType: "mainstream", Device: "all", Active: true, TriggerClick: 1, FrequencyMinutes: 1, Epoch: "test"}
	store.Set(context.Background(), c.ReadyKey, "test", time.Minute)
	b, _ := json.Marshal(code)
	store.HSet(context.Background(), c.CodeKey(code.ShortCode), "data", b)
	defer store.Del(context.Background(), c.ReadyKey, c.CodeKey(code.ShortCode))
	s.ORTB[0].Del(context.Background(), cfg.ORTBReadySet)
	in := pub.Request{ID: uuid.NewString(), Code: code.ShortCode, PageURL: ""}
	raw, _ := json.Marshal(in)
	req := httptest.NewRequest("POST", "/request", bytes.NewReader(raw))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatal(w.Code, w.Body.String())
	}
	ids, e := s.ORTB[0].SPopN(context.Background(), cfg.ORTBReadySet, 100).Result()
	if e != nil || len(ids) != 1 {
		t.Fatal(ids, e)
	}
	id := ids[0]
	defer s.ORTB[0].Del(context.Background(), id, id+":pub-uuid")
	values, e := s.ORTB[0].HMGet(context.Background(), id, ortbHMGetFields...).Result()
	if e != nil {
		t.Fatal(e)
	}
	msg, ok, e := buildOrtbKafkaMessage(0, id, values)
	if e != nil || !ok {
		t.Fatal(e)
	}
	var event events.OrtbEvent
	if e = proto.Unmarshal(msg.Value, &event); e != nil {
		t.Fatal(e)
	}
	if event.Uuid != id || event.PubUserId != code.UserID || event.AdCodeId != code.ID || event.WinUserId != "" || !event.RequestError || event.Code != 400 || event.EventTimeMs == 0 || event.Format != "POP" || event.Typic != "MAINSTREAM" {
		t.Fatal(&event)
	}
}
func TestPublisherRTBErrorIsClassifiedAtSerialization(t *testing.T) {
	values := make([]interface{}, 36)
	values[6] = "503"
	values[29] = "publisher"
	values[34] = "0"
	msg, ok, e := buildOrtbKafkaMessage(0, uuid.NewString(), values)
	if e != nil || !ok {
		t.Fatal(e)
	}
	var event events.OrtbEvent
	proto.Unmarshal(msg.Value, &event)
	if !event.RequestError || event.ErrorReason != "rtb_status_503" {
		t.Fatal(&event)
	}
	values[29] = nil
	msg, _, _ = buildOrtbKafkaMessage(0, uuid.NewString(), values)
	proto.Unmarshal(msg.Value, &event)
	if event.RequestError || event.ErrorReason != "" {
		t.Fatal("ordinary SSP changed", &event)
	}
}
