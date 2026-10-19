package pub

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/redis/go-redis/v9"
	c "gitlab.com/twinbid-exchange/RTB-exchange/internal/pubcontract"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/pubuuid"
	shard "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/redis"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/ua"
	"net/http"
	"strings"
	"time"
)

// The existing ORTB store is a list of standalone shards, NOT the cabinet's
// Redis Cluster. Use its DB and ordered addresses verbatim (FNV-1a UUID sharding).
const prepareLua = `if redis.call('EXISTS',KEYS[1])==1 or redis.call('EXISTS',KEYS[2])==1 then return 0 end
local fields=cjson.decode(ARGV[1]);for k,v in pairs(fields) do redis.call('HSET',KEYS[1],k,v) end
redis.call('PEXPIRE',KEYS[1],ARGV[3]);redis.call('HSET',KEYS[2],'state','prepared','signature',ARGV[2]);redis.call('PEXPIRE',KEYS[2],ARGV[3]);return 1`

// ClaimLua and errorLua serialize at the same shard. Only a PREPARED request can
// become an error; once the adapter has claimed it, pub can never commit it.
const errorLua = `local state=redis.call('HGET',KEYS[2],'state')
if state=='error' then return 1 end
if state and state~='prepared' then return 0 end
if not state and redis.call('EXISTS',KEYS[1])==1 then return 0 end
local fields=cjson.decode(ARGV[1]);for k,v in pairs(fields) do redis.call('HSET',KEYS[1],k,v) end
redis.call('PEXPIRE',KEYS[1],ARGV[2]);redis.call('HSET',KEYS[2],'state','error');redis.call('PEXPIRE',KEYS[2],ARGV[2]);redis.call('SADD',KEYS[3],KEYS[1]);return 1`

func writeError(ctx context.Context, clients []*redis.Client, set, id string, fields map[string]string) (bool, error) {
	r, _, e := shard.SelectShard(clients, id)
	if e != nil {
		return false, e
	}
	b, e := json.Marshal(fields)
	if e != nil {
		return false, e
	}
	n, e := r.Eval(ctx, errorLua, []string{id, pubuuid.Marker(id), set}, b, pubuuid.TTL.Milliseconds()).Int()
	return n == 1, e
}
func (s *Server) publisherFields(r *http.Request, in Request, code c.Code, host, site, zone, reason string, status int) map[string]string {
	typic, feed := "", ""
	switch code.TrafficType {
	case "adult":
		typic, feed = "ADULT", s.Config.AdultFeed
	case "mainstream":
		typic, feed = "MAINSTREAM", s.Config.MainstreamFeed
	}
	uaFields := ua.ParseUA(r.UserAgent())
	fields := map[string]string{"EVENT_TIME": time.Now().UTC().Format("2006-01-02 15:04:05.000"), "FORMAT": "POP", "TYPIC": typic, "SPP_DOMAIN": feed, "CODE": fmt.Sprint(status), "pub_user_id": code.UserID, "ad_code_id": code.ID, "zone_id": zone, "request_id": in.ID, "SITE_ID": site, "SITE_DOMAIN": host, "site_page": in.PageURL, "request_error": "0", "error_reason": reason, "BROWSER": uaFields.Browser, "BROWSER_VERSION": uaFields.BrowserVersion, "OS": uaFields.OS, "OS_VERSION": uaFields.OSVersion, "DEVICE": uaFields.Device}
	ip := s.clientIP(r)
	if len(ip) > 0 {
		if strings.Contains(ip, ":") {
			fields["IPV6"] = ip
		} else {
			fields["IP"] = ip
		}
	}
	if reason != "" {
		fields["request_error"] = "1"
	}
	return fields
}
func (s *Server) prepareORTB(ctx context.Context, id, signature string, fields map[string]string) error {
	r, _, e := shard.SelectShard(s.ORTB, id)
	if e != nil {
		return e
	}
	b, e := json.Marshal(fields)
	if e != nil {
		return e
	}
	n, e := r.Eval(ctx, prepareLua, []string{id, pubuuid.Marker(id)}, b, signature, pubuuid.TTL.Milliseconds()).Int()
	if e != nil {
		return e
	}
	if n != 1 {
		return fmt.Errorf("ORTB UUID collision/replay")
	}
	return nil
}

func (s *Server) CloseORTB() error { return shard.CloseClients(s.ORTB) }
