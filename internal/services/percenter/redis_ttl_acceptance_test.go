package percenter

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

type testRedisValue struct {
	value     string
	expiresAt time.Time
}

type testRedisServer struct {
	ln       net.Listener
	mu       sync.Mutex
	kv       map[string]testRedisValue
	sets     map[string]map[string]struct{}
	versions map[string]uint64
	execHook func()
	wg       sync.WaitGroup
}

type testRedisConn struct {
	multi   bool
	queue   [][]string
	watched map[string]uint64
}

type testRedisReply struct {
	kind  byte
	text  string
	n     int64
	items []testRedisReply
	nil   bool
}

func newTestRedisServer(t *testing.T) *testRedisServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &testRedisServer{ln: ln, kv: make(map[string]testRedisValue), sets: make(map[string]map[string]struct{}), versions: make(map[string]uint64)}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				defer conn.Close()
				s.serveConn(conn)
			}()
		}
	}()
	t.Cleanup(func() {
		_ = s.ln.Close()
		s.wg.Wait()
	})
	return s
}

func (s *testRedisServer) addr() string { return s.ln.Addr().String() }

func (s *testRedisServer) serveConn(conn net.Conn) {
	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	state := &testRedisConn{}
	for {
		cmd, err := readTestRedisCommand(r)
		if err != nil {
			return
		}
		if len(cmd) == 0 {
			return
		}
		name := strings.ToUpper(cmd[0])
		switch name {
		case "MULTI":
			state.multi = true
			state.queue = nil
			_ = writeTestRedisReply(w, testRedisReply{kind: '+', text: "OK"})
			_ = w.Flush()
			continue
		case "WATCH":
			s.mu.Lock()
			if state.watched == nil {
				state.watched = make(map[string]uint64)
			}
			for _, key := range cmd[1:] {
				state.watched[key] = s.versions[key]
			}
			s.mu.Unlock()
			_ = writeTestRedisReply(w, testRedisReply{kind: '+', text: "OK"})
			_ = w.Flush()
			continue
		case "UNWATCH":
			state.watched = nil
			_ = writeTestRedisReply(w, testRedisReply{kind: '+', text: "OK"})
			_ = w.Flush()
			continue
		case "DISCARD":
			state.multi = false
			state.queue = nil
			state.watched = nil
			_ = writeTestRedisReply(w, testRedisReply{kind: '+', text: "OK"})
			_ = w.Flush()
			continue
		case "EXEC":
			s.mu.Lock()
			hook := s.execHook
			s.mu.Unlock()
			if hook != nil {
				hook()
			}
			s.mu.Lock()
			watchConflict := false
			for key, version := range state.watched {
				if s.versions[key] != version {
					watchConflict = true
					break
				}
			}
			if watchConflict {
				s.mu.Unlock()
				state.multi = false
				state.queue = nil
				state.watched = nil
				_ = writeTestRedisReply(w, testRedisReply{kind: '*', nil: true})
				_ = w.Flush()
				continue
			}
			items := make([]testRedisReply, 0, len(state.queue))
			for _, queued := range state.queue {
				items = append(items, s.execLocked(queued))
			}
			s.mu.Unlock()
			state.multi = false
			state.queue = nil
			state.watched = nil
			_ = writeTestRedisReply(w, testRedisReply{kind: '*', items: items})
			_ = w.Flush()
			continue
		}
		if state.multi {
			state.queue = append(state.queue, append([]string(nil), cmd...))
			_ = writeTestRedisReply(w, testRedisReply{kind: '+', text: "QUEUED"})
			_ = w.Flush()
			continue
		}
		s.mu.Lock()
		reply := s.execLocked(cmd)
		s.mu.Unlock()
		_ = writeTestRedisReply(w, reply)
		_ = w.Flush()
	}
}

func (s *testRedisServer) purgeLocked(key string) {
	v, ok := s.kv[key]
	if ok && !v.expiresAt.IsZero() && !time.Now().Before(v.expiresAt) {
		delete(s.kv, key)
	}
}

