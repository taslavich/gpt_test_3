// Package callback stores private callback context in the existing winner hashes.
package callback

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const Field = "callback_context_v1"

type Record struct {
	Format      string `json:"format"`
	Destination string `json:"destination"`
	NURL        string `json:"nurl"`
	BURL        string `json:"burl"`
	SSP         string `json:"ssp"`
	WinnerType  string `json:"winner_type"`
	PublisherID string `json:"publisher_id,omitempty"`
}

func ValidID(id string) bool {
	u, e := uuid.Parse(id)
	return e == nil && u != uuid.Nil && u.String() == id
}
func (r Record) Validate() error {
	switch r.Format {
	case "POP", "BAN", "NAT", "IPP", "VID":
	default:
		return errors.New("invalid callback format")
	}
	if r.WinnerType != "adv" && r.WinnerType != "dsp" {
		return errors.New("invalid winner type")
	}
	for _, raw := range []string{r.Destination, r.NURL, r.BURL} {
		if raw != "" {
			if e := ValidateURL(raw); e != nil {
				return e
			}
		}
	}
	return nil
}
func ValidateURL(raw string) error {
	u, e := url.Parse(raw)
	if e != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil || strings.ContainsAny(raw, "\r\n") {
		return errors.New("invalid callback destination")
	}
	if strings.EqualFold(u.Hostname(), "localhost") || strings.HasSuffix(strings.ToLower(u.Hostname()), ".localhost") {
		return errors.New("nonpublic callback destination")
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && !PublicIP(ip) {
		return errors.New("nonpublic callback IP")
	}
	return nil
}
func PublicIP(ip net.IP) bool {
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	// Carrier-grade NAT and IPv4-mapped metadata endpoints are not public DSPs.
	for _, cidr := range []string{"100.64.0.0/10", "192.0.0.0/24", "198.18.0.0/15", "0.0.0.0/8"} {
		_, n, _ := net.ParseCIDR(cidr)
		if n.Contains(ip) {
			return false
		}
	}
	return true
}
func Decode(fields map[string]string) (Record, error) {
	var r Record
	if fields[Field] == "" {
		return r, redis.Nil
	}
	if e := json.Unmarshal([]byte(fields[Field]), &r); e != nil {
		return r, e
	}
	return r, r.Validate()
}
func Save(ctx context.Context, c *redis.Client, id string, r Record, ttl time.Duration, requireWinner bool) error {
	if c == nil || !ValidID(id) {
		return errors.New("invalid callback store/id")
	}
	if e := r.Validate(); e != nil {
		return e
	}
	if ttl <= 0 {
		return errors.New("callback retention must be positive")
	}
	b, e := json.Marshal(r)
	if e != nil {
		return e
	}
	// One atomic command: never publish context without retention, never resurrect
	// a missing ADV winner as an incomplete billing record. ADV keeps its existing
	// authorization TTL; DSP uses the existing REDIS_UUID_KEY_TTL.
	n, e := c.Eval(ctx, `if ARGV[3]=='1' then
  if redis.call('EXISTS',KEYS[1])==0 then return 0 end
  if redis.call('PTTL',KEYS[1])<=0 then return 0 end
  local fields=redis.call('HMGET',KEYS[1],'format','price','user_id','campaign_id')
  if fields[1]~=ARGV[5] or not tonumber(fields[2]) or tonumber(fields[2])<=0 or not fields[3] or fields[3]=='' or not fields[4] or fields[4]=='' then return 0 end
 end
 local previous=redis.call('HGET',KEYS[1],ARGV[4]);if previous and previous~=ARGV[1] then return -1 end
 redis.call('HSET',KEYS[1],'uuid',KEYS[1],ARGV[4],ARGV[1])
 if ARGV[3]~='1' and not previous then redis.call('PEXPIRE',KEYS[1],ARGV[2]) end
 return 1`, []string{id}, string(b), ttl.Milliseconds(), boolArg(requireWinner), Field, r.Format).Int()
	if e != nil {
		return e
	}
	if n == -1 {
		return errors.New("callback UUID context is immutable")
	}
	if n != 1 {
		return redis.Nil
	}
	return nil
}
func boolArg(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

type collectorKey struct{}

func Collecting(ctx context.Context) (context.Context, map[string]Record) {
	m := map[string]Record{}
	return context.WithValue(ctx, collectorKey{}, m), m
}
func Collect(ctx context.Context, id string, r Record) {
	if m, ok := ctx.Value(collectorKey{}).(map[string]Record); ok {
		m[id] = r
	}
}

// Resolve and validate every redirect hop and every DNS address at dial time.
// No proxy environment: a configured HTTP proxy could bypass the IP guard.
func HTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 2 * time.Second, KeepAlive: 30 * time.Second}
	tr := &http.Transport{MaxIdleConns: 256, MaxIdleConnsPerHost: 16, IdleConnTimeout: 90 * time.Second}
	tr.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, e := net.SplitHostPort(address)
		if e != nil {
			return nil, e
		}
		ips, e := net.DefaultResolver.LookupIPAddr(ctx, host)
		if e != nil {
			return nil, e
		}
		if len(ips) == 0 {
			return nil, errors.New("empty DSP DNS")
		}
		for _, ip := range ips {
			if !PublicIP(ip.IP) {
				return nil, fmt.Errorf("blocked DSP address for %s", host)
			}
		}
		var last error
		for _, ip := range ips {
			conn, e := dialer.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
			if e == nil {
				return conn, nil
			}
			last = e
		}
		return nil, last
	}
	return &http.Client{Timeout: 3 * time.Second, Transport: tr, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many DSP redirects")
		}
		return ValidateURL(req.URL.String())
	}}
}
