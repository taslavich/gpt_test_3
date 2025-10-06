package kafka_service

import (
	"fmt"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/config"
)

func checkKafkaBrokers(brokers []string) error {
	for _, broker := range brokers {
		conn, err := kafka.Dial("tcp", broker)
		if err != nil {
			return fmt.Errorf("failed to connect to broker %s: %v", broker, err)
		}
		defer conn.Close()

		// Дополнительно проверяем, что брокер отвечает
		_, err = conn.ApiVersions()
		if err != nil {
			return fmt.Errorf("broker %s not responding: %v", broker, err)
		}

		log.Printf("✅ Connected to Kafka broker: %s", broker)
	}
	return nil
}

func InitKafkaReader(cfg config.KafkaConfig) (*kafka.Reader, error) {
	// Сначала проверяем подключение к брокерам
	log.Printf("🔌 Checking Kafka connection to: %v", cfg.KafkaBrokers)

	err := checkKafkaBrokers(cfg.KafkaBrokers)
	if err != nil {
		return nil, fmt.Errorf("Kafka connection failed: %v", err)
	}

	// Проверяем существование топика
	err = checkKafkaTopic(cfg.KafkaBrokers, cfg.KafkaTopic)
	if err != nil {
		return nil, fmt.Errorf("Kafka topic check failed: %v", err)
	}

	// Создаем ридер
	kafkaReader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  cfg.KafkaBrokers,
		Topic:    cfg.KafkaTopic,
		GroupID:  cfg.KafkaGroupID,
		MinBytes: 10e3,
		MaxBytes: 10e6,
		MaxWait:  1 * time.Second,
	})

	log.Println("✅ Kafka reader initialized successfully")
	return kafkaReader, nil
}

func checkKafkaTopic(brokers []string, topic string) error {
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
	// Проверяем подключение ко всем брокерам
	for _, broker := range brokers {
		conn, err := kafka.Dial("tcp", broker)
		if err != nil {
			return fmt.Errorf("failed to connect to Kafka broker %s: %v", broker, err)
		}
		defer conn.Close()

		// Проверяем, что брокер отвечает
		_, err = conn.ApiVersions()
		if err != nil {
			return fmt.Errorf("Kafka broker %s not responding: %v", broker, err)
		}

		log.Printf("✅ Connected to Kafka broker: %s", broker)
	}

	// Проверяем существование топика на первом брокере
	conn, err := kafka.Dial("tcp", brokers[0])
	if err != nil {
		return fmt.Errorf("failed to connect for topic check: %v", err)
	}
	defer conn.Close()

	partitions, err := conn.ReadPartitions()
	if err != nil {
		return fmt.Errorf("failed to read partitions: %v", err)
	}

	topicExists := false
	for _, p := range partitions {
		if p.Topic == topic {
			topicExists = true
			break
		}
	}

	if !topicExists {
		log.Printf("⚠️ Topic %s does not exist, but will be auto-created", topic)
		// Не возвращаем ошибку, т.к. топик может создаваться автоматически
	}

	log.Printf("✅ Kafka writer connection OK - brokers: %v, topic: %s", brokers, topic)
	return nil
}

func CreateKafkaWriter(brokers []string, topic string) (*kafka.Writer, error) {
	// Проверяем подключение перед созданием writer
	log.Printf("🔌 Checking Kafka writer connection to brokers: %v, topic: %s", brokers, topic)

	err := checkKafkaWriterConnection(brokers, topic)
	if err != nil {
		return nil, fmt.Errorf("Kafka writer initialization failed: %v", err)
	}

	// Создаем writer с массивом брокеров
	writer := &kafka.Writer{
		Addr:         kafka.TCP(brokers...), // Вот здесь массив брокеров!
		Topic:        topic,
		Balancer:     &kafka.LeastBytes{},
		Async:        false,
		BatchTimeout: 100 * time.Millisecond,
		MaxAttempts:  3,
	}

	log.Println("✅ Kafka writer initialized successfully")
	return writer, nil
}
