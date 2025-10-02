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
	go func(latency, reqCount int64) {
		ctx, cancel = context.WithCancel(context.Background())
		ticker := time.NewTicker(90 * time.Second)
		defer ticker.Stop()
		stopCount := 0

		for {
			select {
			case <-ticker.C:
				lat := atomic.LoadInt64(&latency)
				count := atomic.LoadInt64(&reqCount)

				log.Println(lat / count)
				stopCount++
				if stopCount == 2 {
					ctx.Done()
				}

			case <-ctx.Done():
				return
			}
		}
	}(latency, reqCount)

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
