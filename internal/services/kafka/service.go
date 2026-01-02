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

// Новая функция для автоматического создания топика
func ensureTopicExists(brokers []string, topic string) error {
	conn, err := kafka.Dial("tcp", brokers[0])
	if err != nil {
		return fmt.Errorf("⚠️ Failed to connect to broker %s: %v", brokers[0], err)
	}
	defer conn.Close()

	// Получаем контроллера (ведущий брокер)
	controller, err := conn.Controller()
	if err != nil {
		return fmt.Errorf("failed to get controller: %v", err)
	}

	// Подключаемся к контроллеру для создания темы
	controllerConn, err := kafka.Dial("tcp", fmt.Sprintf("%s:%d", controller.Host, controller.Port))
	if err != nil {
		return fmt.Errorf("failed to connect to controller: %v", err)
	}
	defer controllerConn.Close()

	// Получаем список тем
	partitions, err := controllerConn.ReadPartitions()
	if err != nil {
		return fmt.Errorf("failed to read partitions: %v", err)
	}

	// Проверяем существует ли тема
	for _, p := range partitions {
		if p.Topic == topic {
			log.Printf("✅ Topic %s already exists", topic)
			return nil
		}
	}

	// Для продакшена RTB-статистики
	retentionHours := 24 // или 72 для 3 дней

	configs := []kafka.ConfigEntry{
		{
			ConfigName:  "retention.ms",
			ConfigValue: fmt.Sprintf("%d", retentionHours*60*60*1000),
		},
		{
			ConfigName:  "retention.bytes",
			ConfigValue: fmt.Sprintf("%d", 2*1024*1024*1024), // 2 GB
		},
		{
			ConfigName:  "cleanup.policy",
			ConfigValue: "delete",
		},
		{
			ConfigName:  "segment.bytes",
			ConfigValue: fmt.Sprintf("%d", 100*1024*1024), // 100 MB сегменты
		},
	}

	// Создаем тему если не существует
	topicConfigs := []kafka.TopicConfig{
		{
			Topic:             topic,
			NumPartitions:     1,
			ReplicationFactor: 1,
			ConfigEntries:     configs,
		},
	}

	err = controllerConn.CreateTopics(topicConfigs...)
	if err != nil {
		return fmt.Errorf("failed to create topic: %v", err)
	}

	log.Printf("✅ Created topic: %s with %d partitions", topic, 1)

	// Даем время для создания темы
	time.Sleep(2 * time.Second)
	return nil
}

func InitKafkaReader(cfg config.KafkaConfig) (*kafka.Reader, error) {
	// Сначала проверяем подключение к брокерам
	log.Printf("🔌 Checking Kafka connection to: %v", cfg.KafkaBrokers)

	err := checkKafkaBrokers(cfg.KafkaBrokers)
	if err != nil {
		return nil, fmt.Errorf("Kafka connection failed: %v", err)
	}

	// Автоматически создаем топик если не существует
	log.Printf("🔍 Ensuring Kafka topic exists: %s", cfg.KafkaTopic)
	err = ensureTopicExists(cfg.KafkaBrokers, cfg.KafkaTopic)
	if err != nil {
		return nil, fmt.Errorf("⚠️ Failed to ensure topic exists: %v", err)
		// Не прерываем выполнение, продолжаем попытку инициализации ридера
	} else {
		log.Printf("✅ Kafka topic %s is ready", cfg.KafkaTopic)
	}

	// Создаем ридер
	kafkaReader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: cfg.KafkaBrokers,
		Topic:   cfg.KafkaTopic,
		GroupID: cfg.KafkaGroupID,

		// 🔴 КЛЮЧЕВО ДЛЯ СКОРОСТИ
		MinBytes: 10 << 20,  // 10 MB
		MaxBytes: 100 << 20, // 100 MB
		MaxWait:  5 * time.Millisecond,

		// 🔴 внутренний буфер
		QueueCapacity: 50000,

		// 🔴 коммиты не на каждый ReadMessage
		CommitInterval: time.Second,

		// опционально
		ReadLagInterval: -1,
	})

	log.Println("✅ Kafka reader initialized successfully")
	return kafkaReader, nil
}

// Старая функция checkKafkaTopic оставлена для обратной совместимости
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
