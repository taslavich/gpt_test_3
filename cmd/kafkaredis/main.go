package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
	utils "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/utils_grpc"
	auction "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/adv/service"
	kafkaService "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/kafka"
	redisService "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/redis"
)

type importController struct{ enabled atomic.Bool }

func newImportController() *importController {
	controller := &importController{}
	controller.enabled.Store(true)
	return controller
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg, err := config.LoadConfig[config.KafkaredisConfig](ctx)
	if err != nil {
		log.Fatalf("cannot load kafkaredis config: %v", err)
	}
	if err := validateConfig(cfg); err != nil {
		log.Fatalf("invalid kafkaredis config: %v", err)
	}

	redisAddr := strings.TrimSpace(cfg.RedisUUIDAddr)
	if redisAddr == "" && len(cfg.RedisShardAddrs) > 0 {
		redisAddr = strings.TrimSpace(cfg.RedisShardAddrs[0])
	}
	runtimeRedis, err := redisService.NewRedisClient(redisAddr, cfg.RedisPassword, cfg.RedisDBAdvRuntime, cfg.RedisPoolSize, cfg.RedisMinIdleConns)
	if err != nil {
		log.Fatalf("cannot initialize kafkaredis Redis: %v", err)
	}
	defer runtimeRedis.Close()
	if err := runtimeRedis.Ping(ctx).Err(); err != nil {
		log.Fatalf("kafkaredis Redis unavailable: %v", err)
	}
	if err := redisService.ValidateAOF(ctx, runtimeRedis); err != nil {
		log.Fatalf("kafkaredis Redis persistence is unsafe: %v", err)
	}

	if strings.TrimSpace(cfg.PostgresDSN) == "" {
		log.Fatal("POSTGRES_DSN is required")
	}
	db, err := sql.Open("postgres", cfg.PostgresDSN)
	if err != nil {
		log.Fatalf("cannot open PostgreSQL: %v", err)
	}
	db.SetMaxOpenConns(cfg.PostgresMaxOpenConns)
	db.SetMaxIdleConns(cfg.PostgresMaxIdleConns)
	db.SetConnMaxLifetime(cfg.PostgresConnMaxLifetime)
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("PostgreSQL unavailable: %v", err)
	}
	if err := auction.MigrateADVSchema(ctx, db); err != nil {
		log.Fatalf("ADV schema migration failed: %v", err)
	}

	writer, err := kafkaService.CreateKafkaWriter(cfg.KafkaBrokers, cfg.KafkaTopicSpentTotals)
	if err != nil {
		log.Fatalf("cannot initialize spent_totals writer: %v", err)
	}
	writer.Balancer = &kafka.Hash{}
	writer.RequiredAcks = kafka.RequireAll
	writer.BatchSize = cfg.KafkaExportBatchSize
	writer.BatchBytes = int64(cfg.KafkaExportBatchBytes)
	defer writer.Close()
	reader, err := kafkaService.InitKafkaReader(cfg.KafkaConfig, cfg.KafkaTopicSpentTotals, cfg.KafkaGroupIDSpentTotals)
	if err != nil {
		log.Fatalf("cannot initialize spent_totals reader: %v", err)
	}
	defer reader.Close()

	notifier := utils.NewBotMessage(cfg.BotBaseURL, cfg.BotInternalSecret)
	controller := newImportController()
	server, err := startControlServer(cfg, controller)
	if err != nil {
		log.Fatalf("cannot start kafkaredis control server: %v", err)
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("kafkaredis control server shutdown failed: %v", err)
		}
	}()

	go runExporter(ctx, runtimeRedis, writer, cfg, notifier)
	go runImporter(ctx, reader, db, controller, cfg, notifier)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	cancel()
}

