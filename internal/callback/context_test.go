package callback

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func TestCallbackURLsRejectNonpublicAndNonHTTPDestinations(t *testing.T) {
	for _, raw := range []string{"javascript:alert(1)", "https://u:p@public.test/", "http://127.0.0.1/x", "http://[::1]/", "http://169.254.169.254/", "http://10.0.0.1/", "http://100.64.1.1/", "http://localhost/", "https://public.test/\r\nX: y"} {
		if ValidateURL(raw) == nil {
			t.Fatal("accepted", raw)
		}
	}
	for _, raw := range []string{"https://dsp.test/win?token=kept%2F", "http://public.test:8080/path"} {
		if e := ValidateURL(raw); e != nil {
			t.Fatal(e)
		}
	}
	client := HTTPClient()
	if _, e := client.Get("http://127.0.0.1:9/private"); e == nil {
		t.Fatal("SSRF allowed")
	}
}
func TestUUIDAndCollectorAreRequestScoped(t *testing.T) {
	id := uuid.NewString()
	for _, invalid := range []string{"", uuid.Nil.String(), "winner", strings.ToUpper(id), id + "&f=4"} {
		if ValidID(invalid) {
			t.Fatal(invalid)
		}
	}
	ctx, m := Collecting(context.Background())
	Collect(ctx, id, Record{Format: "POP"})
	_, other := Collecting(context.Background())
	if len(m) != 1 || len(other) != 0 {
		t.Fatal("cross-request context")
	}
}
func TestCallbackRedisAOFRestartAndRetention(t *testing.T) {
	binary := os.Getenv("CALLBACK_TEST_REDIS_SERVER")
	if binary == "" {
		t.Skip("CALLBACK_TEST_REDIS_SERVER unset")
	}
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	dir := t.TempDir()
	var cmd *exec.Cmd
	start := func() {
		t.Helper()
		cmd = exec.Command(binary, "--bind", "127.0.0.1", "--port", strconv.Itoa(port), "--dir", dir, "--appendonly", "yes", "--appendfsync", "always", "--logfile", filepath.Join(dir, "redis.log"))
		if e = cmd.Start(); e != nil {
			t.Fatal(e)
		}
	}
	stop := func() {
		if cmd != nil {
			cmd.Process.Signal(syscall.SIGTERM)
			cmd.Wait()
			cmd = nil
		}
	}
	defer stop()
	start()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	connect := func() *redis.Client {
		t.Helper()
		c := redis.NewClient(&redis.Options{Addr: net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), MaxRetries: -1, DialTimeout: 100 * time.Millisecond})
		for i := 0; i < 100; i++ {
			if c.Ping(ctx).Err() == nil {
				return c
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("Redis failed to start")
		return nil
	}
	c := connect()
	id := uuid.NewString()
	record := Record{Format: "POP", Destination: "https://advertiser.test/land", WinnerType: "dsp", SSP: "ssp"}
	if e = Save(ctx, c, id, record, time.Minute, false); e != nil {
		t.Fatal(e)
	}
	if c.PTTL(ctx, id).Val() < 50*time.Second {
		t.Fatal("late callbacks lost")
	}
	if e = Save(ctx, c, uuid.NewString(), record, time.Hour, true); !errors.Is(e, redis.Nil) {
		t.Fatal("resurrected missing ADV winner", e)
	}

	// Exercise the production DB split on the same AOF-backed instance as well.
	secondary := redis.NewClient(&redis.Options{Addr: c.Options().Addr, DB: 1})
	advStore := redis.NewClient(&redis.Options{Addr: c.Options().Addr, DB: 6})
	advID := uuid.NewString()
	advRecord := record
	advRecord.WinnerType = "adv"
	advStore.HSet(ctx, advID, "format", "POP", "price", "0.25", "user_id", "user", "campaign_id", "campaign")
	advStore.Expire(ctx, advID, time.Minute)
	if e = Save(ctx, secondary, id, record, time.Minute, false); e != nil {
		t.Fatal(e)
	}
	if e = Save(ctx, advStore, advID, advRecord, time.Minute, true); e != nil {
		t.Fatal(e)
	}
	addr := c.Options().Addr
	secondary.Close()
	advStore.Close()
	c.Close()
	// Kill without Redis graceful shutdown: appendfsync=always is the durability
	// boundary, not an orderly process exit or a local in-process cache.
	if e = cmd.Process.Kill(); e != nil {
		t.Fatal(e)
	}
	cmd.Wait()
	cmd = nil
	start()
	c = connect()
	defer c.Close()
	got, e := Decode(c.HGetAll(ctx, id).Val())
	if e != nil || got != record {
		t.Fatal("AOF restart lost context", got, e)
	}

	for db, want := range map[int]Record{1: record, 6: advRecord} {
		restored := redis.NewClient(&redis.Options{Addr: addr, DB: db})
		key := id
		if db == 6 {
			key = advID
		}
		fields := restored.HGetAll(ctx, key).Val()
		got, err := Decode(fields)
		restored.Close()
		if err != nil || got != want {
			t.Fatal("AOF DB split lost context", db, got, err)
		}
		if db == 6 && fields["price"] != "0.25" {
			t.Fatal("ADV winner fields lost")
		}
	}
	c.PExpire(ctx, id, time.Millisecond)
	time.Sleep(5 * time.Millisecond)
	if _, e = Decode(c.HGetAll(ctx, id).Val()); !errors.Is(e, redis.Nil) {
		t.Fatal("expired context accepted", e)
	}
}

func TestCallbackSaveRejectsBrokenADVAndContextMutation(t *testing.T) {
	addr := os.Getenv("PUB_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("PUB_TEST_REDIS_ADDR unset")
	}
	ctx := context.Background()
	c := redis.NewClient(&redis.Options{Addr: addr, DB: 14})
	defer c.Close()
	id := uuid.NewString()
	defer c.Del(ctx, id)
	r := Record{Format: "POP", WinnerType: "adv", Destination: "https://advertiser.test/original"}
	c.HSet(ctx, id, "format", "POP", "price", "1", "user_id", "user", "campaign_id", "campaign")
	if e := Save(ctx, c, id, r, time.Hour, true); !errors.Is(e, redis.Nil) {
		t.Fatal("winner without retention exposed", e)
	}
	c.Expire(ctx, id, time.Hour)
	if e := Save(ctx, c, id, r, time.Hour, true); e != nil {
		t.Fatal(e)
	}
	altered := r
	altered.Destination = "https://other.test/changed"
	if e := Save(ctx, c, id, altered, time.Hour, true); e == nil {
		t.Fatal("winner destination was mutable")
	}
	got, e := Decode(c.HGetAll(ctx, id).Val())
	if e != nil || got != r {
		t.Fatal(got, e)
	}
	c.PExpire(ctx, id, time.Second)
	if e := Save(ctx, c, id, r, 24*time.Hour, true); e != nil {
		t.Fatal(e)
	}
	if c.PTTL(ctx, id).Val() > time.Second {
		t.Fatal("ADV authorization extended")
	}
}
