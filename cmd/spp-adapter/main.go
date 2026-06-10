package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-co-op/gocron"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/geoBadIp"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	httpServer "gitlab.com/twinbid-exchange/RTB-exchange/internal/http"
	services "gitlab.com/twinbid-exchange/RTB-exchange/internal/services"
	redis_service "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/redis"
	sppAdapter "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/service"
	sppAdapterWeb "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/web"
)

func main() {
	workStatus := sppAdapterWeb.NewWorkStatus(false, false)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.LoadConfig[config.SppAdapterConfig](ctx)
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

	if err := redis_service.PingClients(ctx, "spp-adapter", redisClients.Ortb); err != nil {
		log.Fatalf("Failed to connect to Redis shards: %v", err)
	}
	log.Println("✅ Connected to Redis shards")

	if _, err := os.Stat(cfg.GeoIpDbPath); os.IsNotExist(err) {
		log.Fatalf("GeoIP file does not exist at path: %s", cfg.GeoIpDbPath)
	} else {
		log.Printf("GeoIP file exists: %s", cfg.GeoIpDbPath)
	}

	badIp, err := geoBadIp.NewBadIPService(cfg.GeoIpDbPath)
	if err != nil {
		log.Fatalf("failed to create bad ip service: %v", err)
	}

	geoIp, err := geoBadIp.NewGeoIPService(cfg.GeoIpDbPath)
	if err != nil {
		log.Fatalf("failed to create geo ip service: %v", err)
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

	geoToLang, err := geoBadIp.NewGeoToLang(cfg.GeoToLangPath)
	if err != nil {
		log.Fatalf("failed to NewGeoToLang: %v", err)
	}

	s := gocron.NewScheduler(time.UTC)
	s.Every(30).Seconds().Do(func() {
		if err := siteIdsAndDomains.WriteSiteIdDomainToTheFile(); err != nil {
			log.Printf("Cannot WriteSiteIdDomainToTheFile: %v", err)
		}
		log.Println("Made WriteSiteIdDomainToTheFile")
	})
	go s.StartAsync()
	log.Println("⏰ Scheduler started (every 30 seconds)")

	redisWriteErrorMonitor := services.NewRedisWriteErrorMonitor("SSP adapter ORTB", func(count uint64) {
services.StopAllSspAdapterOrtbStreams(ctx, cfg.SspAdapterWorkStatusURLs)
	})
	redisWriteErrorMonitor.Start()

	router := chi.NewRouter()
	router.Use(httpServer.WorkSspAdapterMiddleware(workStatus))
	router = httpServer.InitHttpRouter(router)
	sppAdapterWeb.InitHttpRoutes(
		ctx,
		router,
		redisClients.Ortb,
		badIp.IsBad,
		geoIp.GetCountryAndCityIdISO,
		client,
		cfg.GetWinnerBidTimeout,
		cfg.SspPopAdlFeeds,
		cfg.SspPopMcFeeds,
		cfg.SspIppAdlFeeds,
		cfg.SspIppMcFeeds,
		cfg.SspBanAdlFeeds,
		cfg.SspBanMcFeeds,
		cfg.SspNatAdlFeeds,
		cfg.SspNatMcFeeds,
		workStatus,
		siteIdsAndDomains,
		geoToLang,
		redisWriteErrorMonitor,
	)
	log.Println("HTTP routes initialized")

	httpServer.RunHttpServer(ctx, router, cfg.HttpServer.Host, cfg.HttpServer.Port)
}
