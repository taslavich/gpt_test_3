package main

import (
	"context"
	"fmt"
	"log"

	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	httpServer "gitlab.com/twinbid-exchange/RTB-exchange/internal/http"
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

	redisClient := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", cfg.RedisHost, cfg.RedisPort),
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	defer redisClient.Close()

	log.Println(cfg.RedisHost, cfg.RedisPort)

	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	log.Println("✅ Connected to Redis")

	router := httpServer.InitHttpRouter()
	sppAdapterWeb.InitHttpsRoutes(
		ctx,
		router,
		redisClient,
		cfg.AdmTimeout,
		cfg.NurlTimeout,
	)
	log.Println("HTTP routes initialized")

	httpServer.RunHttpsServer(ctx, router, cfg.Host, cfg.Port, cfg.FullChain, cfg.PrivKey)
}
