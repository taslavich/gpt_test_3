package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	dbpkg "gitlab.com/twinbid-exchange/RTB-exchange/internal/db"
	kafkaService "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/kafka"
	redisService "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/redis"
)

type workController struct{ enabled atomic.Bool }

func newWorkController() *workController {
	w := &workController{}
	w.enabled.Store(true)
	return w
}
func (w *workController) Enabled() bool { return w.enabled.Load() }
func (w *workController) Set(v bool)    { w.enabled.Store(v) }

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	cfg, err := config.LoadConfig[config.KafkaRedisConfig](ctx)
	if err != nil {
		log.Fatal(err)
	}
	if err := validateConfig(cfg); err != nil {
		log.Fatal(err)
	}
	rdb, err := redisService.NewRedisClient(cfg.AdvRedisAddr, cfg.RedisPassword, cfg.RedisDBAdvRuntime, cfg.RedisPoolSize, cfg.RedisMinIdleConns)
	if err != nil {
		log.Fatal(err)
	}
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("failed to ping ADV runtime Redis DB%d at %s: %v", cfg.RedisDBAdvRuntime, cfg.AdvRedisAddr, err)
	}
	writers, err := kafkaService.CreateAdvKafkaWriters(cfg.KafkaConfig)
	if err != nil {
		log.Fatal(err)
	}
	defer writers.Close()
	readers, err := kafkaService.CreateAdvKafkaReaders(cfg.KafkaConfig)
	if err != nil {
		log.Fatal(err)
	}
	defer readers.Close()
	db, err := sql.Open("postgres", cfg.PostgresDSN)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("failed to ping Postgres: %v", err)
	}
	if err := dbpkg.MigrateUserGoalSpent(ctx, db); err != nil {
		log.Fatal(err)
	}
	work := newWorkController()
	bot := newBot(cfg)
	srvErr := startControlServer(ctx, cfg, work)
	go runExporter(ctx, cfg, rdb, writers, bot)
	runImporter(ctx, cfg, readers, db, work, bot)
	select {
	case err := <-srvErr:
		if err != nil {
			log.Printf("⚠️ kafkaredis HTTP server error: %v", err)
		}
	default:
	}
}

func validateConfig(cfg *config.KafkaRedisConfig) error {
	if strings.TrimSpace(cfg.AdvRedisAddr) == "" {
		return errors.New("ADV_REDIS_ADDR is required")
	}
	if strings.TrimSpace(cfg.PostgresDSN) == "" {
		return errors.New("POSTGRES_DSN is required")
	}
	if len(cfg.KafkaBrokers) == 0 {
		return errors.New("KAFKA_BROKERS is required")
	}
	if strings.TrimSpace(cfg.KafkaTopicSpentTotals) == "" {
		return errors.New("KAFKA_TOPIC_SPENT_TOTALS is required")
	}
	if strings.TrimSpace(cfg.KafkaGroupIDSpentTotals) == "" {
		return errors.New("KAFKA_GROUP_ID_SPENT_TOTALS is required")
	}
	if cfg.ExporterInterval <= 0 || cfg.FetchBackoff <= 0 || cfg.SelfControlTimeout <= 0 {
		return errors.New("kafkaredis durations must be positive")
	}
	if cfg.RedisDBAdvRuntime < 0 {
		return errors.New("REDIS_DB_ADV_RUNTIME must be nonnegative")
	}
	return nil
}

func newBot(cfg *config.KafkaRedisConfig) *utils.BotMessage {
	if strings.TrimSpace(cfg.BotBaseURL) == "" {
		return nil
	}
	return utils.NewBotMessage(cfg.BotBaseURL, cfg.BotInternalSecret)
}

func alert(ctx context.Context, bot *utils.BotMessage, text string) {
	log.Printf("⚠️ %s", text)
	if bot == nil {
		return
	}
	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := bot.SendTextMessageToBot(c, text); err != nil {
		log.Printf("⚠️ failed to send Telegram alert: %v", err)
	}
}

