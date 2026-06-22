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
}

type KafkaWriters struct {
	Ortb        *kafka.Writer
	Impressions *kafka.Writer
	Clicks      *kafka.Writer
}

func kafkaTopics(cfg config.KafkaConfig) []string {
	return []string{
		cfg.KafkaTopicOrtb,
		cfg.KafkaTopicImpressions,
		cfg.KafkaTopicClicks,
	}
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

	ortbReader, err := InitKafkaReader(
		cfg,
		cfg.KafkaTopicOrtb,
		cfg.KafkaGroupIDOrtb,
	)
	if err != nil {
		return nil, err
	}

	impressionsReader, err := InitKafkaReader(
		cfg,
		cfg.KafkaTopicImpressions,
		cfg.KafkaGroupIDImpressions,
	)
	if err != nil {
		if closeErr := ortbReader.Close(); closeErr != nil {
			log.Printf("⚠️ failed to close ORTB Kafka reader after init error: %v", closeErr)
		}
		return nil, err
	}

	clicksReader, err := InitKafkaReader(
		cfg,
		cfg.KafkaTopicClicks,
		cfg.KafkaGroupIDClicks,
	)
	if err != nil {
		if closeErr := impressionsReader.Close(); closeErr != nil {
			log.Printf("⚠️ failed to close Impressions Kafka reader after init error: %v", closeErr)
		}
		if closeErr := ortbReader.Close(); closeErr != nil {
			log.Printf("⚠️ failed to close ORTB Kafka reader after init error: %v", closeErr)
		}
		return nil, err
	}

	return &KafkaReaders{
		Ortb:        ortbReader,
		Impressions: impressionsReader,
		Clicks:      clicksReader,
	}, nil
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

	ortbWriter, err := CreateKafkaWriter(cfg.KafkaBrokers, cfg.KafkaTopicOrtb)
	if err != nil {
		return nil, err
	}

	impressionsWriter, err := CreateKafkaWriter(cfg.KafkaBrokers, cfg.KafkaTopicImpressions)
	if err != nil {
		if closeErr := ortbWriter.Close(); closeErr != nil {
			log.Printf("⚠️ failed to close ORTB Kafka writer after init error: %v", closeErr)
		}

		return nil, err
	}

	clicksWriter, err := CreateKafkaWriter(cfg.KafkaBrokers, cfg.KafkaTopicClicks)
	if err != nil {
		if closeErr := ortbWriter.Close(); closeErr != nil {
			log.Printf("⚠️ failed to close ORTB Kafka writer after init error: %v", closeErr)
		}
		if closeErr := impressionsWriter.Close(); closeErr != nil {
			log.Printf("⚠️ failed to close Impressions Kafka writer after init error: %v", closeErr)
		}
		return nil, err
	}

	return &KafkaWriters{
		Ortb:        ortbWriter,
		Impressions: impressionsWriter,
		Clicks:      clicksWriter,
	}, nil
}
