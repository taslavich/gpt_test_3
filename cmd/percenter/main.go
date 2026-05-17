package main

/*import (
	"context"
	"crypto/tls"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/ClickHouse/clickhouse-go/v2"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/services/percenter"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.LoadConfig[config.PercenterConfig](ctx)
	if err != nil {
		log.Fatalf("Cannot load config: %v", err)
	}
	log.Println("Config initialized!")

	log.Println(cfg.Clickhouse.Username, cfg.Clickhouse.Password)

	addr := net.JoinHostPort(cfg.Clickhouse.Host, cfg.Clickhouse.Port)

	conn := clickhouse.OpenDB(&clickhouse.Options{
		Addr:     []string{addr},
		Protocol: clickhouse.Native,
		TLS:      &tls.Config{},
		Auth: clickhouse.Auth{
			Username: cfg.Clickhouse.Username,
			Password: cfg.Clickhouse.Password,
			Database: cfg.Clickhouse.Database,
		},
	})
	defer conn.Close()

	if err := conn.PingContext(ctx); err != nil {
		log.Fatalf("❌ ClickHouse ping failed: %v", err)
	}
	log.Println("✅ Connected to ClickHouse")

	adapter := percenter.NewPercenter(
		cfg.UriOfBidEngine,
	)
	client, cancelFunc := adapter.GetGrpClient()
	defer cancelFunc()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	for {
		select {
		case <-sigChan:
			log.Printf("Shutting down Percenter")
			return
		default:
			err := percenter.GotDynamicGeoPercentPerSsp(ctx, conn, client)
			if err != nil {
				log.Printf("❌ Processing error: %v", err)
				continue
			}
			log.Print("Did counting dynamic percents")
		}
	}
}
*/
