package main

import (
	"context"
	"crypto/tls"
	"database/sql"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/go-chi/chi/v5"
	_ "github.com/lib/pq"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	advGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/adv"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	httpServer "gitlab.com/twinbid-exchange/RTB-exchange/internal/http"
	auction "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/adv/service"
	advWeb "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/adv/web"
	antiControl "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/antiperekrut"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/percenter"
	redisService "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/redis"
	"google.golang.org/grpc"
)

func main() {
	if err := run(); err != nil {
		log.Printf("ADV terminated with error: %v", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.LoadConfig[config.AdvConfig](ctx)
	if err != nil {
		return fmt.Errorf("cannot load ADV config: %w", err)
	}
	if err := validateConfig(cfg); err != nil {
		return fmt.Errorf("invalid ADV config: %w", err)
	}
	simplePolicy, complexPolicy, err := percenter.PoliciesFromConfig(cfg.PercenterPolicyConfig)
	if err != nil {
		return fmt.Errorf("invalid ADV percenter policy config: %w", err)
	}
	log.Printf(
		"[ADV][PERCENTER_POLICY] simple_interval=%s simple_rebenchmark=%s simple_ttl=%s simple_min_impressions=%d simple_retention=%.2f simple_steps=%v complex_interval=%s complex_rebenchmark=%s complex_ttl=%s complex_min_impressions=%d complex_buyout_retention=%.2f complex_efficiency_retention=%.2f complex_ssp_steps=%v complex_margin_steps=%v max_margin=%.2f",
		simplePolicy.OptimizeInterval, simplePolicy.RebenchmarkInterval, simplePolicy.StateTTL, simplePolicy.MinImpressions, simplePolicy.WinRateRetention, simplePolicy.SearchStepsPP,
		complexPolicy.OptimizeInterval, complexPolicy.RebenchmarkInterval, complexPolicy.StateTTL, complexPolicy.MinImpressions, complexPolicy.BuyoutRetention, complexPolicy.EfficiencyRetention, complexPolicy.SSPSearchStepsPercent, complexPolicy.MarginSearchStepsPP, complexPolicy.MaxMargin,
	)
	redisAddr := strings.TrimSpace(cfg.RedisUUIDAddr)
	if redisAddr == "" && len(cfg.RedisShardAddrs) > 0 {
		redisAddr = strings.TrimSpace(cfg.RedisShardAddrs[0])
	}

	runtimeRedis, err := redisService.NewRedisClient(redisAddr, cfg.RedisPassword, cfg.RedisDBAdvRuntime, cfg.RedisPoolSize, cfg.RedisMinIdleConns)
	if err != nil {
		return fmt.Errorf("cannot initialize ADV runtime Redis DB %d: %w", cfg.RedisDBAdvRuntime, err)
	}
	defer runtimeRedis.Close()
	winnerRedis, err := redisService.NewRedisClient(redisAddr, cfg.RedisPassword, cfg.RedisDBAdvWinner, cfg.RedisPoolSize, cfg.RedisMinIdleConns)
	if err != nil {
		return fmt.Errorf("cannot initialize ADV winner Redis DB %d: %w", cfg.RedisDBAdvWinner, err)
	}
	defer winnerRedis.Close()
	percenterRedis, err := redisService.NewRedisClient(strings.TrimSpace(cfg.RedisADVAddr), cfg.RedisPassword, cfg.RedisDBAdvPercenter, cfg.RedisPoolSize, cfg.RedisMinIdleConns)
	if err != nil {
		return fmt.Errorf("cannot initialize ADV percenter Redis DB %d: %w", cfg.RedisDBAdvPercenter, err)
	}
	defer percenterRedis.Close()
	if err := runtimeRedis.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("ADV runtime Redis unavailable: %w", err)
	}
	if err := winnerRedis.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("ADV winner Redis unavailable: %w", err)
	}
	if err := percenterRedis.Ping(ctx).Err(); err != nil {
		// DB7 is fail-open for auction pricing. The percenter state stores will
		// return errors and AuctionService will use its last known local point or
		// a bounded baseline while durable telemetry keeps retrying.
		log.Printf("[ADV][PERCENTER_REDIS_DEGRADED] redis_db=%d error=%v", cfg.RedisDBAdvPercenter, err)
	}

	statsRedisAddrs := cfg.RedisShardAddrs
	if cfg.RedisUseTLS {
		statsRedisAddrs = cfg.RedisShardTLSAddrs
	}
	statsRedisClients, err := redisService.NewRedisShardedClientsForDB(
		statsRedisAddrs, cfg.RedisPassword, cfg.RedisDBOrtb, cfg.RedisUseTLS, cfg.RedisPoolSize, cfg.RedisMinIdleConns,
	)
	if err != nil {
		return fmt.Errorf("cannot initialize ADV ORTB stats Redis: %w", err)
	}
	defer func() {
		if err := redisService.CloseClients(statsRedisClients); err != nil {
			log.Printf("ADV stats Redis close failed: %v", err)
		}
	}()
	if err := redisService.PingClients(ctx, "adv-stats", statsRedisClients); err != nil {
		return fmt.Errorf("ADV ORTB stats Redis unavailable: %w", err)
	}

	percentStore, err := auction.NewPercentStore(cfg.AdvPercentMapFilePath)
	if err != nil {
		return fmt.Errorf("cannot initialize ADV percent map: %w", err)
	}
	qualityStore, err := auction.NewQualityStore(cfg.AdvQualityMapFilePath)
	if err != nil {
		return fmt.Errorf("cannot initialize ADV quality map: %w", err)
	}
	siteIDQualityStore, err := auction.NewSiteIDQualityStore(cfg.AdvSiteIDQualityMapFilePath)
	if err != nil {
		return fmt.Errorf("[ADV][SITE_ID_QUALITY_MAP][startup] initialization failed: %w", err)
	}
	vpnStore, err := auction.NewVPNStore(cfg.AdvVPNDBPath)
	if err != nil {
		return fmt.Errorf("[ADV][VPN_DB][startup] initialization failed: %w", err)
	}
	defer vpnStore.Close()

	if strings.TrimSpace(cfg.PostgresDSN) == "" {
		return fmt.Errorf("POSTGRES_DSN is required for ADV")
	}
	db, err := sql.Open("postgres", cfg.PostgresDSN)
	if err != nil {
		return fmt.Errorf("cannot open ADV PostgreSQL: %w", err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ADV PostgreSQL unavailable: %w", err)
	}
	runtimeStore := auction.NewRuntimeStore(runtimeRedis, cfg.AdvPacingCurrentTTL, cfg.AdvPacingSlotTTL)
	winnerStore := auction.NewWinnerStore(winnerRedis, cfg.AdvWinnerTTL)
	auctionService := auction.NewAuctionService(runtimeStore, winnerStore, percentStore, qualityStore, siteIDQualityStore)
	telemetryOutbox, err := percenter.OpenObservabilityOutbox(cfg.AdvPercenterTelemetryOutboxPath)
	if err != nil {
		return fmt.Errorf("cannot initialize ADV percenter telemetry outbox: %w", err)
	}
	defer telemetryOutbox.Close()
	hostname, _ := os.Hostname()
	// Instance identifies one process incarnation, not only the host. This avoids
	// EventID collisions if ADV is orderly restarted inside the same minute while
	// an earlier minute bucket from that host is still pending in bbolt.
	telemetryInstance := fmt.Sprintf("%s/%d/%d", hostname, os.Getpid(), time.Now().UTC().UnixNano())
	advTelemetry := percenter.NewADVTelemetry("adv", telemetryInstance, telemetryOutbox)
	auctionService.SetPercenterTelemetry(advTelemetry)
	telemetryFlush := cfg.AdvPercenterTelemetryFlush
	if telemetryFlush <= 0 {
		telemetryFlush = time.Minute
	}
	var telemetryWG sync.WaitGroup
	telemetryWG.Add(1)
	go func() {
		defer telemetryWG.Done()
		ticker := time.NewTicker(telemetryFlush)
		defer ticker.Stop()
		flushClosed := func(now time.Time) {
			if err := advTelemetry.Flush(now); err != nil {
				log.Printf("[ADV][PERCENTER_TELEMETRY_OUTBOX_ERROR] %v", err)
			}
			if err := percenter.RelayObservabilityOutbox(ctx, telemetryOutbox, percenterRedis); err != nil {
				log.Printf("[ADV][PERCENTER_TELEMETRY_RELAY_ERROR] %v", err)
			}
		}
		flushClosed(time.Now().UTC())
		for {
			select {
			case <-ctx.Done():
				shutdownAt := time.Now().UTC()
				if err := advTelemetry.FlushAll(shutdownAt); err != nil {
					log.Printf("[ADV][PERCENTER_TELEMETRY_SHUTDOWN_OUTBOX_ERROR] %v", err)
				}
				// Use a short independent context: the parent is already cancelled, but
				// a best-effort final relay must still be allowed to persist to Redis.
				relayCtx, relayCancel := context.WithTimeout(context.Background(), 2*time.Second)
				if err := percenter.RelayObservabilityOutbox(relayCtx, telemetryOutbox, percenterRedis); err != nil {
					log.Printf("[ADV][PERCENTER_TELEMETRY_SHUTDOWN_RELAY_ERROR] %v", err)
				}
				relayCancel()
				return
			case now := <-ticker.C:
				flushClosed(now.UTC())
			}
		}
	}()
	// Any startup error after the telemetry worker is running must cancel and
	// wait for it before deferred outbox/Redis closes execute. Runtime/signal
	// shutdown normally completes this wait first, making this defer a no-op.
	defer func() {
		cancel()
		waitForADVBackground(&telemetryWG, 4*time.Second)
	}()
	auctionService.ConfigureSimplePercenter(percenter.NewSimpleStateStore(percenterRedis, simplePolicy), simplePolicy)
	auctionService.ConfigureComplexPercenter(percenter.NewComplexStateStore(percenterRedis, complexPolicy), complexPolicy)
	auctionService.SetStatsRedisClients(statsRedisClients)
	auctionService.SetVPNClassifier(vpnStore)
	auctionService.SetAntiPerekrutEnabled(cfg.AntiperekrutEnabled)
	auctionService.StartDiagnostics(ctx)
	diagnosticsEnabled, err := boolEnvironment("AUCTION_DIAGNOSTICS_ENABLED", false)
	if err != nil {
		return fmt.Errorf("invalid AUCTION_DIAGNOSTICS_ENABLED: %w", err)
	}
	auctionService.SetDiagnosticsEnabled(diagnosticsEnabled)

	botNotifier := utils.NewBotMessageWithTimeout(cfg.BotBaseURL, cfg.BotInternalSecret, cfg.AntiperekrutControlTimeout)
	telegramConfigured := strings.TrimSpace(cfg.BotBaseURL) != "" && strings.TrimSpace(cfg.BotInternalSecret) != ""
	botSend := func(sendCtx context.Context, text string) error {
		if !telegramConfigured {
			return nil
		}
		return botNotifier.SendTextMessageToBot(sendCtx, text)
	}
	if telegramConfigured {
		auctionService.SetSnapshotWarningNotifier(botSend)
	} else {
		log.Printf("[ADV][TELEGRAM_DISABLED] credentials are not configured; auction continues without snapshot/percenter warnings")
	}
	var antiManager *auction.AntiPerekrutManager
	var startupEvent antiControl.StartupEvent

	if cfg.AntiperekrutEnabled {
		// Production rollout applies the additive migration from the cabinet first.
		// Automatic DDL is opt-in because the ADV database role may intentionally
		// have DML permissions without ALTER TABLE privileges.
		if cfg.AntiperekrutAutoMigrate {
			if err := auction.EnsureAntiPerekrutSchema(ctx, db); err != nil {
				return fmt.Errorf("cannot migrate antiperekrut schema: %w", err)
			}
		}

		clickhouseAddr := net.JoinHostPort(
			cfg.ClickhouseConfig.Host,
			cfg.ClickhouseConfig.Port,
		)
		clickhouseConn, err := clickhouse.Open(
			&clickhouse.Options{
				Addr:     []string{clickhouseAddr},
				Protocol: clickhouse.Native,
				TLS:      &tls.Config{MinVersion: tls.VersionTLS12},
				Auth: clickhouse.Auth{
					Username: cfg.ClickhouseConfig.Username,
					Password: cfg.ClickhouseConfig.Password,
					Database: cfg.ClickhouseConfig.Database,
				},
				MaxOpenConns: 2,
				MaxIdleConns: 2,
			},
		)
		if err != nil {
			return fmt.Errorf("cannot initialize ADV ClickHouse client: %w", err)
		}
		defer func() {
			if err := clickhouseConn.Close(); err != nil {
				log.Printf("ADV ClickHouse close failed: %v", err)
			}
		}()

		if err := auctionService.RefreshFromPostgres(ctx, db); err != nil {
			return fmt.Errorf("initial ADV snapshot failed: %w", err)
		}

		antiManager, err = auction.NewAntiPerekrutManager(
			db, clickhouseConn, cfg.ClickhouseConfig.Database, runtimeStore,
			auctionService.CurrentSnapshot, cfg.AntiperekrutTickOffset, botSend,
		)
		if err != nil {
			return fmt.Errorf("cannot initialize antiperekrut: %w", err)
		}
		auctionService.SetAntiPerekrutManager(antiManager)

		antiperekrutHostname, _ := os.Hostname()
		startupEvent = antiControl.NewStartupEvent("adv", antiperekrutHostname)
		if _, err := antiManager.RegisterStartupEvent(ctx, startupEvent.EventID, startupEvent.SourceService, startupEvent.SourceInstance); err != nil {
			_ = botSend(ctx, fmt.Sprintf("[ADV][ANTIPEREKRUT_STARTUP_ERROR] %v", err))
			return fmt.Errorf("cannot register ADV startup reset: %w", err)
		}
		if err := antiManager.Refresh(ctx); err != nil {
			_ = botSend(ctx, fmt.Sprintf("[ADV][ANTIPEREKRUT_INITIAL_REFRESH_ERROR] %v", err))
			return fmt.Errorf("initial antiperekrut state failed: %w", err)
		}
		if state := antiManager.State(); state == nil || state.LoadedAt.IsZero() {
			return fmt.Errorf("initial antiperekrut state has not completed ClickHouse/Redis loading")
		}
		antiManager.Start(ctx)
	} else {
		if err := auctionService.RefreshFromPostgres(ctx, db); err != nil {
			return fmt.Errorf("initial ADV snapshot failed: %w", err)
		}
		log.Print("antiperekrut is disabled by ANTIPEREKRUT_ENABLED=true")
	}
	log.Printf("AdvServiceControlURLs: %q", cfg.AdvServiceControlURLs)

	auctionService.StartPostgresRefreshTicker(ctx, db, cfg.CampaignRefreshInterval, func(err error) {
		_ = botSend(ctx, fmt.Sprintf("[ADV][SNAPSHOT_REFRESH_ERROR] %v", err))
		log.Printf("ADV snapshot refresh failed; previous snapshot retained: %v", err)
	})
	auctionService.StartPacingTicker(ctx, cfg.AdvPacingTickInterval, func(err error) {
		log.Printf("ADV pacing update failed: %v", err)
	})

	workController := advWeb.NewWorkController()
	grpcServer := grpc.NewServer()
	advGrpc.RegisterAdvServiceServer(grpcServer, advWeb.NewServer(auctionService, workController))

	router := httpServer.InitHttpRouter(chi.NewRouter())
	advWeb.InitHttpRoutes(router, percentStore, qualityStore, siteIDQualityStore, workController, advWeb.AntiPerekrutHTTPConfig{
		Manager:        antiManager,
		AuctionService: auctionService,
	})

	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", cfg.GrpcServer.Host, cfg.GrpcServer.Port))
	if err != nil {
		return fmt.Errorf("ADV gRPC listen failed: %w", err)
	}
	errChan := make(chan error, 1)
	// The control endpoint must be reachable before startup fan-out, while the
	// auction gRPC server must not become ready until the initial attempt has
	// addressed every configured ADV URL and at least one durable ACK exists.
	go httpServer.RunHttpServer(ctx, router, cfg.HttpServer.Host, cfg.HttpServer.Port)
	if cfg.AntiperekrutEnabled {
		err = antiControl.FanoutStartupEvent(ctx, antiControl.ClientConfig{
			Enabled: true, URLs: []string(cfg.AdvServiceControlURLs),
			RequestTimeout: cfg.AntiperekrutControlTimeout, RetryInitial: cfg.AntiperekrutRetryInitial, RetryMax: cfg.AntiperekrutRetryMax,
		}, startupEvent, botSend)
		if err != nil {
			_ = botSend(ctx, fmt.Sprintf("[ADV][ANTIPEREKRUT_FANOUT_ERROR] %v", err))
			return fmt.Errorf("ADV startup reset fan-out failed: %w", err)
		}
	}
	go func() {
		errChan <- grpcServer.Serve(listener)
	}()
	log.Printf("ADV started: grpc=%s:%d http=%s:%d", cfg.GrpcServer.Host, cfg.GrpcServer.Port, cfg.HttpServer.Host, cfg.HttpServer.Port)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)
	return waitForADVTermination(
		stop,
		errChan,
		cancel,
		grpcServer.GracefulStop,
		grpcServer.Stop,
		&telemetryWG,
		4*time.Second,
		4*time.Second,
	)
}

