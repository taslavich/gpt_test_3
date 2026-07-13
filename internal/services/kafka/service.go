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

type KafkaReaders struct{ Ortb, Impressions, Clicks, Conversions *kafka.Reader }
type KafkaWriters struct{ Ortb, Impressions, Clicks, Conversions *kafka.Writer }
type AdvKafkaWriters struct{ SpentTotals *kafka.Writer }
type AdvKafkaReaders struct{ SpentTotals *kafka.Reader }

func (w *AdvKafkaWriters) Close() error {
	if w == nil || w.SpentTotals == nil {
		return nil
	}
	return w.SpentTotals.Close()
}
func (r *AdvKafkaReaders) Close() error {
	if r == nil || r.SpentTotals == nil {
		return nil
	}
	return r.SpentTotals.Close()
}
func (r *KafkaReaders) Close() error {
	return closeReaders(map[string]*kafka.Reader{"ORTB": r.Ortb, "Impressions": r.Impressions, "Clicks": r.Clicks, "Conversions": r.Conversions})
}
func (w *KafkaWriters) Close() error {
	return closeWriters(map[string]*kafka.Writer{"ORTB": w.Ortb, "Impressions": w.Impressions, "Clicks": w.Clicks, "Conversions": w.Conversions})
}

func closeReaders(readers map[string]*kafka.Reader) error {
	var last error
	for n, r := range readers {
		if r != nil {
			if err := r.Close(); err != nil {
				last = fmt.Errorf("failed to close %s Kafka reader: %w", n, err)
				log.Printf("⚠️ %v", last)
			}
		}
	}
	return last
}
func closeWriters(writers map[string]*kafka.Writer) error {
	var last error
	for n, w := range writers {
		if w != nil {
			if err := w.Close(); err != nil {
				last = fmt.Errorf("failed to close %s Kafka writer: %w", n, err)
				log.Printf("⚠️ %v", last)
			}
		}
	}
	return last
}