func (s *testRedisServer) execLocked(cmd []string) testRedisReply {
	if len(cmd) == 0 {
		return testRedisReply{kind: '-', text: "ERR empty command"}
	}
	name := strings.ToUpper(cmd[0])
	switch name {
	case "PING":
		return testRedisReply{kind: '+', text: "PONG"}
	case "HELLO":
		return testRedisReply{kind: '*', items: []testRedisReply{
			{kind: '$', text: "server"}, {kind: '$', text: "redis"},
			{kind: '$', text: "version"}, {kind: '$', text: "7.2.0"},
			{kind: '$', text: "proto"}, {kind: ':', n: 2},
			{kind: '$', text: "id"}, {kind: ':', n: 1},
			{kind: '$', text: "mode"}, {kind: '$', text: "standalone"},
			{kind: '$', text: "role"}, {kind: '$', text: "master"},
			{kind: '$', text: "modules"}, {kind: '*', items: nil},
		}}
	case "CLIENT", "SELECT", "AUTH":
		return testRedisReply{kind: '+', text: "OK"}
	case "GET":
		if len(cmd) < 2 {
			return testRedisReply{kind: '-', text: "ERR wrong number of arguments"}
		}
		key := cmd[1]
		s.purgeLocked(key)
		v, ok := s.kv[key]
		if !ok {
			return testRedisReply{kind: '$', nil: true}
		}
		return testRedisReply{kind: '$', text: v.value}
	case "SET":
		if len(cmd) < 3 {
			return testRedisReply{kind: '-', text: "ERR wrong number of arguments"}
		}
		key, value := cmd[1], cmd[2]
		s.purgeLocked(key)
		nx := false
		var ttl time.Duration
		for i := 3; i < len(cmd); i++ {
			switch strings.ToUpper(cmd[i]) {
			case "NX":
				nx = true
			case "PX":
				if i+1 < len(cmd) {
					ms, _ := strconv.ParseInt(cmd[i+1], 10, 64)
					ttl = time.Duration(ms) * time.Millisecond
					i++
				}
			case "EX":
				if i+1 < len(cmd) {
					sec, _ := strconv.ParseInt(cmd[i+1], 10, 64)
					ttl = time.Duration(sec) * time.Second
					i++
				}
			}
		}
		if _, exists := s.kv[key]; nx && exists {
			return testRedisReply{kind: '$', nil: true}
		}
		entry := testRedisValue{value: value}
		if ttl > 0 {
			entry.expiresAt = time.Now().Add(ttl)
		}
		s.kv[key] = entry
		s.versions[key]++
		return testRedisReply{kind: '+', text: "OK"}
	case "DEL":
		var removed int64
		for _, key := range cmd[1:] {
			s.purgeLocked(key)
			mutated := false
			if _, ok := s.kv[key]; ok {
				delete(s.kv, key)
				removed++
				mutated = true
			}
			if _, ok := s.sets[key]; ok {
				delete(s.sets, key)
				removed++
				mutated = true
			}
			if mutated {
				s.versions[key]++
			}
		}
		return testRedisReply{kind: ':', n: removed}
	case "SADD":
		if len(cmd) < 3 {
			return testRedisReply{kind: '-', text: "ERR wrong number of arguments"}
		}
		set := s.sets[cmd[1]]
		if set == nil {
			set = make(map[string]struct{})
			s.sets[cmd[1]] = set
		}
		var added int64
		for _, member := range cmd[2:] {
			if _, ok := set[member]; !ok {
				set[member] = struct{}{}
				added++
			}
		}
		if added > 0 {
			s.versions[cmd[1]]++
		}
		return testRedisReply{kind: ':', n: added}
	case "SREM":
		if len(cmd) < 3 {
			return testRedisReply{kind: '-', text: "ERR wrong number of arguments"}
		}
		set := s.sets[cmd[1]]
		var removed int64
		for _, member := range cmd[2:] {
			if _, ok := set[member]; ok {
				delete(set, member)
				removed++
			}
		}
		if removed > 0 {
			s.versions[cmd[1]]++
		}
		return testRedisReply{kind: ':', n: removed}
	case "SMEMBERS":
		set := s.sets[cmd[1]]
		items := make([]testRedisReply, 0, len(set))
		for member := range set {
			items = append(items, testRedisReply{kind: '$', text: member})
		}
		return testRedisReply{kind: '*', items: items}
	case "EXISTS":
		var count int64
		for _, key := range cmd[1:] {
			s.purgeLocked(key)
			if _, ok := s.kv[key]; ok {
				count++
				continue
			}
			if _, ok := s.sets[key]; ok {
				count++
			}
		}
		return testRedisReply{kind: ':', n: count}
	case "PTTL":
		key := cmd[1]
		s.purgeLocked(key)
		v, ok := s.kv[key]
		if !ok {
			return testRedisReply{kind: ':', n: -2}
		}
		if v.expiresAt.IsZero() {
			return testRedisReply{kind: ':', n: -1}
		}
		ms := time.Until(v.expiresAt).Milliseconds()
		if ms < 0 {
			delete(s.kv, key)
			return testRedisReply{kind: ':', n: -2}
		}
		return testRedisReply{kind: ':', n: ms}
	default:
		return testRedisReply{kind: '-', text: fmt.Sprintf("ERR unsupported test command %s", name)}
	}
}

