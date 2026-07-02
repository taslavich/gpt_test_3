package main

import (
	"context"
	"log"

	"github.com/go-chi/chi/v5"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	httpServer "gitlab.com/twinbid-exchange/RTB-exchange/internal/http"
	services "gitlab.com/twinbid-exchange/RTB-exchange/internal/services"
	redis_service "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/redis"
	sppAdapterWeb "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/web"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.LoadConfig[config.AdmAdapterConfig](ctx)
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
	defer func() {
		if err := redisClients.Close(); err != nil {
			log.Printf("⚠️ failed to close Redis clients: %v", err)
		}
	}()

	if err := redis_service.PingClients(ctx, "Imp", redisClients.Impressions); err != nil {
		log.Fatalf("Failed to connect to Imp Redis shards: %v", err)
	}
	log.Println("✅ Connected to Imp Redis shards")

	if err := redis_service.PingClients(ctx, "Clicks", redisClients.Clicks); err != nil {
		log.Fatalf("Failed to connect to Clicks Redis shards: %v", err)
	}
	log.Println("✅ Connected to Clicks Redis shards")

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
	defer func() {
		if err := redisAdmClient.Close(); err != nil {
			log.Printf("⚠️ failed to close ADM Redis client: %v", err)
		}
	}()

	redisBurlClient, err := redis_service.NewRedisClient(
		cfg.RedisUUIDAddr,
		cfg.RedisPassword,
		cfg.RedisDBBurl,
		cfg.RedisPoolSize,
		cfg.RedisMinIdleConns,
	)
	if err != nil {
		log.Fatalf("Cannot init BURL Redis client: %v", err)
	}
	defer func() {
		if err := redisBurlClient.Close(); err != nil {
			log.Printf("⚠️ failed to close BURL Redis client: %v", err)
		}
	}()

	if err := redisAdmClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to ADM Redis: %v", err)
	}
	if err := redisBurlClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to BURL Redis: %v", err)
	}
	log.Println("✅ Connected to ADM/BURL Redis")

	redisWriteErrorMonitor := services.NewRedisWriteErrorMonitorWithSettings(
		"adm-adapter",
		cfg.RedisWriteErrorLogThresholdPerSec,
		cfg.RedisWriteErrorStopThresholdPerSec,
		cfg.RedisWriteErrorMonitorTickerInterval,
		func(count uint64, workStatusURL string) {
			services.StopSspAdapterOrtbStreams(ctx, cfg.SspAdapterWorkStatusURL)
		},
		utils.NewBotMessage(cfg.BotBaseURL, cfg.BotInternalSecret),
		ctx,
	)
	redisWriteErrorMonitor.Start()

	admRouter := httpServer.InitHttpRouter(chi.NewRouter())
	sppAdapterWeb.InitHttpsRoutes(
		ctx,
		admRouter,
		redisClients.Clicks,
		redisAdmClient,
		cfg.RedisSetClicks,
		cfg.AdmTimeout,
		cfg.BurlTimeout,
		redisWriteErrorMonitor,
		cfg.SspAdapterWorkStatusURL,
		redisBurlClient,
		cfg.RedisSetImpressions,
		redisClients.Impressions,
	)
	log.Println("ADM HTTPS routes initialized")

	nurlBurlRouter := httpServer.InitHttpRouter(chi.NewRouter())
	sppAdapterWeb.InitNurlBurlRoutes(
		ctx,
		nurlBurlRouter,
		redisClients.Impressions,
		redisBurlClient,
		cfg.RedisSetImpressions,
		cfg.BurlTimeout,
		redisWriteErrorMonitor,
		cfg.SspAdapterWorkStatusURL,
	)
	log.Println("NURL/BURL HTTP routes initialized")

	go httpServer.RunHttpServer(ctx, nurlBurlRouter, cfg.HttpServer.Host, 80)
	httpServer.RunHttpsServerOptimized(ctx, admRouter, cfg.HttpServer.Host, cfg.HttpServer.Port, cfg.FullChain, cfg.PrivKey, cfg.RsaFullChain, cfg.RsaPrivKey)
}
