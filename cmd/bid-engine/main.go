package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	bidEngineGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/bidEngine"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	httpServer "gitlab.com/twinbid-exchange/RTB-exchange/internal/http"
	bidEngine "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/bidEngine/service"
	bidEngineWeb "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/bidEngine/web"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/types"

	"google.golang.org/grpc"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.LoadConfig[config.BiddingEngineConfig](ctx)
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

	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	log.Println("✅ Connected to Redis")

	sspGeoDspMapAdult, err := utils.InitSspGeoDspMap[*types.PercentAndBidfloor](cfg.SspGeoDspPercentsAdultFilePath)
	if err != nil {
		log.Fatalf("Failed to InitSspGeoPercentsLogic: %v", err)
	}

	sspGeoDspMapMainstream, err := utils.InitSspGeoDspMap[*types.PercentAndBidfloor](cfg.SspGeoDspPercentsMainstreamFilePath)
	if err != nil {
		log.Fatalf("Failed to InitSspGeoPercentsLogic: %v", err)
	}

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
	s := grpc.NewServer()
	bidEngineGrpc.RegisterBidEngineServiceServer(
		s,
		bidEngineWeb.NewServer(
			cfg.ProfitPercent,
			redisClient,
			cfg.RedisSetOrtb,
			bidEngine.GetWinnerBidInternal_V_2_5,
			cfg.SspGeoDspPercentsAdultFilePath,
			&sspGeoDspMapAdult,
			cfg.SspGeoDspPercentsMainstreamFilePath,
			&sspGeoDspMapMainstream,
			cfg.AdmDomain,
		),
	)

	router := httpServer.InitHttpRouter(chi.NewRouter())
	bidEngineWeb.InitHttpRoutes(
		router,
		cfg.SspGeoDspPercentsAdultFilePath,
		cfg.SspGeoDspPercentsMainstreamFilePath,
		&sspGeoDspMapAdult,
		&sspGeoDspMapMainstream,
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

	go httpServer.RunHttpServer(ctx, router, cfg.HttpServer.Host, cfg.HttpServer.Port)

	log.Printf("Server started on %s:%d", cfg.GrpcServer.Host, cfg.GrpcServer.Port)
	if err := s.Serve(lis); err != nil {
		errChan <- err
		log.Printf("failed to serve: %v", err)
	}
}