func readTestRedisCommand(r *bufio.Reader) ([]string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	if line == "" || line[0] != '*' {
		return nil, fmt.Errorf("unexpected RESP header %q", line)
	}
	n, err := strconv.Atoi(line[1:])
	if err != nil {
		return nil, err
	}
	cmd := make([]string, 0, n)
	for i := 0; i < n; i++ {
		header, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		header = strings.TrimSuffix(strings.TrimSuffix(header, "\n"), "\r")
		if header == "" || header[0] != '$' {
			return nil, fmt.Errorf("unexpected bulk header %q", header)
		}
		length, err := strconv.Atoi(header[1:])
		if err != nil {
			return nil, err
		}
		buf := make([]byte, length)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		if _, err := r.ReadByte(); err != nil { // \r
			return nil, err
		}
		if _, err := r.ReadByte(); err != nil { // \n
			return nil, err
		}
		cmd = append(cmd, string(buf))
	}
	return cmd, nil
}

func writeTestRedisReply(w *bufio.Writer, reply testRedisReply) error {
	switch reply.kind {
	case '+':
		_, err := fmt.Fprintf(w, "+%s\r\n", reply.text)
		return err
	case '-':
		_, err := fmt.Fprintf(w, "-%s\r\n", reply.text)
		return err
	case ':':
		_, err := fmt.Fprintf(w, ":%d\r\n", reply.n)
		return err
	case '$':
		if reply.nil {
			_, err := w.WriteString("$-1\r\n")
			return err
		}
		if _, err := fmt.Fprintf(w, "$%d\r\n", len(reply.text)); err != nil {
			return err
		}
		if _, err := w.WriteString(reply.text); err != nil {
			return err
		}
		_, err := w.WriteString("\r\n")
		return err
	case '*':
		if reply.nil {
			_, err := w.WriteString("*-1\r\n")
			return err
		}
		if _, err := fmt.Fprintf(w, "*%d\r\n", len(reply.items)); err != nil {
			return err
		}
		for _, item := range reply.items {
			if err := writeTestRedisReply(w, item); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported reply kind %q", reply.kind)
	}
}

func newTestRedisClient(t *testing.T, server *testRedisServer) *redis.Client {
	t.Helper()
	client := redis.NewClient(&redis.Options{
		Addr:            server.addr(),
		Protocol:        2,
		DisableIdentity: true,
		PoolSize:        1,
		MaxRetries:      -1,
		DialTimeout:     time.Second,
		ReadTimeout:     time.Second,
		WriteTimeout:    time.Second,
	})
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func shortTTLTestSimplePolicy(stateTTL, pendingTTL time.Duration) SimplePolicy {
	return SimplePolicy{
		WinRateRetention: .50, MinImpressions: 5,
		OptimizeInterval: 5 * time.Minute, RebenchmarkInterval: 6 * time.Hour,
		SearchStepsPP: []float64{5, 2, 1}, MaxMargin: .90,
		StateTTL: stateTTL, PendingHistoryTTL: pendingTTL,
	}
}

func TestSimplePendingHistorySurvivesStateExpiryAndRecoversOnce(t *testing.T) {
	server := newTestRedisServer(t)
	client := newTestRedisClient(t, server)
	ctx := context.Background()
	policy := shortTTLTestSimplePolicy(60*time.Millisecond, 2*time.Second)
	store := NewSimpleStateStore(client, policy)

	const segmentHash = "simple-ttl-recovery"
	if _, err := store.GetOrInitPricing(ctx, segmentHash, "campaign-1", 1, .2, false, time.Now().UTC(), "ALL"); err != nil {
		t.Fatal(err)
	}
	state, err := store.Get(ctx, segmentHash)
	if err != nil {
		t.Fatal(err)
	}
	event := HistoryEvent{
		EventID:   stableObservabilityID("history", segmentHash, strconv.FormatUint(state.PointVersion, 10)),
		Timestamp: time.Now().UTC(), SegmentHash: segmentHash, CampaignID: "campaign-1",
		TypeModel: TypeModelSimple, PointVersion: state.PointVersion,
	}
	state.PendingHistory = &event
	saved, err := store.SaveCAS(ctx, state, state.PointVersion)
	if err != nil || !saved {
		t.Fatalf("SaveCAS saved=%v err=%v", saved, err)
	}

	if exists, err := client.Exists(ctx, pendingHistoryRecordPrefix+event.EventID).Result(); err != nil || exists != 1 {
		t.Fatalf("pending mirror not committed: exists=%d err=%v", exists, err)
	}

	// Move beyond optimizer StateTTL while remaining comfortably inside the
	// independent PendingHistory retention.
	time.Sleep(140 * time.Millisecond)
	if _, err := store.Get(ctx, segmentHash); err != redis.Nil {
		t.Fatalf("optimizer state must expire after short StateTTL, got %v", err)
	}
	if exists, err := client.Exists(ctx, pendingHistoryRecordPrefix+event.EventID).Result(); err != nil || exists != 1 {
		t.Fatalf("pending history mirror must outlive optimizer state: exists=%d err=%v", exists, err)
	}

	outbox, err := OpenObservabilityOutbox(t.TempDir() + "/pending-recovery.db")
	if err != nil {
		t.Fatal(err)
	}
	defer outbox.Close()

	recovered, err := RecoverPendingHistory(ctx, client, outbox, 100)
	if err != nil {
		t.Fatal(err)
	}
	if recovered != 1 {
		t.Fatalf("recovered=%d want 1", recovered)
	}
	records, err := outbox.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].ID != event.EventID {
		t.Fatalf("unexpected recovered outbox records: %#v", records)
	}

	recovered, err = RecoverPendingHistory(ctx, client, outbox, 100)
	if err != nil {
		t.Fatal(err)
	}
	if recovered != 0 {
		t.Fatalf("repeated recovery must be idempotent, recovered=%d", recovered)
	}
	records, err = outbox.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("repeated recovery created duplicate logical event: %d", len(records))
	}
}