func startControlServer(ctx context.Context, cfg *config.KafkaRedisConfig, work *workController) <-chan error {
	mux := http.NewServeMux()
	mux.HandleFunc("/work_status/postgres_sync", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		switch r.URL.Query().Get("work") {
		case "true":
			work.Set(true)
			w.WriteHeader(http.StatusOK)
		case "false":
			work.Set(false)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	})
	srv := &http.Server{Addr: fmt.Sprintf("%s:%d", cfg.HTTPHost, cfg.HTTPPort), Handler: mux}
	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("⚠️ kafkaredis HTTP shutdown failed: %v", err)
		}
	}()
	return errCh
}

func runExporter(ctx context.Context, cfg *config.KafkaRedisConfig, rdb *redis.Client, writers *kafkaService.AdvKafkaWriters, bot *utils.BotMessage) {
	t := time.NewTicker(cfg.ExporterInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			err := kafkaService.ScanSpentTotalsWithCorruptHook(ctx, rdb, cfg.RedisScanCount, func(c context.Context, m kafkaService.SpentTotalMessage) error {
				pc, cancel := context.WithTimeout(c, 5*time.Second)
				defer cancel()
				return kafkaService.PublishSpentTotal(pc, writers.SpentTotals, m)
			}, func(key, raw string, err error) {
				alert(ctx, bot, fmt.Sprintf("spent_totals corrupt Redis value skipped key=%s value=%q error=%v", key, raw, err))
			})
			if err != nil {
				alert(ctx, bot, fmt.Sprintf("spent_totals export failed; Redis totals left unchanged; retrying next tick: %v", err))
			}
		}
	}
}

func runImporter(ctx context.Context, cfg *config.KafkaRedisConfig, readers *kafkaService.AdvKafkaReaders, db *sql.DB, work *workController, bot *utils.BotMessage) {
	for ctx.Err() == nil {
		if !work.Enabled() {
			sleepOrDone(ctx, cfg.FetchBackoff)
			continue
		}
		msg, err := readers.SpentTotals.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("⚠️ spent_totals fetch failed: %v", err)
				sleepOrDone(ctx, cfg.FetchBackoff)
			}
			continue
		}
		var st kafkaService.SpentTotalMessage
		if err := json.Unmarshal(msg.Value, &st); err != nil {
			work.Set(false)
			alert(ctx, bot, fmt.Sprintf("spent_totals invalid JSON; import stopped; no commit topic=%s partition=%d offset=%d stop_status=local work=false error=%v", msg.Topic, msg.Partition, msg.Offset, err))
			continue
		}
		if err := st.Validate(); err != nil {
			work.Set(false)
			alert(ctx, bot, fmt.Sprintf("spent_totals invalid total; import stopped; no commit topic=%s partition=%d offset=%d stop_status=local work=false error=%v", msg.Topic, msg.Partition, msg.Offset, err))
			continue
		}
		if err := kafkaService.ApplySpentTotal(ctx, db, st); err != nil {
			work.Set(false)
			stopStatus := callSelfStop(ctx, cfg)
			alert(ctx, bot, fmt.Sprintf("spent_totals Postgres apply failed; no commit; self-stop status=%s error=%v event=%s:%s", stopStatus, err, st.EntityType, st.EntityID))
			continue
		}
		if err := readers.SpentTotals.CommitMessages(ctx, msg); err != nil {
			alert(ctx, bot, fmt.Sprintf("spent_totals commit failed topic=%s partition=%d offset=%d: %v", msg.Topic, msg.Partition, msg.Offset, err))
			continue
		}
	}
}

func callSelfStop(ctx context.Context, cfg *config.KafkaRedisConfig) string {
	if strings.TrimSpace(cfg.SelfControlURL) == "" {
		return "skipped: KAFKAREDIS_SELF_CONTROL_URL empty"
	}
	c, cancel := context.WithTimeout(ctx, cfg.SelfControlTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(c, http.MethodPut, cfg.SelfControlURL+"?work=false", nil)
	if err != nil {
		return "request_error=" + err.Error()
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "network_error=" + err.Error()
	}
	defer resp.Body.Close()
	return fmt.Sprintf("http_%d", resp.StatusCode)
}

func sleepOrDone(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