func startControlServer(cfg *config.KafkaredisConfig, controller *importController) (*http.Server, error) {
	if cfg == nil || controller == nil {
		return nil, fmt.Errorf("kafkaredis control server config or controller is nil")
	}
	router := chi.NewRouter()
	router.Put("/work_status/postgres_sync", func(w http.ResponseWriter, r *http.Request) {
		value, err := strconv.ParseBool(strings.TrimSpace(r.URL.Query().Get("work")))
		if err != nil {
			http.Error(w, "work must be true or false", http.StatusBadRequest)
			return
		}
		controller.enabled.Store(value)
		w.WriteHeader(http.StatusOK)
	})
	router.Get("/work_status/postgres_sync", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"work":%t}`, controller.enabled.Load())
	})
	address := net.JoinHostPort(cfg.HttpServer.Host, strconv.Itoa(int(cfg.HttpServer.Port)))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", address, err)
	}
	server := &http.Server{Addr: address, Handler: router, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("kafkaredis control server failed: %v", err)
		}
	}()
	return server, nil
}

func runExporter(ctx context.Context, client *redis.Client, writer *kafka.Writer, cfg *config.KafkaredisConfig, notifier *utils.BotMessage) {
	interval := cfg.RedisExportInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	exportCfg := kafkaService.SpentTotalsExportConfig{
		ScanCount:  int64(cfg.RedisScanCount),
		BatchSize:  cfg.KafkaExportBatchSize,
		BatchBytes: cfg.KafkaExportBatchBytes,
	}
	export := func() {
		if err := kafkaService.ExportSpentTotalsWithConfig(ctx, client, writer, exportCfg); err != nil {
			message := fmt.Sprintf("kafkaredis Redis->Kafka spent_totals export failed; Redis totals retained and the next tick will retry current absolute totals: %v", err)
			log.Print(message)
			if notifyErr := notifier.SendTextMessageToBot(ctx, message); notifyErr != nil {
				log.Printf("Telegram notification failed: %v", notifyErr)
			}
		}
	}
	export()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			export()
		}
	}
}

func runImporter(ctx context.Context, reader *kafka.Reader, db *sql.DB, controller *importController, cfg *config.KafkaredisConfig, notifier *utils.BotMessage) {
	disabledPoll := cfg.ImportDisabledPollInterval
	if disabledPoll <= 0 {
		disabledPoll = 250 * time.Millisecond
	}
	batchSize := cfg.KafkaImportBatchSize
	if batchSize <= 0 {
		batchSize = 2000
	}
	batchTimeout := cfg.KafkaImportBatchTimeout
	if batchTimeout <= 0 {
		batchTimeout = 100 * time.Millisecond
	}

	// Keep fetched but uncommitted messages in memory while the importer is
	// stopped. Re-enabling retries exactly this batch. A process restart also
	// replays it because Kafka offsets were not committed.
	var pending []kafka.Message
	for {
		if ctx.Err() != nil {
			return
		}
		if !controller.enabled.Load() {
			select {
			case <-ctx.Done():
				return
			case <-time.After(disabledPoll):
				continue
			}
		}

		if len(pending) == 0 {
			messages, err := fetchSpentTotalsBatch(ctx, reader, batchSize, batchTimeout)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				stopImporter(context.Background(), controller, cfg, notifier, nil, fmt.Errorf("fetch spent_totals batch: %w", err))
				continue
			}
			if len(messages) == 0 {
				continue
			}
			pending = messages
		}

		events := make([]kafkaService.SpentTotal, 0, len(pending))
		var processErr error
		for i, message := range pending {
			event, err := kafkaService.DecodeSpentTotal(message)
			if err != nil {
				processErr = fmt.Errorf("decode spent_totals batch message=%d partition=%d offset=%d: %w", i, message.Partition, message.Offset, err)
				break
			}
			events = append(events, event)
		}
		if processErr == nil {
			processErr = kafkaService.ApplySpentTotalsBatch(ctx, db, events)
		}
		if processErr == nil {
			commitMessages := compactCommitMessages(pending)
			processErr = reader.CommitMessages(ctx, commitMessages...)
			if processErr != nil {
				processErr = fmt.Errorf("commit spent_totals batch offsets: %w", processErr)
			}
		}
		if processErr != nil {
			stopImporter(context.Background(), controller, cfg, notifier, pending, processErr)
			continue
		}

		log.Printf("kafkaredis PostgreSQL import batch committed: messages=%d offsets=%d", len(pending), len(compactCommitMessages(pending)))
		pending = nil
		// No sleep after success: consume the next batch immediately.
	}
}

func fetchSpentTotalsBatch(ctx context.Context, reader *kafka.Reader, batchSize int, timeout time.Duration) ([]kafka.Message, error) {
	if reader == nil {
		return nil, errors.New("spent_totals reader is nil")
	}
	if batchSize <= 0 {
		return nil, errors.New("spent_totals batch size must be positive")
	}
	if timeout <= 0 {
		timeout = 100 * time.Millisecond
	}

	readCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	messages := make([]kafka.Message, 0, batchSize)
	for len(messages) < batchSize {
		message, err := reader.FetchMessage(readCtx)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				return messages, nil
			}
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, nil
}

func compactCommitMessages(messages []kafka.Message) []kafka.Message {
	latestByPartition := make(map[int]kafka.Message, len(messages))
	for _, message := range messages {
		previous, exists := latestByPartition[message.Partition]
		if !exists || message.Offset > previous.Offset {
			latestByPartition[message.Partition] = message
		}
	}
	result := make([]kafka.Message, 0, len(latestByPartition))
	for _, message := range latestByPartition {
		result = append(result, message)
	}
	return result
}

func stopImporter(ctx context.Context, controller *importController, cfg *config.KafkaredisConfig, notifier *utils.BotMessage, messages []kafka.Message, cause error) {
	statusCode := callSelfControl(ctx, cfg, false)
	if statusCode != http.StatusOK {
		controller.enabled.Store(false)
	}
	topic, partition, firstOffset, lastOffset := cfg.KafkaTopicSpentTotals, -1, int64(-1), int64(-1)
	if len(messages) > 0 {
		topic = messages[0].Topic
		partition = messages[0].Partition
		firstOffset = messages[0].Offset
		lastOffset = messages[len(messages)-1].Offset
	}
	text := fmt.Sprintf("kafkaredis PostgreSQL import stopped with uncommitted batch retained: error=%v topic=%s first_partition=%d first_offset=%d last_offset=%d batch_messages=%d stop_status=%d", cause, topic, partition, firstOffset, lastOffset, len(messages), statusCode)
	log.Print(text)
	if err := notifier.SendTextMessageToBot(ctx, text); err != nil {
		log.Printf("Telegram notification failed: %v", err)
	}
}

func callSelfControl(ctx context.Context, cfg *config.KafkaredisConfig, work bool) int {
	endpoint := strings.TrimSpace(cfg.SelfControlURL)
	if endpoint == "" {
		endpoint = fmt.Sprintf("http://127.0.0.1:%d/work_status/postgres_sync", cfg.HttpServer.Port)
	}
	separator := "?"
	if strings.Contains(endpoint, "?") {
		separator = "&"
	}
	requestCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPut, endpoint+separator+"work="+strconv.FormatBool(work), nil)
	if err != nil {
		return http.StatusServiceUnavailable
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return http.StatusServiceUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return http.StatusServiceUnavailable
	}
	return http.StatusOK
}

func validateConfig(cfg *config.KafkaredisConfig) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	if cfg.RedisDBAdvRuntime != 5 {
		return fmt.Errorf("kafkaredis requires REDIS_DB_ADV_RUNTIME=5")
	}
	if strings.TrimSpace(cfg.RedisUUIDAddr) == "" && len(cfg.RedisShardAddrs) == 0 {
		return fmt.Errorf("REDIS_UUID_ADDR or REDIS_SHARD_ADDRS is required")
	}
	if strings.TrimSpace(cfg.PostgresDSN) == "" {
		return fmt.Errorf("POSTGRES_DSN is required")
	}
	if len(cfg.KafkaBrokers) == 0 || strings.TrimSpace(cfg.KafkaTopicSpentTotals) == "" || strings.TrimSpace(cfg.KafkaGroupIDSpentTotals) == "" {
		return fmt.Errorf("Kafka brokers, spent_totals topic and group ID are required")
	}
	if cfg.RedisExportInterval <= 0 || cfg.KafkaImportBatchTimeout <= 0 || cfg.ImportDisabledPollInterval <= 0 || cfg.PostgresConnMaxLifetime <= 0 {
		return fmt.Errorf("kafkaredis intervals and timeouts must be positive")
	}
	if cfg.RedisScanCount <= 0 || cfg.KafkaExportBatchSize <= 0 || cfg.KafkaExportBatchBytes <= 0 || cfg.KafkaImportBatchSize <= 0 {
		return fmt.Errorf("kafkaredis scan and batch limits must be positive")
	}
	if cfg.PostgresMaxOpenConns <= 0 || cfg.PostgresMaxIdleConns < 0 || cfg.PostgresMaxIdleConns > cfg.PostgresMaxOpenConns {
		return fmt.Errorf("invalid PostgreSQL connection pool settings")
	}
	if cfg.HttpServer.Port == 0 {
		return fmt.Errorf("HTTP_PORT is required")
	}
	if strings.TrimSpace(cfg.BotBaseURL) == "" || strings.TrimSpace(cfg.BotInternalSecret) == "" {
		return fmt.Errorf("BOT_BASE_URL and BOT_INTERNAL_SECRET are required")
	}
	return nil
}