func TestSimpleStateTTLRefreshUsesCurrentSharedPolicyWithoutReset(t *testing.T) {
	server := newTestRedisServer(t)
	client := newTestRedisClient(t, server)
	ctx := context.Background()

	oldPolicy := shortTTLTestSimplePolicy(120*time.Millisecond, 2*time.Second)
	oldStore := NewSimpleStateStore(client, oldPolicy)
	const segmentHash = "simple-ttl-refresh"
	if _, err := oldStore.GetOrInitPricing(ctx, segmentHash, "campaign-1", 1, .2, false, time.Now().UTC(), "ALL"); err != nil {
		t.Fatal(err)
	}
	state, err := oldStore.Get(ctx, segmentHash)
	if err != nil {
		t.Fatal(err)
	}
	originalVersion := state.PointVersion

	newPolicy := oldPolicy
	newPolicy.StateTTL = 900 * time.Millisecond
	if !state.Compatible(segmentHash, "campaign-1", 1, .2, false, newPolicy) {
		t.Fatal("TTL-only policy change must not invalidate optimizer point")
	}
	newStore := NewSimpleStateStore(client, newPolicy)
	saved, err := newStore.SaveCAS(ctx, state, originalVersion)
	if err != nil || !saved {
		t.Fatalf("TTL refresh SaveCAS saved=%v err=%v", saved, err)
	}

	pttl, err := client.PTTL(ctx, SimpleStateKey(segmentHash)).Result()
	if err != nil {
		t.Fatal(err)
	}
	if pttl < 650*time.Millisecond {
		t.Fatalf("state TTL was not refreshed from current shared policy: %s", pttl)
	}

	// Past the old TTL the state must still be present, with unchanged point
	// identity. This proves SaveCAS used the new policy TTL instead of falling
	// back to the original/default TTL.
	time.Sleep(220 * time.Millisecond)
	refreshed, err := newStore.Get(ctx, segmentHash)
	if err != nil {
		t.Fatalf("state expired according to old TTL after refresh: %v", err)
	}
	if refreshed.PointVersion != originalVersion {
		t.Fatalf("TTL-only refresh changed point_version: got %d want %d", refreshed.PointVersion, originalVersion)
	}
	if !refreshed.Compatible(segmentHash, "campaign-1", 1, .2, false, newPolicy) {
		t.Fatal("state became incompatible after TTL-only refresh")
	}

	// Ensure stored JSON is still a normal optimizer state rather than a
	// TTL-specific replacement/reset payload.
	raw, err := client.Get(ctx, SimpleStateKey(segmentHash)).Bytes()
	if err != nil {
		t.Fatal(err)
	}
	var decoded SimpleState
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.PointVersion != originalVersion || decoded.SegmentHash != segmentHash {
		t.Fatalf("unexpected refreshed state: %#v", decoded)
	}
}