func kafkaTopics(cfg config.KafkaConfig) []string {
	return []string{cfg.KafkaTopicOrtb, cfg.KafkaTopicImpressions, cfg.KafkaTopicClicks, cfg.KafkaTopicConversions, cfg.KafkaTopicSpentTotals}
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
	return ensureTopicExistsWithReplication(brokers, topic, numPartitions, 1)
}
func ensureTopicExistsWithReplication(brokers []string, topic string, numPartitions, replicationFactor int) error {
	if len(brokers) == 0 {
		return fmt.Errorf("kafka brokers list is empty")
	}
	if strings.TrimSpace(topic) == "" {
		return fmt.Errorf("kafka topic is empty")
	}
	if numPartitions <= 0 {
		return fmt.Errorf("numPartitions must be positive, got %d", numPartitions)
	}
	if replicationFactor <= 0 {
		return fmt.Errorf("replicationFactor must be positive, got %d", replicationFactor)
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
	controllerConn, err := kafka.Dial("tcp", fmt.Sprintf("%s:%d", controller.Host, controller.Port))
	if err != nil {
		return fmt.Errorf("failed to connect to Kafka controller: %w", err)
	}
	defer controllerConn.Close()
	parts, err := controllerConn.ReadPartitions()
	if err != nil {
		return fmt.Errorf("failed to read Kafka partitions: %w", err)
	}
	for _, p := range parts {
		if p.Topic == topic {
			log.Printf("✅ Kafka topic already exists: %s", topic)
			return nil
		}
	}
	cfg := kafka.TopicConfig{Topic: topic, NumPartitions: numPartitions, ReplicationFactor: replicationFactor, ConfigEntries: []kafka.ConfigEntry{{ConfigName: "retention.ms", ConfigValue: fmt.Sprintf("%d", DefaultRetentionHours*60*60*1000)}}}
	if err := controllerConn.CreateTopics(cfg); err != nil {
		lower := strings.ToLower(err.Error())
		if strings.Contains(lower, "already exists") || strings.Contains(lower, "topic_already_exists") {
			log.Printf("✅ Kafka topic already exists after create attempt: %s", topic)
			return nil
		}
		return fmt.Errorf("failed to create Kafka topic %s: %w", topic, err)
	}
	time.Sleep(2 * time.Second)
	return nil
}
func EnsureTopicsExist(cfg config.KafkaConfig) error {
	for _, t := range kafkaTopics(cfg) {
		partitions := DefaultTopicPartitions
		repl := 1
		if t == cfg.KafkaTopicSpentTotals {
			partitions = cfg.KafkaSpentTotalsPartitions
			repl = cfg.KafkaSpentTotalsReplicationFactor
		}
		if err := ensureTopicExistsWithReplication(cfg.KafkaBrokers, t, partitions, repl); err != nil {
			return fmt.Errorf("failed to ensure Kafka topic %s exists: %w", t, err)
		}
	}
	return nil
}

func InitKafkaReader(cfg config.KafkaConfig, topic, groupID string) (*kafka.Reader, error) {
	if err := checkKafkaBrokers(cfg.KafkaBrokers); err != nil {
		return nil, fmt.Errorf("Kafka connection failed: %w", err)
	}
	partitions := DefaultTopicPartitions
	repl := 1
	if topic == cfg.KafkaTopicSpentTotals {
		partitions = cfg.KafkaSpentTotalsPartitions
		repl = cfg.KafkaSpentTotalsReplicationFactor
	}
	if err := ensureTopicExistsWithReplication(cfg.KafkaBrokers, topic, partitions, repl); err != nil {
		return nil, err
	}
	if strings.TrimSpace(groupID) == "" {
		return nil, fmt.Errorf("Kafka groupID is empty for topic %s", topic)
	}
	return kafka.NewReader(kafka.ReaderConfig{Brokers: cfg.KafkaBrokers, Topic: topic, GroupID: groupID, MinBytes: 1, MaxBytes: 128 << 20, MaxWait: 100 * time.Millisecond, CommitInterval: 0, ReadLagInterval: -1}), nil
}
func InitKafkaReaders(cfg config.KafkaConfig) (*KafkaReaders, error) {
	if err := EnsureTopicsExist(cfg); err != nil {
		return nil, err
	}
	r1, e := InitKafkaReader(cfg, cfg.KafkaTopicOrtb, cfg.KafkaGroupIDOrtb)
	if e != nil {
		return nil, e
	}
	r2, e := InitKafkaReader(cfg, cfg.KafkaTopicImpressions, cfg.KafkaGroupIDImpressions)
	if e != nil {
		r1.Close()
		return nil, e
	}
	r3, e := InitKafkaReader(cfg, cfg.KafkaTopicClicks, cfg.KafkaGroupIDClicks)
	if e != nil {
		closeReaders(map[string]*kafka.Reader{"1": r1, "2": r2})
		return nil, e
	}
	r4, e := InitKafkaReader(cfg, cfg.KafkaTopicConversions, cfg.KafkaGroupIDConversions)
	if e != nil {
		closeReaders(map[string]*kafka.Reader{"1": r1, "2": r2, "3": r3})
		return nil, e
	}
	return &KafkaReaders{r1, r2, r3, r4}, nil
}

func checkKafkaTopic(brokers []string, topic string) error {
	conn, err := kafka.Dial("tcp", brokers[0])
	if err != nil {
		return err
	}
	defer conn.Close()
	parts, err := conn.ReadPartitions()
	if err != nil {
		return err
	}
	for _, p := range parts {
		if p.Topic == topic {
			return nil
		}
	}
	return fmt.Errorf("topic %s not found", topic)
}
func checkKafkaWriterConnection(brokers []string, topic string) error {
	if err := checkKafkaBrokers(brokers); err != nil {
		return err
	}
	return checkKafkaTopic(brokers, topic)
}
func CreateKafkaWriter(brokers []string, topic string) (*kafka.Writer, error) {
	return CreateKafkaWriterWithPartitions(brokers, topic, DefaultTopicPartitions, 1)
}
func newKafkaWriter(brokers []string, topic string, balancer kafka.Balancer) *kafka.Writer {
	return &kafka.Writer{Addr: kafka.TCP(brokers...), Topic: topic, Balancer: balancer, Async: false, RequiredAcks: kafka.RequireOne, MaxAttempts: 3, WriteTimeout: 5 * time.Second}
}

func spentTotalsBalancer() kafka.Balancer { return &kafka.Hash{} }

func CreateKafkaWriterWithPartitions(brokers []string, topic string, partitions, replicationFactor int) (*kafka.Writer, error) {
	if err := ensureTopicExistsWithReplication(brokers, topic, partitions, replicationFactor); err != nil {
		return nil, err
	}
	if err := checkKafkaWriterConnection(brokers, topic); err != nil {
		return nil, err
	}
	return newKafkaWriter(brokers, topic, &kafka.LeastBytes{}), nil
}

func CreateSpentTotalsWriter(cfg config.KafkaConfig) (*kafka.Writer, error) {
	if err := ensureTopicExistsWithReplication(cfg.KafkaBrokers, cfg.KafkaTopicSpentTotals, cfg.KafkaSpentTotalsPartitions, cfg.KafkaSpentTotalsReplicationFactor); err != nil {
		return nil, err
	}
	if err := checkKafkaWriterConnection(cfg.KafkaBrokers, cfg.KafkaTopicSpentTotals); err != nil {
		return nil, err
	}
	return newKafkaWriter(cfg.KafkaBrokers, cfg.KafkaTopicSpentTotals, spentTotalsBalancer()), nil
}
func CreateKafkaWriters(cfg config.KafkaConfig) (*KafkaWriters, error) {
	if err := EnsureTopicsExist(cfg); err != nil {
		return nil, err
	}
	w1, e := CreateKafkaWriter(cfg.KafkaBrokers, cfg.KafkaTopicOrtb)
	if e != nil {
		return nil, e
	}
	w2, e := CreateKafkaWriter(cfg.KafkaBrokers, cfg.KafkaTopicImpressions)
	if e != nil {
		w1.Close()
		return nil, e
	}
	w3, e := CreateKafkaWriter(cfg.KafkaBrokers, cfg.KafkaTopicClicks)
	if e != nil {
		closeWriters(map[string]*kafka.Writer{"1": w1, "2": w2})
		return nil, e
	}
	w4, e := CreateKafkaWriter(cfg.KafkaBrokers, cfg.KafkaTopicConversions)
	if e != nil {
		closeWriters(map[string]*kafka.Writer{"1": w1, "2": w2, "3": w3})
		return nil, e
	}
	return &KafkaWriters{w1, w2, w3, w4}, nil
}
func CreateAdvKafkaWriters(cfg config.KafkaConfig) (*AdvKafkaWriters, error) {
	w, err := CreateSpentTotalsWriter(cfg)
	if err != nil {
		return nil, err
	}
	return &AdvKafkaWriters{w}, nil
}
func CreateAdvKafkaReaders(cfg config.KafkaConfig) (*AdvKafkaReaders, error) {
	r, err := InitKafkaReader(cfg, cfg.KafkaTopicSpentTotals, cfg.KafkaGroupIDSpentTotals)
	if err != nil {
		return nil, err
	}
	return &AdvKafkaReaders{r}, nil
}
