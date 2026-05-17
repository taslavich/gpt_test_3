package main

import (
	"context"
	"fmt"
	"log"

	"github.com/go-chi/chi/v5"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	httpServer "gitlab.com/twinbid-exchange/RTB-exchange/internal/http"
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

	redisClientImp, redisClientClicks := redis_service.NewRedisImpClicksClients(
		fmt.Sprintf("%s:%s", cfg.RedisHost, cfg.RedisPort),
		cfg.RedisPassword,
		cfg.RedisDBImpressions,
		cfg.RedisDBClicks,
	)
	defer redisClientImp.Close()
	defer redisClientClicks.Close()

	log.Println(cfg.RedisHost, cfg.RedisPort)

	if err := redisClientImp.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Imp Redis: %v", err)
	}
	log.Println("✅ Connected to Imp Redis")

	if err := redisClientClicks.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Clicks Redis: %v", err)
	}
	log.Println("✅ Connected to Clicks Redis")

	router := httpServer.InitHttpRouter(chi.NewRouter())
	sppAdapterWeb.InitHttpsRoutes(
		ctx,
		router,
		redisClientImp,
		redisClientClicks,
		cfg.AdmTimeout,
		cfg.NurlTimeout,
	)
	log.Println("HTTP routes initialized")

	httpServer.RunHttpsServerOptimized(ctx, router, cfg.HttpServer.Host, cfg.HttpServer.Port, cfg.FullChain, cfg.PrivKey, cfg.RsaFullChain, cfg.RsaPrivKey)
}
