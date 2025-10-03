package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/filter"
	dspRouterGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/dspRouter"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	maxproc "gitlab.com/twinbid-exchange/RTB-exchange/internal/mp"
	dspRouterWeb "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/dspRouter/web"

	"google.golang.org/grpc"
)

func main() {
	if _, err := maxproc.Set(); err != nil {
		log.Printf("automaxprocs setup failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.LoadConfig[config.RouterConfig](ctx)
	if err != nil {
		log.Fatalf("Cannot load config: %v", err)
	}
	log.Println("Config initialized!")

	log.Println("Timeout", cfg.BidResponsesTimeout)

	/*redisClient := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", cfg.RedisHost, cfg.RedisPort),
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	defer redisClient.Close()

	if err := waitForRedis(ctx, redisClient, 10, 2*time.Second); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	log.Println("✅ Connected to Redis")*/

	ruleManager := filter.NewRuleManager()

	fileLoader := filter.NewFileRuleLoader(ruleManager, cfg.DspRulesConfigPath, cfg.SppRulesConfigPath)
	if err := waitForFile(ctx, cfg.DspRulesConfigPath, 10, time.Second); err != nil {
		log.Fatalf("DSP rules are not available: %v", err)
	}
	if err := waitForFile(ctx, cfg.SppRulesConfigPath, 10, time.Second); err != nil {
		log.Fatalf("SPP rules are not available: %v", err)
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

	name := "DSP1"
	var price float32 = 0.72
	BidId := fmt.Sprint(name, name)
	Nurl := "Nurl"
	Burl := "Burl"
	adid := "ADID"
	jsonData, err := json.Marshal(&ortb_V2_5.BidResponse{
		Id: &name,
		Seatbid: &ortb_V2_5.SeatBid{
			Bid: []*ortb_V2_5.Bid{
				{
					Id:    &BidId,
					Price: &price,
					Adid:  &adid,
					Nurl:  &Nurl,
					Burl:  &Burl,
				},
			},
		},
	})
	if err != nil {
		log.Fatalf("Can not marshal in GetBids_V2_5: %w", err)
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(jsonData)),
	}

	s := grpc.NewServer()
	dspRouterGrpc.RegisterDspRouterServiceServer(
		s,
		dspRouterWeb.NewServer(
			ruleManager,
			fileLoader,
			processor,
			cfg.DspRulesConfigPath,
			cfg.SppRulesConfigPath,
			cfg.DSPEndpoints_v_2_4,
			cfg.DSPEndpoints_v_2_5,
			nil,
			cfg.BidResponsesTimeout,
			cfg.MaxParallelRequests,
			cfg.Debug,
			resp,
		),
	)

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
			cfg.Host,
			cfg.Port,
		),
	)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	log.Printf("Server started on %s:%d", cfg.Host, cfg.Port)
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
