package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	orchestratorGrpc "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/services/orchestrator"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	antiControl "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/antiperekrut"
	orchestrator "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/orchestrator/service"
	orchestratorWeb "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/orchestrator/web"

	"google.golang.org/grpc"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.LoadConfig[config.OrchestratorConfig](ctx)
	if err != nil {
		log.Fatalf("Cannot load config: %v", err)
	}
	log.Println("Config initialized!")

	o := orchestrator.NewOrchestrator(cfg.UriOfBidEngine, cfg.UriOfDspRouter)

	clients, cancelFunc := o.GetGrpClients()

	defer cancelFunc()

	s := grpc.NewServer()
	orchestratorGrpc.RegisterOrchestratorServiceServer(
		s,
		orchestratorWeb.NewServer(
			clients.BidEngineGrpcClient,
			clients.DspRouterGrpcClient,
			cfg.GetBidsTimeout,
			cfg.AuctionTimeout,
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
			cfg.GrpcServer.Host,
			cfg.GrpcServer.Port,
		),
	)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	if cfg.AntiperekrutEnabled {
		if len(cfg.AdvServiceControlURLs) == 0 {
			log.Fatal("antiperekrut startup reset requires ADV_SERVICE_CONTROL_URLS")
		}
		if strings.TrimSpace(cfg.BotBaseURL) == "" || strings.TrimSpace(cfg.BotInternalSecret) == "" {
			log.Fatal("antiperekrut startup reset requires BOT_BASE_URL and BOT_INTERNAL_SECRET")
		}
		startupHost, _ := os.Hostname()
		startupNotifier := utils.NewBotMessageWithTimeout(cfg.BotBaseURL, cfg.BotInternalSecret, cfg.AntiperekrutControlTimeout)
		startupEvent := antiControl.NewStartupEvent("orchestrator", startupHost)
		if err := antiControl.FanoutStartupEvent(ctx, antiControl.ClientConfig{
			Enabled:        true,
			URLs:           []string(cfg.AdvServiceControlURLs),
			RequestTimeout: cfg.AntiperekrutControlTimeout,
			RetryInitial:   cfg.AntiperekrutRetryInitial,
			RetryMax:       cfg.AntiperekrutRetryMax,
		}, startupEvent, startupNotifier.SendTextMessageToBot); err != nil {
			_ = startupNotifier.SendTextMessageToBot(ctx, fmt.Sprintf("[orchestrator][ANTIPEREKRUT_STARTUP_ERROR] %v", err))
			log.Fatalf("cannot deliver antiperekrut startup event: %v", err)
		}
	} else {
		log.Print("antiperekrut startup reset is disabled by ANTIPEREKRUT_ENABLED=false")
	}

	log.Printf("Server started on %s:%d", cfg.GrpcServer.Host, cfg.GrpcServer.Port)
	if err := s.Serve(lis); err != nil {
		errChan <- err
		log.Printf("failed to serve: %v", err)
	}
}
