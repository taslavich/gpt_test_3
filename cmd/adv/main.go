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

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/go-chi/chi/v5"
	_ "github.com/lib/pq"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	advGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/adv"
	httpServer "gitlab.com/twinbid-exchange/RTB-exchange/internal/http"
	auction "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/adv/service"
	advWeb "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/adv/web"
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

	clickhouseAddr := net.JoinHostPort(
		cfg.ClickhouseConfig.Host,
		cfg.ClickhouseConfig.Port,
	)

	clickhouseConn, err := clickhouse.Open(
		&clickhouse.Options{
			Addr: []string{
				clickhouseAddr,
			},
			Protocol: clickhouse.Native,
			TLS: &tls.Config{
				MinVersion: tls.VersionTLS12,
			},
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
		log.Fatalf(
			"cannot initialize ADV ClickHouse client: %v",
			err,
		)
	}

	defer func() {
		if err := clickhouseConn.Close(); err != nil {
			log.Printf(
				"ADV ClickHouse close failed: %v",
				err,
			)
		}
	}()

	runtimeStore := auction.NewRuntimeStore(runtimeRedis, cfg.AdvPacingCurrentTTL, cfg.AdvPacingSlotTTL)
	winnerStore := auction.NewWinnerStore(winnerRedis, cfg.AdvWinnerTTL)
	auctionService := auction.NewAuctionService(runtimeStore, winnerStore, percentStore, qualityStore)
	if err := auctionService.RefreshFromPostgres(ctx, db); err != nil {
		log.Fatalf("initial ADV snapshot failed: %v", err)
	}

	auctionService.StartUserDSPPriceTicker(
		ctx,
		clickhouseConn,
		cfg.ClickhouseConfig.Database,
		func(err error) {
			log.Printf(
				"[ADV][USER_DSP_PRICE_MAP_ERROR] ClickHouse refresh failed; empty map published: %v",
				err,
			)
		},
	)

	auctionService.StartPostgresRefreshTicker(ctx, db, cfg.CampaignRefreshInterval, func(err error) {
		log.Printf("ADV snapshot refresh failed; previous snapshot retained: %v", err)
	})
	auctionService.StartPacingTicker(ctx, cfg.AdvPacingTickInterval, func(err error) {
		log.Printf("ADV pacing update failed: %v", err)
	})

	workController := advWeb.NewWorkController()
	grpcServer := grpc.NewServer()
	advGrpc.RegisterAdvServiceServer(grpcServer, advWeb.NewServer(auctionService, workController))

	router := httpServer.InitHttpRouter(chi.NewRouter())
	advWeb.InitHttpRoutes(router, percentStore, qualityStore, workController)

	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", cfg.GrpcServer.Host, cfg.GrpcServer.Port))
	if err != nil {
		log.Fatalf("ADV gRPC listen failed: %v", err)
	}
	errChan := make(chan error, 1)
	go httpServer.RunHttpServer(ctx, router, cfg.HttpServer.Host, cfg.HttpServer.Port)
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
	return nil
}
