package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/filter"
	dspRouterGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/dspRouter"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	httpServer "gitlab.com/twinbid-exchange/RTB-exchange/internal/http"
	maxproc "gitlab.com/twinbid-exchange/RTB-exchange/internal/mp"
	services "gitlab.com/twinbid-exchange/RTB-exchange/internal/services"
	dspRouterWeb "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/dspRouter/web"
	redis_service "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/redis"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
)

func main() {
	cores := runtime.NumCPU()
	runtime.GOMAXPROCS(cores)
	/*if err := utils.OptimizeAll("router"); err != nil {
		log.Fatalf("OptimizeAll failed: %v", err)
	}*/

	if _, err := maxproc.Set(); err != nil {
		log.Fatalf("automaxprocs setup failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.LoadConfig[config.RouterConfig](ctx)
	if err != nil {
		log.Fatalf("Cannot load config: %v", err)
	}
	log.Println("Config initialized!")

	log.Println("Timeout", cfg.BidResponsesTimeout)

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

	if err := redis_service.PingClients(ctx, "router", redisClients.Ortb); err != nil {
		log.Fatalf("Failed to connect to Redis shards: %v", err)
	}
	log.Println("✅ Connected to Redis shards")

	ruleManager := filter.NewRuleManager()

	fileLoader := filter.NewFileRuleLoader(
		ruleManager,
		cfg.DspRulesConfigPathV25,
		cfg.SppRulesConfigPathV25,
	)

	if err := waitForFile(ctx, cfg.DspRulesConfigPathV25, 10, time.Second); err != nil {
		log.Fatalf("DSP V25 rules are not available: %v", err)
	}
	if err := waitForFile(ctx, cfg.SppRulesConfigPathV25, 10, time.Second); err != nil {
		log.Fatalf("SPP V25 rules are not available: %v", err)
	}

	if err := fileLoader.LoadDSPRules(); err != nil {
		log.Printf("Warning: Failed to load dsp filter rules: %v", err)
	} else {
		log.Println("Filter rules loaded successfully")
	}

	if err := fileLoader.LoadSPPRules(); err != nil {
		log.Printf("Warning: Failed to load spp filter rules: %v", err)
	} else {
		log.Println("Filter rules loaded successfully")
	}

	processor := filter.NewOptimizedFilterProcessor(ruleManager)

	sspGeoDspMapAdult, err := utils.InitSspGeoDspMap[bool](cfg.SspGeoDspLinksAdultFilePath)
	if err != nil {
		log.Fatalf("Failed to InitSspGeoPercentsLogic: %v", err)
	}

	sspGeoDspMapMainstream, err := utils.InitSspGeoDspMap[bool](cfg.SspGeoDspLinksMainstreamFilePath)
	if err != nil {
		log.Fatalf("Failed to InitSspGeoPercentsLogic: %v", err)
	}

	clients := dspRouterWeb.InitSspHttpClients(
		cfg.DSPEndpointsAdult_v_2_5,
		cfg.DSPEndpointsMainstream_v_2_5,
	)

	filtersAdl, err := filter.NewFiltersBox(cfg.DspFiltersAdlFilePath)
	if err != nil {
		log.Fatalf("Failed to NewFiltersBox: %v", err)
	}

	filtersMc, err := filter.NewFiltersBox(cfg.DspFiltersMcFilePath)
	if err != nil {
		log.Fatalf("Failed to NewFiltersBox: %v", err)
	}

	changersAdl, err := filter.NewChangersBoxChanger(cfg.DspChangersAdlFilePath)
	if err != nil {
		log.Fatalf("Failed to NewChangersBoxChanger: %v", err)
	}

	changersMc, err := filter.NewChangersBoxChanger(cfg.DspChangersMcFilePath)
	if err != nil {
		log.Fatalf("Failed to NewChangersBoxChanger: %v", err)
	}

	cidSspDspMapAdl, err := filter.InitCidSspDspMap(cfg.CidSspDspLinksAdultFilePath)
	if err != nil {
		log.Fatalf("Failed to InitCidSspDspMap: %v", err)
	}

	cidSspDspMapMc, err := filter.InitCidSspDspMap(cfg.CidSspDspLinksMainstreamFilePath)
	if err != nil {
		log.Fatalf("Failed to InitCidSspDspMap: %v", err)
	}

	redisWriteErrorMonitor := services.NewRedisWriteErrorMonitor("router", func(count uint64) {
		services.StopSspAdapterOrtbStreams(ctx, cfg.SspAdapterWorkStatusURL)
	})
	redisWriteErrorMonitor.Start()

	s := grpc.NewServer()
	routerServer := dspRouterWeb.NewServer(
		ruleManager,
		fileLoader,
		processor,
		cfg.DSPEndpointsAdult_v_2_5,
		cfg.DSPEndpointsMainstream_v_2_5,
		redisClients.Ortb,
		cfg.BidResponsesTimeout,
		&sspGeoDspMapAdult,
		&sspGeoDspMapMainstream,
		clients,
		filtersAdl,
		filtersMc,
		&cidSspDspMapAdl,
		&cidSspDspMapMc,
		changersAdl,
		changersMc,
		cfg.SspHttpClientTimeouts,
		redisWriteErrorMonitor,
	)

	if err := routerServer.LoadNetset(cfg.AllowedIpDbPath); err != nil {
		log.Fatalf("Failed to load netset: %v", err)
	}

	dspRouterGrpc.RegisterDspRouterServiceServer(
		s,
		routerServer,
	)

	router := httpServer.InitHttpRouter(chi.NewRouter())
	dspRouterWeb.InitHttpRoutes(
		router,
		cfg.SspGeoDspLinksAdultFilePath,
		cfg.SspGeoDspLinksMainstreamFilePath,
		&sspGeoDspMapAdult,
		&sspGeoDspMapMainstream,
		cfg.DspFiltersAdlFilePath,
		cfg.DspFiltersMcFilePath,
		filtersAdl,
		filtersMc,
		cfg.DspChangersAdlFilePath,
		cfg.DspChangersMcFilePath,
		changersAdl,
		changersMc,
		cfg.CidSspDspLinksAdultFilePath,
		cfg.CidSspDspLinksMainstreamFilePath,
		&cidSspDspMapAdl,
		&cidSspDspMapMc,
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
	if err := s.Serve(lis); err != nil {
		errChan <- err
		log.Printf("failed to serve: %v", err)
	}
}

func waitForRedis(ctx context.Context, client *redis.Client, attempts int, delay time.Duration) error {
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := client.Ping(ctx).Err(); err == nil {
			return nil
		} else {
			lastErr = err
			log.Printf("Redis is not ready (attempt %d/%d): %v", attempt, attempts, err)
		}

		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return fmt.Errorf("redis ping failed after %d attempts: %w", attempts, lastErr)
}

func waitForFile(ctx context.Context, path string, attempts int, delay time.Duration) error {
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if _, err := os.Stat(path); err == nil {
			return nil
		} else if os.IsNotExist(err) {
			lastErr = err
			log.Printf("File %s not found yet (attempt %d/%d)", path, attempt, attempts)
		} else {
			return err
		}

		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return fmt.Errorf("file %s not found after %d attempts: %w", path, attempts, lastErr)
}
