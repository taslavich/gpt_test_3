package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-chi/chi/v5"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	bidEngineGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/bidEngine"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	httpServer "gitlab.com/twinbid-exchange/RTB-exchange/internal/http"
	services "gitlab.com/twinbid-exchange/RTB-exchange/internal/services"
	bidEngine "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/bidEngine/service"
	bidEngineWeb "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/bidEngine/web"
	redis_service "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/redis"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"

	"google.golang.org/grpc"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.LoadConfig[config.BiddingEngineConfig](ctx)
	if err != nil {
		log.Fatalf("Cannot load config: %v", err)
	}
	log.Println("Config initialized!")

	redisAddrs := cfg.RedisShardAddrs
	if cfg.RedisUseTLS {
		redisAddrs = cfg.RedisShardTLSAddrs
	}

	redisClients, err := redis_service.NewRedisShardedClients(
		redisAddrs,
		cfg.RedisPassword,
		cfg.RedisDBOrtb,
		cfg.RedisDBImpressions,
		cfg.RedisDBClicks,
		cfg.RedisUseTLS,
		cfg.RedisPoolSize,
		cfg.RedisMinIdleConns,
	)
	if err != nil {
		log.Fatalf("Cannot init redis shards: %v", err)
	}
	defer redisClients.Close()

	if err := redis_service.PingClients(ctx, "bid-engine", redisClients.Ortb); err != nil {
		log.Fatalf("Failed to connect to Redis shards: %v", err)
	}
	log.Println("✅ Connected to Redis shards")

	redisAdmClient, err := redis_service.NewRedisClient(
		cfg.RedisUUIDAddr,
		cfg.RedisPassword,
		cfg.RedisDBAdm,
		cfg.RedisPoolSize,
		cfg.RedisMinIdleConns,
	)
	if err != nil {
		log.Fatalf("Cannot init ADM Redis client: %v", err)
	}
	defer redisAdmClient.Close()

	redisNurlClient, err := redis_service.NewRedisClient(
		cfg.RedisUUIDAddr,
		cfg.RedisPassword,
		cfg.RedisDBNurl,
		cfg.RedisPoolSize,
		cfg.RedisMinIdleConns,
	)
	if err != nil {
		log.Fatalf("Cannot init NURL Redis client: %v", err)
	}
	defer redisNurlClient.Close()

	if err := redisAdmClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to ADM Redis: %v", err)
	}
	if err := redisNurlClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to NURL Redis: %v", err)
	}
	log.Println("✅ Connected to ADM/NURL Redis")

	sspGeoDspMapAdult, err := utils.InitSspGeoDspMap[*types.PercentAndBidfloor](cfg.SspGeoDspPercentsAdultFilePath)
	if err != nil {
		log.Fatalf("Failed to InitSspGeoPercentsLogic: %v", err)
	}

	sspGeoDspMapMainstream, err := utils.InitSspGeoDspMap[*types.PercentAndBidfloor](cfg.SspGeoDspPercentsMainstreamFilePath)
	if err != nil {
		log.Fatalf("Failed to InitSspGeoPercentsLogic: %v", err)
	}

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
	redisWriteErrorMonitor := services.NewRedisWriteErrorMonitorWithSettings(
		"bid-engine",
		cfg.RedisWriteErrorLogThresholdPerSec,
		cfg.RedisWriteErrorStopThresholdPerSec,
		cfg.RedisWriteErrorMonitorTickerInterval,
		func(count uint64, workStatusURL string) {
			services.StopSspAdapterOrtbStreams(ctx, workStatusURL)
		},
		utils.NewBotMessage(cfg.BotBaseURL, cfg.BotInternalSecret),
		ctx,
	)
	redisWriteErrorMonitor.Start()

	s := grpc.NewServer()
	bidEngineGrpc.RegisterBidEngineServiceServer(
		s,
		bidEngineWeb.NewServer(
			cfg.ProfitPercent,
			redisClients.Ortb,
			redisAdmClient,
			redisNurlClient,
			cfg.RedisUUIDKeyTTL,
			cfg.RedisSetOrtb,
			bidEngine.GetWinnerBidInternal_V_2_5,
			cfg.SspGeoDspPercentsAdultFilePath,
			&sspGeoDspMapAdult,
			cfg.SspGeoDspPercentsMainstreamFilePath,
			&sspGeoDspMapMainstream,
			cfg.AdmDomain,
			redisWriteErrorMonitor,
		),
	)

	router := httpServer.InitHttpRouter(chi.NewRouter())
	bidEngineWeb.InitHttpRoutes(
		router,
		cfg.SspGeoDspPercentsAdultFilePath,
		cfg.SspGeoDspPercentsMainstreamFilePath,
		&sspGeoDspMapAdult,
		&sspGeoDspMapMainstream,
	)
	log.Println("HTTP routes initialized")

	errChan := make(chan error)

	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

		select {
		case <-stop:
			log.Println("Shutting down gracefully...")
			s.GracefulStop()
		case err := <-errChan:
			log.Fatalf("Server crashed: %v", err)
		}
	}()

	go httpServer.RunHttpServer(ctx, router, cfg.HttpServer.Host, cfg.HttpServer.Port)

	log.Printf("Server started on %s:%d", cfg.GrpcServer.Host, cfg.GrpcServer.Port)
	if err := s.Serve(lis); err != nil {
		errChan <- err
		log.Printf("failed to serve: %v", err)
	}
}
