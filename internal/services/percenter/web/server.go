package web

import (
	"context"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	goredis "github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/percenter"
)

type TickStatus struct {
	StartedAt  time.Time `json:"started_at,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	LastError  string    `json:"last_error,omitempty"`
}

type Server struct {
	redis      *goredis.Client
	store      *percenter.StateStore
	clickhouse clickhouse.Conn
	cfg        *config.PercenterConfig
	policy     percenter.Policy
	startedAt  time.Time

	mu       sync.RWMutex
	lastTick TickStatus

	stateHistoryFailure   func(context.Context, string)
	stateHistoryRecovered func(context.Context, string)
}

func NewServer(
	redisClient *goredis.Client,
	store *percenter.StateStore,
	clickhouseConn clickhouse.Conn,
	cfg *config.PercenterConfig,
	policy percenter.Policy,
) *Server {
	return &Server{
		redis:      redisClient,
		store:      store,
		clickhouse: clickhouseConn,
		cfg:        cfg,
		policy:     policy.Normalize(),
		startedAt:  time.Now().UTC(),
	}
}

func (s *Server) SetStateHistoryHealthReporter(onFailure func(context.Context, string), onRecovered func(context.Context, string)) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.stateHistoryFailure = onFailure
	s.stateHistoryRecovered = onRecovered
	s.mu.Unlock()
}

func (s *Server) reportStateHistoryFailure(ctx context.Context, message string) {
	if s == nil {
		return
	}
	s.mu.RLock()
	fn := s.stateHistoryFailure
	s.mu.RUnlock()
	if fn != nil {
		fn(ctx, message)
	}
}

func (s *Server) reportStateHistoryRecovered(ctx context.Context, message string) {
	if s == nil {
		return
	}
	s.mu.RLock()
	fn := s.stateHistoryRecovered
	s.mu.RUnlock()
	if fn != nil {
		fn(ctx, message)
	}
}

func (s *Server) RecordTickStart(at time.Time) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.lastTick.StartedAt = at
	s.lastTick.FinishedAt = time.Time{}
	s.lastTick.LastError = ""
	s.mu.Unlock()
}

func (s *Server) RecordTickFinish(at time.Time, err error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.lastTick.FinishedAt = at
	if err != nil {
		s.lastTick.LastError = err.Error()
	} else {
		s.lastTick.LastError = ""
	}
	s.mu.Unlock()
}

func (s *Server) tickStatus() TickStatus {
	if s == nil {
		return TickStatus{}
	}
	s.mu.RLock()
	status := s.lastTick
	s.mu.RUnlock()
	return status
}
