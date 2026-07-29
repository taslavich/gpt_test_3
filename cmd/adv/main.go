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
	"strings"
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
	redisService "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/redis"
	"google.golang.org/grpc"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.LoadConfig[config.AdvConfig](ctx)
	if err != nil {
		log.Fatalf("cannot load ADV config: %v", err)
	}
	if err := validateConfig(cfg); err != nil {
		log.Fatalf("invalid ADV config: %v", err)
	}
	redisAddr := strings.TrimSpace(cfg.RedisUUIDAddr)
	if redisAddr == "" && len(cfg.RedisShardAddrs) > 0 {
		redisAddr = strings.TrimSpace(cfg.RedisShardAddrs[0])
	}

	runtimeRedis, err := redisService.NewRedisClient(redisAddr, cfg.RedisPassword, cfg.RedisDBAdvRuntime, cfg.RedisPoolSize, cfg.RedisMinIdleConns)
	if err != nil {
		log.Fatalf("cannot initialize ADV runtime Redis DB %d: %v", cfg.RedisDBAdvRuntime, err)
	}
	defer runtimeRedis.Close()
	winnerRedis, err := redisService.NewRedisClient(redisAddr, cfg.RedisPassword, cfg.RedisDBAdvWinner, cfg.RedisPoolSize, cfg.RedisMinIdleConns)
	if err != nil {
		log.Fatalf("cannot initialize ADV winner Redis DB %d: %v", cfg.RedisDBAdvWinner, err)
	}
	defer winnerRedis.Close()
	if err := runtimeRedis.Ping(ctx).Err(); err != nil {
		log.Fatalf("ADV runtime Redis unavailable: %v", err)
	}
	if err := winnerRedis.Ping(ctx).Err(); err != nil {
		log.Fatalf("ADV winner Redis unavailable: %v", err)
	}

	percentStore, err := auction.NewPercentStore(cfg.AdvPercentMapFilePath)
	if err != nil {
		log.Fatalf("cannot initialize ADV percent map: %v", err)
	}
	qualityStore, err := auction.NewQualityStore(cfg.AdvQualityMapFilePath)
	if err != nil {
		log.Fatalf("cannot initialize ADV quality map: %v", err)
	}

	if strings.TrimSpace(cfg.PostgresDSN) == "" {
		log.Fatal("POSTGRES_DSN is required for ADV")
	}
	db, err := sql.Open("postgres", cfg.PostgresDSN)
	if err != nil {
		log.Fatalf("cannot open ADV PostgreSQL: %v", err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("ADV PostgreSQL unavailable: %v", err)
	}
	runtimeStore := auction.NewRuntimeStore(runtimeRedis, cfg.AdvPacingCurrentTTL, cfg.AdvPacingSlotTTL)
	winnerStore := auction.NewWinnerStore(winnerRedis, cfg.AdvWinnerTTL)
	auctionService := auction.NewAuctionService(runtimeStore, winnerStore, percentStore, qualityStore)
	auctionService.SetAntiPerekrutEnabled(cfg.AntiperekrutEnabled)

	botNotifier := utils.NewBotMessageWithTimeout(cfg.BotBaseURL, cfg.BotInternalSecret, cfg.AntiperekrutControlTimeout)
	var antiManager *auction.AntiPerekrutManager
	var startupEvent antiControl.StartupEvent

	if cfg.AntiperekrutEnabled {
		// Production rollout applies the additive migration from the cabinet first.
		// Automatic DDL is opt-in because the ADV database role may intentionally
		// have DML permissions without ALTER TABLE privileges.
		if cfg.AntiperekrutAutoMigrate {
			if err := auction.EnsureAntiPerekrutSchema(ctx, db); err != nil {
				log.Fatalf("cannot migrate antiperekrut schema: %v", err)
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
			log.Fatalf("cannot initialize ADV ClickHouse client: %v", err)
		}
		defer func() {
			if err := clickhouseConn.Close(); err != nil {
				log.Printf("ADV ClickHouse close failed: %v", err)
			}
		}()

		if err := auctionService.RefreshFromPostgres(ctx, db); err != nil {
			log.Fatalf("initial ADV snapshot failed: %v", err)
		}

		antiManager, err = auction.NewAntiPerekrutManager(
			db, clickhouseConn, cfg.ClickhouseConfig.Database, runtimeStore,
			auctionService.CurrentSnapshot, cfg.AntiperekrutTickOffset, botNotifier.SendTextMessageToBot,
		)
		if err != nil {
			log.Fatalf("cannot initialize antiperekrut: %v", err)
		}
		auctionService.SetAntiPerekrutManager(antiManager)

		hostname, _ := os.Hostname()
		startupEvent = antiControl.NewStartupEvent("adv", hostname)
		if _, err := antiManager.RegisterStartupEvent(ctx, startupEvent.EventID, startupEvent.SourceService, startupEvent.SourceInstance); err != nil {
			_ = botNotifier.SendTextMessageToBot(ctx, fmt.Sprintf("[ADV][ANTIPEREKRUT_STARTUP_ERROR] %v", err))
			log.Fatalf("cannot register ADV startup reset: %v", err)
		}
		if err := antiManager.Refresh(ctx); err != nil {
			_ = botNotifier.SendTextMessageToBot(ctx, fmt.Sprintf("[ADV][ANTIPEREKRUT_INITIAL_REFRESH_ERROR] %v", err))
			log.Fatalf("initial antiperekrut state failed: %v", err)
		}
		if state := antiManager.State(); state == nil || state.LoadedAt.IsZero() {
			log.Fatal("initial antiperekrut state has not completed ClickHouse/Redis loading")
		}
		antiManager.Start(ctx)
	} else {
		if err := auctionService.RefreshFromPostgres(ctx, db); err != nil {
			log.Fatalf("initial ADV snapshot failed: %v", err)
		}
		log.Print("antiperekrut is disabled by ANTIPEREKRUT_ENABLED=false")
	}

	auctionService.StartPostgresRefreshTicker(ctx, db, cfg.CampaignRefreshInterval, func(err error) {
		_ = botNotifier.SendTextMessageToBot(ctx, fmt.Sprintf("[ADV][SNAPSHOT_REFRESH_ERROR] %v", err))
		log.Printf("ADV snapshot refresh failed; previous snapshot retained: %v", err)
	})
	auctionService.StartPacingTicker(ctx, cfg.AdvPacingTickInterval, func(err error) {
		log.Printf("ADV pacing update failed: %v", err)
	})

	workController := advWeb.NewWorkController()
	grpcServer := grpc.NewServer()
	advGrpc.RegisterAdvServiceServer(grpcServer, advWeb.NewServer(auctionService, workController))

	router := httpServer.InitHttpRouter(chi.NewRouter())
	advWeb.InitHttpRoutes(router, percentStore, qualityStore, workController, advWeb.AntiPerekrutHTTPConfig{
		Manager: antiManager,
	})

	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", cfg.GrpcServer.Host, cfg.GrpcServer.Port))
	if err != nil {
		log.Fatalf("ADV gRPC listen failed: %v", err)
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
		}, startupEvent, botNotifier.SendTextMessageToBot)
		if err != nil {
			_ = botNotifier.SendTextMessageToBot(ctx, fmt.Sprintf("[ADV][ANTIPEREKRUT_FANOUT_ERROR] %v", err))
			log.Fatalf("ADV startup reset fan-out failed: %v", err)
		}
	}
	go func() {
		errChan <- grpcServer.Serve(listener)
	}()
	log.Printf("ADV started: grpc=%s:%d http=%s:%d", cfg.GrpcServer.Host, cfg.GrpcServer.Port, cfg.HttpServer.Host, cfg.HttpServer.Port)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case <-stop:
		cancel()
		grpcServer.GracefulStop()
	case err := <-errChan:
		if err != nil {
			log.Fatalf("ADV server stopped: %v", err)
		}
	}
}

func validateConfig(cfg *config.AdvConfig) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	if cfg.RedisDBAdvRuntime != 5 || cfg.RedisDBAdvWinner != 6 {
		return fmt.Errorf("ADV requires Redis DB 5 for runtime and DB 6 for winners")
	}
	if strings.TrimSpace(cfg.RedisUUIDAddr) == "" && len(cfg.RedisShardAddrs) == 0 {
		return fmt.Errorf("REDIS_UUID_ADDR or REDIS_SHARD_ADDRS is required")
	}
	if strings.TrimSpace(cfg.AdvPercentMapFilePath) == "" {
		return fmt.Errorf("ADV_PERCENT_MAP_FILE_PATH is required")
	}
	if strings.TrimSpace(cfg.AdvQualityMapFilePath) == "" {
		return fmt.Errorf("ADV_QUALITY_MAP_FILE_PATH is required")
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