func TestIndexedRecoveryCleanupRemovesOnlyMissingRecords(t *testing.T) {
	server := newTestRedisServer(t)
	client := newTestRedisClient(t, server)
	ctx := context.Background()

	for _, tc := range []struct {
		name      string
		indexKey  string
		member    string
		recordKey string
		present   bool
	}{
		{name: "pending missing", indexKey: pendingHistoryReadyKey, member: "pending-missing", recordKey: pendingHistoryRecordPrefix + "pending-missing"},
		{name: "pending present", indexKey: pendingHistoryReadyKey, member: "pending-present", recordKey: pendingHistoryRecordPrefix + "pending-present", present: true},
		{name: "observability missing", indexKey: ObservabilityReadyKey, member: "observability-missing", recordKey: observabilityRecordPrefix + "observability-missing"},
		{name: "observability present", indexKey: ObservabilityReadyKey, member: "observability-present", recordKey: observabilityRecordPrefix + "observability-present", present: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := client.SAdd(ctx, tc.indexKey, tc.member).Err(); err != nil {
				t.Fatal(err)
			}
			if tc.present {
				if err := client.Set(ctx, tc.recordKey, "payload", time.Minute).Err(); err != nil {
					t.Fatal(err)
				}
			}
			if err := removeIndexedMemberIfRecordMissing(ctx, client, tc.indexKey, tc.member, tc.recordKey); err != nil {
				t.Fatal(err)
			}
			members, err := client.SMembers(ctx, tc.indexKey).Result()
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, member := range members {
				if member == tc.member {
					found = true
					break
				}
			}
			if found != tc.present {
				t.Fatalf("index membership=%v want %v; members=%v", found, tc.present, members)
			}
		})
	}
}

