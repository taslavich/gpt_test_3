package kafka_service

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
)

const (
	DefaultTopicPartitions = 48
	DefaultRetentionHours  = 48
)

type KafkaReaders struct {
	Ortb        *kafka.Reader
	Impressions *kafka.Reader
	Clicks      *kafka.Reader
	Conversions *kafka.Reader
}

type KafkaWriters struct {
	Ortb        *kafka.Writer
	Impressions *kafka.Writer
	Clicks      *kafka.Writer
	Conversions *kafka.Writer
}

func (r *KafkaReaders) Close() error {
	if r == nil {
		return nil
	}
	items := []struct {
		name   string
		reader *kafka.Reader
	}{{"ORTB", r.Ortb}, {"Impressions", r.Impressions}, {"Clicks", r.Clicks}, {"Conversions", r.Conversions}}
	var lastErr error
	for _, item := range items {
		if item.reader == nil {
			continue
		}
		if err := item.reader.Close(); err != nil {
			lastErr = fmt.Errorf("failed to close %s Kafka reader: %w", item.name, err)
			log.Printf("⚠️ %v", lastErr)
		}
	}
	return lastErr
}

func (w *KafkaWriters) Close() error {
	if w == nil {
		return nil
	}
	items := []struct {
		name   string
		writer *kafka.Writer
	}{{"ORTB", w.Ortb}, {"Impressions", w.Impressions}, {"Clicks", w.Clicks}, {"Conversions", w.Conversions}}
	var lastErr error
	for _, item := range items {
		if item.writer == nil {
			continue
		}
		if err := item.writer.Close(); err != nil {
			lastErr = fmt.Errorf("failed to close %s Kafka writer: %w", item.name, err)
			log.Printf("⚠️ %v", lastErr)
		}
	}
	return lastErr
}

func kafkaTopics(cfg config.KafkaConfig) []string {
	return []string{cfg.KafkaTopicOrtb, cfg.KafkaTopicImpressions, cfg.KafkaTopicClicks, cfg.KafkaTopicConversions}
}

func checkKafkaBrokers(brokers []string) error {
	if len(brokers) == 0 {
		return fmt.Errorf("kafka brokers list is empty")
	}

	for _, broker := range brokers {
		conn, err := kafka.Dial("tcp", broker)
		if err != nil {
			return fmt.Errorf("failed to connect to Kafka broker %s: %w", broker, err)
		}

		_, err = conn.ApiVersions()
		closeErr := conn.Close()

		if err != nil {
			return fmt.Errorf("Kafka broker %s not responding: %w", broker, err)
		}

		if closeErr != nil {
			return fmt.Errorf("failed to close Kafka broker connection %s: %w", broker, closeErr)
		}

		log.Printf("✅ Connected to Kafka broker: %s", broker)
	}

	return nil
}

