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
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/filter"
	advGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/adv"
	httpServer "gitlab.com/twinbid-exchange/RTB-exchange/internal/http"
	auction "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/adv/service"
	advWeb "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/adv/web"
	kafkaService "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/kafka"
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

	userBalanceThresholdRedisClient, err := redisService.NewRedisClient(
		redisAddr,
		cfg.RedisPassword,
		cfg.RedisDBUserBalanceThreshold,
		cfg.RedisPoolSize,
		cfg.RedisMinIdleConns,
	)
	if err != nil {
		log.Fatalf("Cannot init user balance threshold redis client: %v", err)
	}
	defer userBalanceThresholdRedisClient.Close()

	userBalanceSpentRedisClient, err := redisService.NewRedisClient(
		redisAddr,
		cfg.RedisPassword,
		cfg.RedisDBUserBalanceSpent,
		cfg.RedisPoolSize,
		cfg.RedisMinIdleConns,
	)
	if err != nil {
		log.Fatalf("Cannot init user balance spent redis client: %v", err)
	}
	defer userBalanceSpentRedisClient.Close()

	campaignBalanceThresholdRedisClient, err := redisService.NewRedisClient(
		redisAddr,
		cfg.RedisPassword,
		cfg.RedisDBCampaignBalanceThreshold,
		cfg.RedisPoolSize,
		cfg.RedisMinIdleConns,
	)
	if err != nil {
		log.Fatalf("Cannot init campaign balance threshold redis client: %v", err)
	}
	defer campaignBalanceThresholdRedisClient.Close()

	campaignBalanceSpentRedisClient, err := redisService.NewRedisClient(
		redisAddr,
		cfg.RedisPassword,
		cfg.RedisDBCampaignBalanceSpent,
		cfg.RedisPoolSize,
		cfg.RedisMinIdleConns,
	)
	if err != nil {
		log.Fatalf("Cannot init campaign balance spent redis client: %v", err)
	}
	defer campaignBalanceSpentRedisClient.Close()

	if err := userBalanceThresholdRedisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to user balance threshold redis: %v", err)
	}
	if err := userBalanceSpentRedisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to user balance spent redis: %v", err)
	}
	if err := campaignBalanceThresholdRedisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to campaign balance threshold redis: %v", err)
	}
	if err := campaignBalanceSpentRedisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to campaign balance spent redis: %v", err)
	}

	advKafkaWriters, err := kafkaService.CreateAdvKafkaWriters(cfg.KafkaConfig)
	if err != nil {
		log.Fatalf("Cannot init ADV kafka writers: %v", err)
	}
	defer func() {
		if err := advKafkaWriters.Close(); err != nil {
			log.Printf("⚠️ failed to close ADV kafka writers: %v", err)
		}
	}()

	advKafkaReaders, err := kafkaService.CreateAdvKafkaReaders(cfg.KafkaConfig)
	if err != nil {
		log.Fatalf("Cannot init ADV kafka readers: %v", err)
	}
	defer func() {
		if err := advKafkaReaders.Close(); err != nil {
			log.Printf("⚠️ failed to close ADV kafka readers: %v", err)
		}
	}()

	processor := filter.NewOptimizedFilterProcessor(filter.NewRuleManager())
	auctionService := auction.NewAuctionService(processor)

	s := grpc.NewServer()
	advGrpc.RegisterAdvServiceServer(
		s,
		advWeb.NewServer(auctionService, userBalanceThresholdRedisClient, userBalanceSpentRedisClient, campaignBalanceThresholdRedisClient, campaignBalanceSpentRedisClient),
	)

	router := httpServer.InitHttpRouter(chi.NewRouter())
	advWeb.InitHttpRoutes(router)
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
