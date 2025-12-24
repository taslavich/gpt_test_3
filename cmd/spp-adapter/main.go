package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-co-op/gocron"
	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/geoBadIp"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	httpServer "gitlab.com/twinbid-exchange/RTB-exchange/internal/http"
	sppAdapter "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/service"
	sppAdapterWeb "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/web"
)

func main() {
	workAdl := false
	workMc := false

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

	siteIdsAndDomains, err := utils.NewSiteIdsAndDomains(cfg.SiteIdDomainPath, cfg.Domains1LevelPath, cfg.Domains23LevelPath)
	if err != nil {
		log.Fatalf("failed to NewSiteIdsAndDomains: %v", err)
	}
	log.Printf("site id filename: %s", cfg.SiteIdDomainPath)

	s := gocron.NewScheduler(time.UTC)
	s.Every(30).Seconds().Do(func() {
		if err := siteIdsAndDomains.WriteSiteIdDomainToTheFile(); err != nil {
			log.Printf("Cannot WriteSiteIdDomainToTheFile: %v", err)
		}
		log.Println("Made WriteSiteIdDomainToTheFile")
	})
	go s.StartAsync()
	log.Println("⏰ Scheduler started (every 30 seconds)")

	router := chi.NewRouter()
	router.Use(httpServer.WorkSspAdapterMiddleware(&workAdl, &workMc))
	router = httpServer.InitHttpRouter(router)
	sppAdapterWeb.InitHttpRoutes(
		ctx,
		router,
		redisClient,
		badIp.IsBad,
		geoIp.GetCountryAndCityIdISO,
		client,
		cfg.GetWinnerBidTimeout,
		cfg.SspAdultFeeds,
		cfg.SspMainStreamFeeds,
		&workAdl,
		&workMc,
		siteIdsAndDomains,
	)
	log.Println("HTTP routes initialized")

	httpServer.RunHttpServer(ctx, router, cfg.HttpServer.Host, cfg.HttpServer.Port)
}
