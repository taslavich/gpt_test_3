package main

import (
	"context"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
	httpServer "gitlab.com/twinbid-exchange/RTB-exchange/internal/http"
	mockDspWeb "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/mock-dsp"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.LoadConfig[config.MockDspConfig](ctx)
	if err != nil {
		log.Fatalf("Cannot load config: %v", err)
	}
	log.Println("Config initialized!")

	log.Println(cfg.DspName)

	BidId := fmt.Sprint(cfg.DspName, cfg.DspName)
	Nurl := "Nurl"
	Burl := "Burl"

	var latency int64 = 0
	var reqCount int64 = 0

	// Запускаем горутину для сбора метрик
	go func(latencyPtr, reqCountPtr *int64) {
		metricsCtx, metricsCancel := context.WithCancel(context.Background())
		defer metricsCancel()

		ticker := time.NewTicker(90 * time.Second)
		defer ticker.Stop()
		stopCount := 0

		for {
			select {
			case <-ticker.C:
				lat := atomic.LoadInt64(latencyPtr)
				count := atomic.LoadInt64(reqCountPtr)

				// ЗАЩИТА ОТ ДЕЛЕНИЯ НА НОЛЬ
				if count > 0 {
					averageLatency := lat / count
					log.Printf("📊 Metrics Report - TotalLatency: %d, Requests: %d, Average Latency: %d ms",
						lat, count, averageLatency)
				} else {
					log.Printf("📊 Metrics Report - No requests processed yet")
				}

				stopCount++
				if stopCount == 2 {
					// После двух отчетов (180 секунд) завершаем
					log.Println("📊 Metrics collection completed after 180 seconds")
					metricsCancel()
					return
				}

			case <-metricsCtx.Done():
				log.Println("📊 Metrics goroutine stopped")
				return
			}
		}
	}(&latency, &reqCount)

	router := httpServer.InitHttpRouter()
	mockDspWeb.InitRoutes(
		ctx,
		router,
		&ortb_V2_5.BidResponse{
			Id: &cfg.DspName,
			Seatbid: &ortb_V2_5.SeatBid{
				Bid: []*ortb_V2_5.Bid{
					{
						Id:    &BidId,
						Price: &cfg.Price,
						Adid:  &cfg.Adid,
						Nurl:  &Nurl,
						Burl:  &Burl,
					},
				},
			},
		},
		&latency,
		&reqCount,
	)
	log.Println("HTTP routes initialized")

	httpServer.RunHttpServer(ctx, router, cfg.Host, cfg.Port)

}
