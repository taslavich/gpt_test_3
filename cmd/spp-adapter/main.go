package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/geoBadIp"
	httpServer "gitlab.com/twinbid-exchange/RTB-exchange/internal/http"
	sppAdapter "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/service"
	sppAdapterWeb "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/web"
)

func main() {
	work := false

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.LoadConfig[config.SppAdapterConfig](ctx)
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

	if _, err := os.Stat(cfg.GeoIpDbPath); os.IsNotExist(err) {
		log.Fatalf("GeoIP file does not exist at path: %s", cfg.GeoIpDbPath)
	} else {
		log.Printf("GeoIP file exists: %s", cfg.GeoIpDbPath)
	}

	badIp, err := geoBadIp.NewBadIPService(cfg.GeoIpDbPath)
	if err != nil {
		log.Fatalf("failed to create bad ip service: %w", err)
	}

	geoIp, err := geoBadIp.NewGeoIPService(cfg.GeoIpDbPath)
	if err != nil {
		log.Fatalf("failed to create geo ip service: %w", err)
	}

	adapter := sppAdapter.NewSspAdapter(
		cfg.UriOfOrchestrator,
	)
	client, cancelFunc := adapter.GetGrpClient()
	defer cancelFunc()

	router := chi.NewRouter()
	router.Use(httpServer.WorkSspAdapterMiddleware(&work))
	router = httpServer.InitHttpRouter(router)
	sppAdapterWeb.InitHttpRoutes(
		ctx,
		router,
		redisClient,
		badIp.IsBad,
		geoIp.GetCountryISO,
		client,
		cfg.GetWinnerBidTimeout,
		cfg.SspFeeds,
		cfg.SspMainStreamFeeds,
		&work,
	)
	log.Println("HTTP routes initialized")

	httpServer.RunHttpServer(ctx, router, cfg.Host, cfg.Port)
}
