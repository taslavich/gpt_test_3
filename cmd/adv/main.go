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
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/filter"
	advGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/adv"
	httpServer "gitlab.com/twinbid-exchange/RTB-exchange/internal/http"
	auction "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/adv/service"
	advWeb "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/adv/web"
	"google.golang.org/grpc"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.LoadConfig[config.AdvConfig](ctx)
	if err != nil {
		log.Fatalf("Cannot load config: %v", err)
	}
	log.Println("Config initialized!")

	processor := filter.NewOptimizedFilterProcessor(filter.NewRuleManager())
	auctionService := auction.NewAuctionService(processor)

	s := grpc.NewServer()
	advGrpc.RegisterAdvServiceServer(
		s,
		advWeb.NewServer(auctionService),
	)

	router := httpServer.InitHttpRouter(chi.NewRouter())
	advWeb.InitHttpRoutes(router)
	log.Println("HTTP routes initialized")

	errChan := make(chan error)

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
	go func() {
		if err := s.Serve(lis); err != nil {
			errChan <- err
			log.Printf("failed to serve: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case <-stop:
		log.Println("Shutting down gracefully...")
		s.GracefulStop()
	case err := <-errChan:
		log.Fatalf("Server crashed: %v", err)
	}
}