func boolEnvironment(name string, fallback bool) (bool, error) {
	raw, exists := os.LookupEnv(name)
	if !exists || strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return false, fmt.Errorf("%s must be true or false: %w", name, err)
	}
	return value, nil
}

func validateConfig(cfg *config.AdvConfig) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	if cfg.RedisDBAdvRuntime != 5 || cfg.RedisDBAdvWinner != 6 || cfg.RedisDBAdvPercenter != 7 {
		return fmt.Errorf("ADV requires Redis DB 5 for runtime, DB 6 for winners, and DB 7 for Simple percenter state")
	}
	if strings.TrimSpace(cfg.RedisUUIDAddr) == "" && len(cfg.RedisShardAddrs) == 0 {
		return fmt.Errorf("REDIS_UUID_ADDR or REDIS_SHARD_ADDRS is required")
	}
	if strings.TrimSpace(cfg.RedisADVAddr) == "" {
		return fmt.Errorf("REDIS_ADV_ADDR is required for percenter state")
	}
	if strings.TrimSpace(cfg.AdvPercentMapFilePath) == "" {
		return fmt.Errorf("ADV_PERCENT_MAP_FILE_PATH is required")
	}
	if strings.TrimSpace(cfg.AdvQualityMapFilePath) == "" {
		return fmt.Errorf("ADV_QUALITY_MAP_FILE_PATH is required")
	}
	if strings.TrimSpace(cfg.AdvSiteIDQualityMapFilePath) == "" {
		return fmt.Errorf("ADV_SITE_ID_QUALITY_MAP_FILE_PATH is required")
	}
	if strings.TrimSpace(cfg.AdvVPNDBPath) == "" {
		return fmt.Errorf("ADV_VPN_DB_PATH is required")
	}
	if strings.TrimSpace(cfg.PostgresDSN) == "" {
		return fmt.Errorf("POSTGRES_DSN is required")
	}
	if cfg.CampaignRefreshInterval <= 0 || cfg.AdvWinnerTTL <= 0 || cfg.AdvPacingTickInterval <= 0 || cfg.AdvPacingCurrentTTL <= 0 || cfg.AdvPacingSlotTTL <= 0 {
		return fmt.Errorf("ADV durations must be positive")
	}
	if cfg.AntiperekrutEnabled {
		if cfg.AntiperekrutTickOffset < 0 || cfg.AntiperekrutTickOffset >= time.Minute {
			return fmt.Errorf("ANTIPEREKRUT_TICK_OFFSET must be in [0,1m)")
		}
		if len(cfg.AdvServiceControlURLs) == 0 {
			return fmt.Errorf("antiperekrut requires ADV_SERVICE_CONTROL_URLS")
		}
		if strings.TrimSpace(cfg.BotBaseURL) == "" || strings.TrimSpace(cfg.BotInternalSecret) == "" {
			return fmt.Errorf("antiperekrut requires BOT_BASE_URL and BOT_INTERNAL_SECRET")
		}
	}
	return nil
}

