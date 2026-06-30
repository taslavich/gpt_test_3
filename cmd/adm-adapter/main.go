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
	defer func() {
		if err := redisNurlClient.Close(); err != nil {
			log.Printf("⚠️ failed to close NURL Redis client: %v", err)
		}
	}()

	if err := redisAdmClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to ADM Redis: %v", err)
	}
	if err := redisNurlClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to NURL Redis: %v", err)
	}
	log.Println("✅ Connected to ADM/NURL Redis")

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
		redisClients.Impressions,
		redisClients.Clicks,
		redisAdmClient,
		redisNurlClient,
		cfg.RedisSetImpressions,
		cfg.RedisSetClicks,
		cfg.AdmTimeout,
		cfg.NurlTimeout,
		redisWriteErrorMonitor,
		cfg.SspAdapterWorkStatusURL,
	)
	log.Println("ADM HTTPS routes initialized")

	nurlRouter := httpServer.InitHttpRouter(chi.NewRouter())
	sppAdapterWeb.InitNurlRoutes(
		ctx,
		nurlRouter,
		redisClients.Impressions,
		redisNurlClient,
		cfg.RedisSetImpressions,
		cfg.NurlTimeout,
		redisWriteErrorMonitor,
		cfg.SspAdapterWorkStatusURL,
	)
	log.Println("NURL HTTP routes initialized")

	go httpServer.RunHttpServer(ctx, nurlRouter, cfg.HttpServer.Host, 80)
	httpServer.RunHttpsServerOptimized(ctx, admRouter, cfg.HttpServer.Host, cfg.HttpServer.Port, cfg.FullChain, cfg.PrivKey, cfg.RsaFullChain, cfg.RsaPrivKey)
}
