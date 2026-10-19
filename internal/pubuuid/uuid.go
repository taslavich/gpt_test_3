// Package pubuuid authenticates and claims an existing ORTB UUID. It carries no publisher metadata.
package pubuuid

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	shard "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/redis"
	"strconv"
	"strings"
	"time"
)

const Header = "X-TwinBid-RTB-UUID"
const SignatureHeader = "X-TwinBid-RTB-UUID-Signature"
const TimeHeader = "X-TwinBid-RTB-UUID-Time"
const TTL = 24 * time.Hour

func Marker(id string) string { return id + ":pub-uuid" }

// Bind the UUID to feed, timestamp and the complete parsed OpenRTB request.
// encoding/json on the same generated BidRequest is deterministic in both processes.
func Sign(secret, id, feed, stamp string, request *ortb.BidRequest) string {
	b, _ := json.Marshal(request)
	sum := sha256.Sum256(b)
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "pub-uuid-v1\n%s\n%s\n%s\n%x", id, feed, stamp, sum)
	return hex.EncodeToString(mac.Sum(nil))
}
func Validate(secret, id, feed, stamp, signature string, request *ortb.BidRequest, now time.Time) error {
	u, e := uuid.Parse(id)
	if e != nil || u == uuid.Nil || u.String() != id || u.Version() != 4 {
		return errors.New("canonical UUIDv4 required")
	}
	t, e := strconv.ParseInt(stamp, 10, 64)
	if e != nil || t < now.Unix()-30 || t > now.Unix()+5 {
		return errors.New("expired UUID header")
	}
	if len(secret) < 32 || request == nil || len(request.GetImp()) != 1 || request.GetImp()[0] == nil || request.GetImp()[0].GetId() == "" {
		return errors.New("one POP impression and secret required")
	}
	a, e := hex.DecodeString(signature)
	if e != nil {
		return errors.New("invalid UUID signature")
	}
	b, _ := hex.DecodeString(Sign(secret, id, feed, stamp, request))
	if !hmac.Equal(a, b) {
		return errors.New("invalid UUID signature")
	}
	return nil
}
func FeedAllowed(list, feed string) bool {
	for _, v := range strings.Split(list, ",") {
		if strings.TrimSpace(v) == feed && feed != "" {
			return true
		}
	}
	return false
}

// Claim touches only the UUID and its authentication marker, never publisher fields.
// The marker outlives loader deletion of the HASH so replay stays denied.
const ClaimLua = `if redis.call('EXISTS',KEYS[1])==0 then return 0 end
if redis.call('HGET',KEYS[2],'state')~='prepared' or redis.call('HGET',KEYS[2],'signature')~=ARGV[1] then return 0 end
redis.call('HSET',KEYS[2],'state','adapter');return 1`

func Claim(ctx context.Context, clients []*redis.Client, id, signature string) error {
	r, _, e := shard.SelectShard(clients, id)
	if e != nil {
		return e
	}
	n, e := r.Eval(ctx, ClaimLua, []string{id, Marker(id)}, signature).Int()
	if e != nil {
		return e
	}
	if n != 1 {
		return errors.New("UUID absent, already claimed or replayed")
	}
	return nil
}
