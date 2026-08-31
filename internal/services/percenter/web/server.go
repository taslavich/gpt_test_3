package web

import (
	"crypto/subtle"
	"net/http"
	"strings"
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
	secret     string
	startedAt  time.Time

	mu       sync.RWMutex
	lastTick TickStatus
}

func NewServer(
	redisClient *goredis.Client,
	store *percenter.StateStore,
	clickhouseConn clickhouse.Conn,
	cfg *config.PercenterConfig,
	policy percenter.Policy,
) *Server {
	secret := ""
	if cfg != nil {
		secret = strings.TrimSpace(cfg.PercenterAdminSecret)
		if secret == "" {
			secret = strings.TrimSpace(cfg.BotInternalSecret)
		}
	}
	return &Server{
		redis:      redisClient,
		store:      store,
		clickhouse: clickhouseConn,
		cfg:        cfg,
		policy:     policy.Normalize(),
		secret:     secret,
		startedAt:  time.Now().UTC(),
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

func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s == nil || strings.TrimSpace(s.secret) == "" {
			writeAPIError(w, http.StatusServiceUnavailable, "admin secret is not configured")
			return
		}
		provided := strings.TrimSpace(r.Header.Get("X-Internal-Secret"))
		if provided == "" {
			authorization := strings.TrimSpace(r.Header.Get("Authorization"))
			if len(authorization) > len("Bearer ") && strings.EqualFold(authorization[:len("Bearer ")], "Bearer ") {
				provided = strings.TrimSpace(authorization[len("Bearer "):])
			}
		}
		if len(provided) != len(s.secret) || subtle.ConstantTimeCompare([]byte(provided), []byte(s.secret)) != 1 {
			writeAPIError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}