func ensureTopicExists(brokers []string, topic string, numPartitions int) error {
	if len(brokers) == 0 {
		return fmt.Errorf("kafka brokers list is empty")
	}

	if strings.TrimSpace(topic) == "" {
		return fmt.Errorf("kafka topic is empty")
	}

	if numPartitions <= 0 {
		return fmt.Errorf("numPartitions must be positive, got %d", numPartitions)
	}

	conn, err := kafka.Dial("tcp", brokers[0])
	if err != nil {
		return fmt.Errorf("failed to connect to Kafka broker %s: %w", brokers[0], err)
	}
	defer conn.Close()

	controller, err := conn.Controller()
	if err != nil {
		return fmt.Errorf("failed to get Kafka controller: %w", err)
	}

	controllerAddr := fmt.Sprintf("%s:%d", controller.Host, controller.Port)

	controllerConn, err := kafka.Dial("tcp", controllerAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to Kafka controller %s: %w", controllerAddr, err)
	}
	defer controllerConn.Close()

	partitions, err := controllerConn.ReadPartitions()
	if err != nil {
		return fmt.Errorf("failed to read Kafka partitions: %w", err)
	}

	for _, p := range partitions {
		if p.Topic == topic {
			log.Printf("✅ Kafka topic already exists: %s", topic)
			return nil
		}
	}

	configs := []kafka.ConfigEntry{
		{
			ConfigName:  "retention.ms",
			ConfigValue: fmt.Sprintf("%d", DefaultRetentionHours*60*60*1000),
		},
		{
			ConfigName:  "retention.bytes",
			ConfigValue: "-1",
		},
		{
			ConfigName:  "cleanup.policy",
			ConfigValue: "delete",
		},
		{
			ConfigName:  "segment.bytes",
			ConfigValue: fmt.Sprintf("%d", 100*1024*1024), // 100 MB
		},
	}

	topicConfig := kafka.TopicConfig{
		Topic:             topic,
		NumPartitions:     numPartitions,
		ReplicationFactor: 1,
		ConfigEntries:     configs,
	}

	if err := controllerConn.CreateTopics(topicConfig); err != nil {
		lowerErr := strings.ToLower(err.Error())
		if strings.Contains(lowerErr, "already exists") || strings.Contains(lowerErr, "topic_already_exists") {
			log.Printf("✅ Kafka topic already exists after create attempt: %s", topic)
			return nil
		}

		return fmt.Errorf("failed to create Kafka topic %s: %w", topic, err)
	}

	log.Printf("✅ Created Kafka topic: %s with %d partitions", topic, numPartitions)

	time.Sleep(2 * time.Second)

	return nil
}

func EnsureTopicsExist(cfg config.KafkaConfig) error {
	for _, topic := range kafkaTopics(cfg) {
		if err := ensureTopicExists(cfg.KafkaBrokers, topic, DefaultTopicPartitions); err != nil {
			return fmt.Errorf("failed to ensure Kafka topic %s exists: %w", topic, err)
		}
	}

	return nil
}

func InitKafkaReader(cfg config.KafkaConfig, topic string, groupID string) (*kafka.Reader, error) {
	log.Printf("🔌 Checking Kafka connection to: %v", cfg.KafkaBrokers)

	if err := checkKafkaBrokers(cfg.KafkaBrokers); err != nil {
		return nil, fmt.Errorf("Kafka connection failed: %w", err)
	}

	if err := ensureTopicExists(cfg.KafkaBrokers, topic, DefaultTopicPartitions); err != nil {
		return nil, fmt.Errorf("failed to ensure Kafka topic %s exists: %w", topic, err)
	}

	if strings.TrimSpace(groupID) == "" {
		return nil, fmt.Errorf("Kafka groupID is empty for topic %s", topic)
	}

	kafkaReader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: cfg.KafkaBrokers,
		Topic:   topic,
		GroupID: groupID,

		MinBytes: 1 << 20,
		MaxBytes: 128 << 20,
		MaxWait:  100 * time.Millisecond,

		QueueCapacity: 10000,

		// Если читаешь через FetchMessage + CommitMessages,
		// auto-commit через CommitInterval не нужен.
		CommitInterval: 0,

		ReadLagInterval: -1,
	})

	log.Printf("✅ Kafka reader initialized: topic=%s groupID=%s", topic, groupID)

	return kafkaReader, nil
}

func InitKafkaReaders(cfg config.KafkaConfig) (*KafkaReaders, error) {
	if err := EnsureTopicsExist(cfg); err != nil {
		return nil, fmt.Errorf("failed to ensure Kafka topics exist: %w", err)
	}

	type readerConfig struct {
		name    string
		topic   string
		groupID string
	}
	readerConfigs := []readerConfig{
		{name: "ORTB", topic: cfg.KafkaTopicOrtb, groupID: cfg.KafkaGroupIDOrtb},
		{name: "Impressions", topic: cfg.KafkaTopicImpressions, groupID: cfg.KafkaGroupIDImpressions},
		{name: "Clicks", topic: cfg.KafkaTopicClicks, groupID: cfg.KafkaGroupIDClicks},
		{name: "Conversions", topic: cfg.KafkaTopicConversions, groupID: cfg.KafkaGroupIDConversions},
	}
	readers := make([]*kafka.Reader, 0, len(readerConfigs))
	closeOnError := func() {
		for i := len(readers) - 1; i >= 0; i-- {
			if closeErr := readers[i].Close(); closeErr != nil {
				log.Printf("⚠️ failed to close %s Kafka reader after init error: %v", readerConfigs[i].name, closeErr)
			}
		}
	}
	for _, readerCfg := range readerConfigs {
		reader, err := InitKafkaReader(cfg, readerCfg.topic, readerCfg.groupID)
		if err != nil {
			closeOnError()
			return nil, err
		}
		readers = append(readers, reader)
	}
	return &KafkaReaders{Ortb: readers[0], Impressions: readers[1], Clicks: readers[2], Conversions: readers[3]}, nil
}

