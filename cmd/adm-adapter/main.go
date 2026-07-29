package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	httpServer "gitlab.com/twinbid-exchange/RTB-exchange/internal/http"
	services "gitlab.com/twinbid-exchange/RTB-exchange/internal/services"
	antiControl "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/antiperekrut"
	redis_service "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/redis"
	billing "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/billing"
	outbox "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/outbox"
	sppAdapterWeb "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/web"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.LoadConfig[config.AdmAdapterConfig](ctx)
	if err != nil {
		log.Fatalf("Cannot load config: %v", err)
	}
	if err := validateConfig(cfg); err != nil {
		log.Fatalf("Invalid ADM adapter config: %v", err)
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
		cfg.RedisDBConversions,
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

	advRuntimeRedis := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisADVAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDBAdvRuntime,
	})
	if err != nil {
		log.Fatalf("Cannot init ADV runtime Redis: %v", err)
	}
	defer advRuntimeRedis.Close()
	advWinnerRedis := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisADVAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDBAdvWinner,
	})
	if err != nil {
		log.Fatalf("Cannot init ADV winner Redis: %v", err)
	}
	defer advWinnerRedis.Close()
	if err := advRuntimeRedis.Ping(ctx).Err(); err != nil {
		log.Fatalf("ADV runtime Redis unavailable: %v", err)
	}
	if err := advWinnerRedis.Ping(ctx).Err(); err != nil {
		log.Fatalf("ADV winner Redis unavailable: %v", err)
	}

	advOutbox, err := outbox.Open(cfg.AdvOutboxPath)
	if err != nil {
		log.Fatalf("Cannot open ADV billing outbox: %v", err)
	}
	defer advOutbox.Close()
	advBillingStore := billing.NewStore(advRuntimeRedis, advWinnerRedis, cfg.AdvAppliedMarkerTTL)

	redisWriteErrorMonitor := services.NewRedisWriteErrorMonitorWithSettings(
		"adm-adapter",
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

	pendingRecords, err := advOutbox.List()
	if err != nil {
		log.Fatalf("Cannot inspect ADV billing outbox: %v", err)
	}
	log.Printf("AdvServiceControlURLs: %q", cfg.AdvServiceControlURLs)
	if len(pendingRecords) > 0 {
		pendingErr := fmt.Errorf("ADV billing outbox contains %d pending events after startup", len(pendingRecords))
		statuses := services.SetADVWorkStatus(ctx, []string(cfg.AdvServiceControlURLs), false)
		redisWriteErrorMonitor.RecordForURL(pendingErr, cfg.SspAdapterWorkStatusURL)
		message := fmt.Sprintf("ADV remains disabled while durable billing outbox is pending: error=%v adv_statuses=%v", pendingErr, statuses)
		log.Print(message)
		if notifyErr := redisWriteErrorMonitor.NotifyNowForRecordedError(message); notifyErr != nil {
			log.Printf("ADV outbox startup notification failed: %v", notifyErr)
		}
	}
	billing.NewWorker(advBillingStore, advOutbox, cfg.AdvOutboxRetryInterval, cfg.AdvOutboxMaxBackoff).Start(ctx)

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
		cfg.RedisSetConversions,
		redisClients.Conversions,
		advBillingStore,
		advOutbox,
		[]string(cfg.AdvServiceControlURLs),
	)
	log.Println("ADM HTTPS routes initialized")

	/*nurlBurlRouter := httpServer.InitHttpRouter(chi.NewRouter())
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

	go httpServer.RunHttpServer(ctx, nurlBurlRouter, cfg.HttpServer.Host, 80)*/
	if cfg.AntiperekrutEnabled {
		if len(cfg.AdvServiceControlURLs) == 0 {
			log.Fatal("antiperekrut startup reset requires ADV_SERVICE_CONTROL_URLS")
		}
		if strings.TrimSpace(cfg.BotBaseURL) == "" || strings.TrimSpace(cfg.BotInternalSecret) == "" {
			log.Fatal("antiperekrut startup reset requires BOT_BASE_URL and BOT_INTERNAL_SECRET")
		}
		startupHost, _ := os.Hostname()
		startupNotifier := utils.NewBotMessageWithTimeout(cfg.BotBaseURL, cfg.BotInternalSecret, cfg.AntiperekrutControlTimeout)
		startupEvent := antiControl.NewStartupEvent("adm-adapter", startupHost)
		if err := antiControl.FanoutStartupEvent(ctx, antiControl.ClientConfig{
			Enabled:        true,
			URLs:           []string(cfg.AdvServiceControlURLs),
			RequestTimeout: cfg.AntiperekrutControlTimeout,
			RetryInitial:   cfg.AntiperekrutRetryInitial,
			RetryMax:       cfg.AntiperekrutRetryMax,
		}, startupEvent, startupNotifier.SendTextMessageToBot); err != nil {
			_ = startupNotifier.SendTextMessageToBot(ctx, fmt.Sprintf("[adm-adapter][ANTIPEREKRUT_STARTUP_ERROR] %v", err))
			log.Fatalf("cannot deliver antiperekrut startup event: %v", err)
		}
	} else {
		log.Print("antiperekrut startup reset is disabled by ANTIPEREKRUT_ENABLED=true")
	}

	httpServer.RunHttpsServerOptimized(ctx, admRouter, cfg.HttpServer.Host, cfg.HttpServer.Port, cfg.FullChain, cfg.PrivKey, cfg.RsaFullChain, cfg.RsaPrivKey)
}

func validateConfig(cfg *config.AdmAdapterConfig) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	if cfg.RedisDBAdvRuntime != 5 || cfg.RedisDBAdvWinner != 6 {
		return fmt.Errorf("ADV billing requires Redis DB 5 for runtime and DB 6 for winners")
	}
	if strings.TrimSpace(cfg.RedisUUIDAddr) == "" {
		return fmt.Errorf("REDIS_UUID_ADDR is required")
	}
	if strings.TrimSpace(cfg.AdvOutboxPath) == "" {
		return fmt.Errorf("ADV_OUTBOX_PATH is required")
	}
	if strings.TrimSpace(cfg.SspAdapterWorkStatusURL) == "" {
		return fmt.Errorf("SSP_ADAPTER_WORK_STATUS_URL is required for the existing Redis error monitor")
	}
	if cfg.AdvOutboxRetryInterval <= 0 || cfg.AdvOutboxMaxBackoff <= 0 || cfg.AdvAppliedMarkerTTL <= 0 {
		return fmt.Errorf("ADV outbox durations must be positive")
	}
	validControlURL := false
	for _, rawURL := range cfg.AdvServiceControlURLs {
		if strings.TrimSpace(rawURL) != "" {
			validControlURL = true
			break
		}
	}
	if !validControlURL {
		return fmt.Errorf("ADV_SERVICE_CONTROL_URLS must contain at least one non-empty URL")
	}
	if strings.TrimSpace(cfg.BotBaseURL) == "" || strings.TrimSpace(cfg.BotInternalSecret) == "" {
		return fmt.Errorf("BOT_BASE_URL and BOT_INTERNAL_SECRET are required for Redis failure notifications")
	}
	return nil
}
