package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-chi/chi/v5"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	dbpkg "gitlab.com/twinbid-exchange/RTB-exchange/internal/db"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/filter"
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
		log.Fatalf("Cannot load config: %v", err)
	}
	log.Println("Config initialized!")

	redisAddr := cfg.RedisUUIDAddr
	if redisAddr == "" && len(cfg.RedisShardAddrs) > 0 {
		redisAddr = cfg.RedisShardAddrs[0]
	}

	advRuntimeRedisClient, err := redisService.NewRedisClient(redisAddr, cfg.RedisPassword, cfg.RedisDBAdvRuntime, cfg.RedisPoolSize, cfg.RedisMinIdleConns)
	if err != nil {
		log.Fatalf("Cannot init ADV runtime redis client: %v", err)
	}
	defer advRuntimeRedisClient.Close()
	advWinnerRedisClient, err := redisService.NewRedisClient(redisAddr, cfg.RedisPassword, cfg.RedisDBAdvWinner, cfg.RedisPoolSize, cfg.RedisMinIdleConns)
	if err != nil {
		log.Fatalf("Cannot init ADV winner redis client: %v", err)
	}
	defer advWinnerRedisClient.Close()
	if err := advRuntimeRedisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to ADV runtime redis: %v", err)
	}
	if err := advWinnerRedisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to ADV winner redis: %v", err)
	}

	processor := filter.NewOptimizedFilterProcessor(filter.NewRuleManager())
	auctionService := auction.NewAuctionService(processor)
	auctionService.SetRuntimeRedis(advRuntimeRedisClient, cfg.PacingCurrentTTL)
	percentStore := auction.NewPercentStore(cfg.SspGeoDspPercentsAdultFilePath, cfg.SspGeoDspPercentsMainstreamFilePath)
	if err := percentStore.LoadInitial(); err != nil {
		log.Fatalf("Failed to load ADV percent store: %v", err)
	}
	auctionService.SetPercentStore(percentStore)
	if cfg.AdvQualityMapFilePath != "" {
		qualityStore, err := auction.LoadQualityStore(cfg.AdvQualityMapFilePath)
		if err != nil {
			log.Fatalf("Failed to load ADV quality map: %v", err)
		}
		auctionService.SetQualityStore(qualityStore)
	}

	if cfg.PostgresDSN != "" {
		db, err := sql.Open("postgres", cfg.PostgresDSN)
		if err != nil {
			log.Fatalf("Cannot open ADV postgres: %v", err)
		}
		defer db.Close()
		if err := dbpkg.MigrateUserGoalSpent(ctx, db); err != nil {
			log.Fatalf("Cannot migrate user goal/spent columns: %v", err)
		}
		auctionService.StartPostgresRefreshTicker(ctx, db, cfg.CampaignRefreshInterval)
	}

	s := grpc.NewServer()
	advServer := advWeb.NewServer(auctionService, advRuntimeRedisClient, advWinnerRedisClient, cfg.AdvWinnerTTL)
	advGrpc.RegisterAdvServiceServer(s, advServer)

	router := httpServer.InitHttpRouter(chi.NewRouter())
	advWeb.InitHttpRoutes(
		router,
		cfg.SspGeoDspPercentsAdultFilePath,
		cfg.SspGeoDspPercentsMainstreamFilePath,
		percentStore,
	)
	advWeb.InitWorkStatusRoutes(router, advServer.WorkController())
	log.Println("HTTP routes initialized")

	errChan := make(chan error)

	lis, err := net.Listen(
		"tcp",
		fmt.Sprintf(
			"%s:%d",
			cfg.GrpcServer.Host,
			cfg.GrpcServer.Port,
		),
	)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	go httpServer.RunHttpServer(ctx, router, cfg.HttpServer.Host, cfg.HttpServer.Port)

	log.Printf("Server started on %s:%d", cfg.GrpcServer.Host, cfg.GrpcServer.Port)
	go func() {
		if err := s.Serve(lis); err != nil {
			errChan <- err
			log.Printf("failed to serve: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case <-stop:
		log.Println("Shutting down gracefully...")
		s.GracefulStop()
	case err := <-errChan:
		log.Fatalf("Server crashed: %v", err)
	}
}