func waitForADVTermination(
	stop <-chan os.Signal,
	errChan <-chan error,
	cancel context.CancelFunc,
	gracefulStop func(),
	forceStop func(),
	telemetryWG *sync.WaitGroup,
	grpcGracefulTimeout time.Duration,
	backgroundTimeout time.Duration,
) error {
	select {
	case <-stop:
		cancel()
		if !stopADVGRPCServerBounded(gracefulStop, forceStop, grpcGracefulTimeout) {
			log.Printf("[ADV][GRPC_FORCE_STOP] GracefulStop exceeded %s; forced Stop was requested", grpcGracefulTimeout)
		}
		if !waitForADVBackground(telemetryWG, backgroundTimeout) {
			return fmt.Errorf("ADV telemetry shutdown timed out after %s", backgroundTimeout)
		}
		return nil
	case runtimeErr := <-errChan:
		// Serve has already terminated. Cancel first so the telemetry worker
		// executes FlushAll + its final relay, then wait only for a bounded
		// interval before returning the original server error to run(). run()
		// returns before main calls os.Exit(1), so all run() defers execute.
		cancel()
		if !waitForADVBackground(telemetryWG, backgroundTimeout) {
			if runtimeErr != nil {
				return fmt.Errorf("ADV server stopped: %w; telemetry shutdown timed out after %s", runtimeErr, backgroundTimeout)
			}
			return fmt.Errorf("ADV telemetry shutdown timed out after %s", backgroundTimeout)
		}
		return runtimeErr
	}
}

func stopADVGRPCServerBounded(gracefulStop, forceStop func(), timeout time.Duration) bool {
	if gracefulStop == nil {
		return true
	}
	if timeout <= 0 {
		if forceStop != nil {
			forceStop()
		}
		return false
	}

	done := make(chan struct{})
	go func() {
		gracefulStop()
		close(done)
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		if forceStop != nil {
			forceStop()
		}
		return false
	}
}

func waitForADVBackground(wg *sync.WaitGroup, timeout time.Duration) bool {
	if wg == nil {
		return true
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		log.Printf("[ADV][SHUTDOWN_TIMEOUT] telemetry worker did not stop within %s", timeout)
		return false
	}
}