func TestIndexedRecoveryCleanupDoesNotDeleteConcurrentRecreation(t *testing.T) {
	server := newTestRedisServer(t)
	client := newTestRedisClient(t, server)
	recreator := newTestRedisClient(t, server)
	ctx := context.Background()
	const member = "recreated-event"
	indexKey := pendingHistoryReadyKey
	recordKey := pendingHistoryRecordPrefix + member

	if err := client.SAdd(ctx, indexKey, member).Err(); err != nil {
		t.Fatal(err)
	}
	execReached := make(chan struct{})
	releaseExec := make(chan struct{})
	var hookOnce sync.Once
	server.mu.Lock()
	server.execHook = func() {
		hookOnce.Do(func() { close(execReached) })
		<-releaseExec
	}
	server.mu.Unlock()

	errCh := make(chan error, 1)
	go func() {
		errCh <- removeIndexedMemberIfRecordMissing(ctx, client, indexKey, member, recordKey)
	}()
	select {
	case <-execReached:
	case <-time.After(time.Second):
		t.Fatal("cleanup never reached watched EXEC")
	}
	if err := recreator.Set(ctx, recordKey, "new-record", time.Minute).Err(); err != nil {
		t.Fatal(err)
	}
	close(releaseExec)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}

	members, err := client.SMembers(ctx, indexKey).Result()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, got := range members {
		if got == member {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("concurrently recreated record lost its index membership: %v", members)
	}
}

func TestSimpleAndComplexStateIndexCleanupPreservesConcurrentRecreation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		indexKey string
		stateKey func(string) string
		cleanup  func(context.Context, *redis.Client, string) error
	}{
		{
			name: "simple", indexKey: SimpleStateIndexKey, stateKey: SimpleStateKey,
			cleanup: func(ctx context.Context, client *redis.Client, hash string) error {
				return (&SimpleStateStore{redis: client}).removeStaleIndexMember(ctx, hash)
			},
		},
		{
			name: "complex", indexKey: ComplexStateIndexKey, stateKey: ComplexStateKey,
			cleanup: func(ctx context.Context, client *redis.Client, hash string) error {
				return (&ComplexStateStore{redis: client}).removeStaleIndexMember(ctx, hash)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := newTestRedisServer(t)
			client := newTestRedisClient(t, server)
			recreator := newTestRedisClient(t, server)
			ctx := context.Background()
			hash := tc.name + "-recreated"
			key := tc.stateKey(hash)

			if err := client.SAdd(ctx, tc.indexKey, hash).Err(); err != nil {
				t.Fatal(err)
			}
			execReached := make(chan struct{})
			releaseExec := make(chan struct{})
			var hookOnce sync.Once
			server.mu.Lock()
			server.execHook = func() {
				hookOnce.Do(func() { close(execReached) })
				<-releaseExec
			}
			server.mu.Unlock()

			errCh := make(chan error, 1)
			go func() { errCh <- tc.cleanup(ctx, client, hash) }()
			select {
			case <-execReached:
			case <-time.After(time.Second):
				t.Fatal("state-index cleanup never reached watched EXEC")
			}
			if err := recreator.Set(ctx, key, `{"recreated":true}`, time.Minute).Err(); err != nil {
				t.Fatal(err)
			}
			close(releaseExec)
			if err := <-errCh; err != nil {
				t.Fatal(err)
			}

			members, err := client.SMembers(ctx, tc.indexKey).Result()
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, member := range members {
				if member == hash {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("concurrently recreated %s state lost index membership: %v", tc.name, members)
			}
		})
	}
}
