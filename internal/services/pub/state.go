package pub

import (
	"context"
	"errors"
	c "gitlab.com/twinbid-exchange/RTB-exchange/internal/pubcontract"
	"time"
)

// All keys used by each Lua script have the same zone hash tag. No cross-slot Lua.
// The counters and pending reservations are atomic per visitor. A Redis
// restart with AOF preserves them; a small amount of unflushed data can be
// lost, which does not justify stopping the entire publisher network.
const reserveLua = `local t=redis.call('TIME'); local now=t[1]*1000+math.floor(t[2]/1000)
 local cached=redis.call('GET',KEYS[3]);if cached then return cached end
 local pending=redis.call('HGET',KEYS[1],'pending');local untilAt=tonumber(redis.call('HGET',KEYS[1],'until') or '0')
 if pending and untilAt>now then return 'busy' end
 local last=tonumber(redis.call('HGET',KEYS[1],'last') or '0');if now-last<tonumber(ARGV[2]) then return 'frequency' end
 local count=tonumber(redis.call('GET',KEYS[2]) or '0');if tonumber(ARGV[3])>0 and count>=tonumber(ARGV[3]) then return 'session_cap' end
 redis.call('HSET',KEYS[1],'pending',ARGV[1],'until',now+45000);redis.call('PEXPIRE',KEYS[1],ARGV[4]);redis.call('SET',KEYS[3],'in_flight','PX',300000);return 'reserved'`
const finishLua = `if redis.call('HGET',KEYS[1],'pending')==ARGV[1] then redis.call('HDEL',KEYS[1],'pending','until') end;redis.call('SET',KEYS[2],ARGV[2],'PX',300000);return 1`
const confirmLua = `local t=redis.call('TIME');local now=t[1]*1000+math.floor(t[2]/1000)
 if redis.call('GET',KEYS[3]) then return 2 end
 if redis.call('HGET',KEYS[1],'pending')~=ARGV[1] or tonumber(redis.call('HGET',KEYS[1],'until') or '0')<now then return 0 end
 redis.call('HSET',KEYS[1],'last',now);redis.call('HDEL',KEYS[1],'pending','until');redis.call('PEXPIRE',KEYS[1],ARGV[2]);redis.call('INCR',KEYS[2]);redis.call('EXPIRE',KEYS[2],86400);redis.call('SET',KEYS[3],'pending','EX',86400);return 1`

func keys(zone, visitor, session, request string) (string, string, string) {
	base := c.Prefix + "{" + c.Hash("frequency-state", zone, visitor) + "}:"
	return base + "visitor:" + visitor, base + "session:" + visitor + ":" + session, base + "request:" + visitor + ":" + request
}
func (s *Server) ready(ctx context.Context) (string, error) {
	if s.safetyUntil.Load() < time.Now().Unix() {
		return "", errors.New("Redis eviction policy unchecked")
	}
	// Published configuration and frequency state persist in Redis. Never
	// require a per-15-second heartbeat from the cabinet backend.
	v, e := s.Redis.Get(ctx, c.EpochKey).Result()
	if e != nil || v == "" {
		return "", errors.New("publisher configuration unavailable")
	}
	return v, nil
}
func stateTTL(code c.Code) int64 { return int64(code.FrequencyMinutes*60000) + 86400000 }
func (s *Server) reserve(ctx context.Context, code c.Code, zone string, r Request) (string, error) {
	a, b, k := keys(zone, r.VisitorID, r.SessionID, r.ID)
	cap := 0
	if code.MaxPerSession != nil {
		cap = *code.MaxPerSession
	}
	v, e := s.Redis.Eval(ctx, reserveLua, []string{a, b, k}, r.ID, int64(code.FrequencyMinutes*60000), cap, stateTTL(code)).Text()
	return v, e
}
func (s *Server) finish(ctx context.Context, d Delivery, result string) error {
	a, _, k := keys(d.ZoneID, d.VisitorID, d.SessionID, d.RequestID)
	return s.Redis.Eval(ctx, finishLua, []string{a, k}, d.RequestID, result).Err()
}
func (s *Server) confirmState(ctx context.Context, d Delivery) (int, error) {
	a, b, _ := keys(d.ZoneID, d.VisitorID, d.SessionID, d.RequestID)
	k := c.Prefix + "{" + c.Hash("frequency-state", d.ZoneID, d.VisitorID) + "}:open:" + d.RequestID
	return s.Redis.Eval(ctx, confirmLua, []string{a, b, k}, d.RequestID, d.StateTTL).Int()
}