func checkKafkaTopic(brokers []string, topic string) error {
	if len(brokers) == 0 {
		return fmt.Errorf("kafka brokers list is empty")
	}

	conn, err := kafka.Dial("tcp", brokers[0])
	if err != nil {
		return err
	}
	defer conn.Close()

	partitions, err := conn.ReadPartitions()
	if err != nil {
		return err
	}

	for _, p := range partitions {
		if p.Topic == topic {
			log.Printf("✅ Kafka topic found: %s", topic)
			return nil
		}
	}

	return fmt.Errorf("topic %s not found", topic)
}

func checkKafkaWriterConnection(brokers []string, topic string) error {
	if err := checkKafkaBrokers(brokers); err != nil {
		return err
	}

	if err := checkKafkaTopic(brokers, topic); err != nil {
		return err
	}

	log.Printf("✅ Kafka writer connection OK: brokers=%v topic=%s", brokers, topic)

	return nil
}

func CreateKafkaWriter(brokers []string, topic string) (*kafka.Writer, error) {
	log.Printf("🔌 Checking Kafka writer connection: brokers=%v topic=%s", brokers, topic)

	if err := ensureTopicExists(brokers, topic, DefaultTopicPartitions); err != nil {
		return nil, fmt.Errorf("failed to ensure Kafka topic %s exists: %w", topic, err)
	}

	if err := checkKafkaWriterConnection(brokers, topic); err != nil {
		return nil, fmt.Errorf("Kafka writer initialization failed for topic %s: %w", topic, err)
	}

	writer := &kafka.Writer{
		Addr:     kafka.TCP(brokers...),
		Topic:    topic,
		Balancer: &kafka.LeastBytes{},

		Async: false,

		RequiredAcks: kafka.RequireOne,

		MaxAttempts:  3,
		WriteTimeout: 5 * time.Second,
	}

	log.Printf("✅ Kafka writer initialized: topic=%s", topic)

	return writer, nil
}

func CreateKafkaWriters(cfg config.KafkaConfig) (*KafkaWriters, error) {
	if err := EnsureTopicsExist(cfg); err != nil {
		return nil, fmt.Errorf("failed to ensure Kafka topics exist: %w", err)
	}

	type writerConfig struct {
		name  string
		topic string
	}
	writerConfigs := []writerConfig{
		{name: "ORTB", topic: cfg.KafkaTopicOrtb},
		{name: "Impressions", topic: cfg.KafkaTopicImpressions},
		{name: "Clicks", topic: cfg.KafkaTopicClicks},
		{name: "Conversions", topic: cfg.KafkaTopicConversions},
	}
	writers := make([]*kafka.Writer, 0, len(writerConfigs))
	closeOnError := func() {
		for i := len(writers) - 1; i >= 0; i-- {
			if closeErr := writers[i].Close(); closeErr != nil {
				log.Printf("⚠️ failed to close %s Kafka writer after init error: %v", writerConfigs[i].name, closeErr)
			}
		}
	}
	for _, writerCfg := range writerConfigs {
		writer, err := CreateKafkaWriter(cfg.KafkaBrokers, writerCfg.topic)
		if err != nil {
			closeOnError()
			return nil, err
		}
		writers = append(writers, writer)
	}
	return &KafkaWriters{Ortb: writers[0], Impressions: writers[1], Clicks: writers[2], Conversions: writers[3]}, nil
}
