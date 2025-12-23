package config

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
	"github.com/joho/godotenv"
)

// Кастомный тип для map[string]string
type MapStringToString map[string]string

func (m *MapStringToString) SetValue(value string) error {
	*m = make(MapStringToString)
	if value == "" {
		return nil
	}

	pairs := strings.Split(value, ",")
	for _, pair := range pairs {
		// Ищем только ПЕРВЫЙ знак = как разделитель ключ-значение
		idx := strings.Index(pair, "|")
		if idx == -1 {
			continue // пропускаем некорректные пары
		}

		key := strings.TrimSpace(pair[:idx])
		valueStr := strings.TrimSpace(pair[idx+1:])
		(*m)[key] = valueStr
	}
	return nil
}

type MapStringToDuration map[string]time.Duration

func (m *MapStringToDuration) SetValue(value string) error {
	*m = make(MapStringToDuration)
	if value == "" {
		return nil
	}

	pairs := strings.Split(value, ",")
	for _, pair := range pairs {
		// Ищем только ПЕРВЫЙ знак | как разделитель ключ-значение
		idx := strings.Index(pair, "|")
		if idx == -1 {
			continue // пропускаем некорректные пары
		}

		key := strings.TrimSpace(pair[:idx])
		durationStr := strings.TrimSpace(pair[idx+1:])

		// Парсим duration из строки
		duration, err := time.ParseDuration(durationStr)
		if err != nil {
			return fmt.Errorf("invalid duration format for key '%s': %w", key, err)
		}

		(*m)[key] = duration
	}
	return nil
}

// Кастомный тип для map[string][]string
type MapStringToStringSlice map[string][]string

func (m *MapStringToStringSlice) SetValue(value string) error {
	*m = make(MapStringToStringSlice)
	if value == "" {
		return nil
	}

	pairs := strings.Split(value, ",")
	for _, pair := range pairs {
		// Ищем только ПЕРВЫЙ знак = как разделитель ключ-значение
		idx := strings.Index(pair, "=")
		if idx == -1 {
			continue // пропускаем некорректные пары
		}

		key := strings.TrimSpace(pair[:idx])
		valueStr := strings.TrimSpace(pair[idx+1:])

		// Разделяем URL по |
		urls := strings.Split(valueStr, "|")
		(*m)[key] = make([]string, len(urls))
		for i, url := range urls {
			(*m)[key][i] = strings.TrimSpace(url)
		}
	}
	return nil
}

// Кастомный тип для []string
type ListString []string

func (l *ListString) SetValue(value string) error {
	*l = make(ListString, 0)
	if value == "" {
		return nil
	}

	items := strings.Split(value, ",")
	for _, item := range items {
		*l = append(*l, strings.TrimSpace(item))
	}
	return nil
}

type BiddingEngineConfig struct {
	HttpServer                          HttpServer
	GrpcServer                          GrpcServer
	ProfitPercent                       float32 `yaml:"PROFIT_PERCENT" env:"PROFIT_PERCENT" env-default:"0.2"`
	SystemHostname                      string  `yaml:"SYSTEM_HOSTNAME" env:"SYSTEM_HOSTNAME"`
	SspGeoDspPercentsAdultFilePath      string  `yaml:"SSP_GEO_DSP_PERCENTS_ADULT_FILE_PATH" env:"SSP_GEO_DSP_PERCENTS_ADULT_FILE_PATH"`
	SspGeoDspPercentsMainstreamFilePath string  `yaml:"SSP_GEO_DSP_PERCENTS_MAINSTREAM_FILE_PATH" env:"SSP_GEO_DSP_PERCENTS_MAINSTREAM_FILE_PATH"`
	RedisConfig
}

type RouterConfig struct {
	GrpcServer                   GrpcServer
	HttpServer                   HttpServer
	DSPEndpointsAdult_v_2_5      MapStringToString `yaml:"DSP_ENDPOINTS_ADULT_V_2_5" env:"DSP_ENDPOINTS_ADULT_V_2_5"`
	DSPEndpointsMainstream_v_2_5 MapStringToString `yaml:"DSP_ENDPOINTS_MAINSTREAM_V_2_5" env:"DSP_ENDPOINTS_MAINSTREAM_V_2_5"`

	DspRulesConfigPathV25 string `yaml:"DSP_RULES_CONFIG_PATH" env:"DSP_RULES_CONFIG_PATH_V_25"`
	SppRulesConfigPathV25 string `yaml:"SPP_RULES_CONFIG_PATH" env:"SPP_RULES_CONFIG_PATH_V_25"`

	AllowedIpDbPath                  string `yaml:"ALLOWED_IP_DB_PATH" env:"ALLOWED_IP_DB_PATH"`
	SspGeoDspLinksAdultFilePath      string `yaml:"SSP_GEO_DSP_LINKS_ADULT_FILE_PATH" env:"SSP_GEO_DSP_LINKS_ADULT_FILE_PATH"`
	SspGeoDspLinksMainstreamFilePath string `yaml:"SSP_GEO_DSP_LINKS_MAINSTREAM_FILE_PATH" env:"SSP_GEO_DSP_LINKS_MAINSTREAM_FILE_PATH"`

	BidResponsesTimeout time.Duration `yaml:"BID_RESPONSES_TIMEOUT" env:"BID_RESPONSES_TIMEOUT"`

	DspFiltersFilePath string `yaml:"DSP_FILTERS_FILE_PATH" env:"DSP_FILTERS_FILE_PATH"`

	SspHttpClientTimeouts MapStringToDuration `yaml:"SSP_HTTP_CLIENT_TIMEOUT" env:"SSP_HTTP_CLIENT_TIMEOUT"`

	MaxParallelRequests int  `yaml:"MAX_PARALLEL_REQUESTS" env:"MAX_PARALLEL_REQUESTS" env-default:"64"`
	Debug               bool `yaml:"DEBUG" env:"DEBUG" env-default:"false"`

	RedisConfig
}

type OrchestratorConfig struct {
	GrpcServer     GrpcServer
	UriOfBidEngine string        `yaml:"URI_OF_BID_ENGINE" env:"URI_OF_BID_ENGINE"`
	UriOfDspRouter string        `yaml:"URI_OF_DSP_ROUTER" env:"URI_OF_DSP_ROUTER"`
	AuctionTimeout time.Duration `yaml:"AUCTION_TIMEOUT" env:"AUCTION_TIMEOUT"`
	GetBidsTimeout time.Duration `yaml:"GET_BIDS_TIMEOUT" env:"GET_BIDS_TIMEOUT"`

	RedisConfig
}

type SppAdapterConfig struct {
	HttpServer          HttpServer
	UriOfOrchestrator   string            `yaml:"URI_OF_ORCHESTRATOR" env:"URI_OF_ORCHESTRATOR"`
	AdmTimeout          time.Duration     `yaml:"ADM_TIMEOUT" env:"ADM_TIMEOUT"`
	NurlTimeout         time.Duration     `yaml:"NURL_TIMEOUT" env:"NURL_TIMEOUT"`
	GetWinnerBidTimeout time.Duration     `yaml:"GET_WINNER_BID_TIMEOUT" env:"GET_WINNER_BID_TIMEOUT"`
	GeoIpDbPath         string            `yaml:"GEO_IP_DB_PATH" env:"GEO_IP_DB_PATH"`
	SspAdultFeeds       MapStringToString `yaml:"SSP_ADULT_FEEDS" env:"SSP_ADULT_FEEDS"`
	SspMainStreamFeeds  MapStringToString `yaml:"SSP_MAINSTREAM_FEEDS" env:"SSP_MAINSTREAM_FEEDS"`
	SiteIdDomainPath    string            `yaml:"SITE_ID_DOMAIN_PATH" env:"SITE_ID_DOMAIN_PATH"`

	RedisConfig
}

type AdmAdapterConfig struct {
	HttpServer   HttpServer
	AdmTimeout   time.Duration `yaml:"ADM_TIMEOUT" env:"ADM_TIMEOUT"`
	NurlTimeout  time.Duration `yaml:"NURL_TIMEOUT" env:"NURL_TIMEOUT"`
	FullChain    string        `yaml:"FULLCHAIN_PEM" env:"FULLCHAIN_PEM"`
	PrivKey      string        `yaml:"PRIVKEY_PEM" env:"PRIVKEY_PEM"`
	RsaFullChain string        `yaml:"RSA_FULLCHAIN_PEM" env:"RSA_FULLCHAIN_PEM"`
	RsaPrivKey   string        `yaml:"RSA_PRIVKEY_PEM" env:"RSA_PRIVKEY_PEM"`

	RedisConfig
}

type KafkaLoaderConfig struct {
	RedisConfig
	KafkaConfig
	BatchSize int64 `yaml:"BATCH_SIZE" env:"BATCH_SIZE"`
}

type ClickhouseConfig struct {
	ClickHouseTable string `yaml:"CLICK_HOUSE_TABLE" env:"CLICK_HOUSE_TABLE"`
	BatchSize       int    `yaml:"CLICKHOUSE_BATCH_SIZE" env:"CLICKHOUSE_BATCH_SIZE"`
	Username        string `yaml:"CLICKHOUSE_USERNAME" env:"CLICKHOUSE_USERNAME"`
	Password        string `yaml:"CLICKHOUSE_PASSWORD" env:"CLICKHOUSE_PASSWORD"`
	Host            string `yaml:"CLICKHOUSE_HOST" env:"CLICKHOUSE_HOST" env-default:"hntzp0jsnf.europe-west4.gcp.clickhouse.cloud"`
	Port            string `yaml:"CLICKHOUSE_PORT" env:"CLICKHOUSE_PORT" env-default:"9440"`
	Database        string `yaml:"CLICKHOUSE_DB" env:"CLICKHOUSE_DB" env-default:"rtb"`
}

type PercenterConfig struct {
	Clickhouse     ClickhouseConfig
	UriOfBidEngine string `yaml:"URI_OF_BID_ENGINE" env:"URI_OF_BID_ENGINE"`
}

type ClickhouseLoaderConfig struct {
	Kafka      KafkaConfig
	Clickhouse ClickhouseConfig
	TimeoutSec int `yaml:"TIMEOUT_SEC" env:"TIMEOUT_SEC"`
}

type MockDspConfig struct {
	HttpServer HttpServer
	DspName    string  `env:"DSP_NAME"`
	Price      float32 `env:"PRICE"`
	Adid       string  `env:"ADID"`
	Adm        string  `env:"ADM"`
}

type RedisConfig struct {
	RedisHost     string `yaml:"REDIS_HOST" env:"REDIS_HOST"`
	RedisPort     string `yaml:"REDIS_PORT" env:"REDIS_PORT"`
	RedisDB       int    `yaml:"REDIS_DB" env:"REDIS_DB"`
	RedisPassword string `yaml:"REDIS_PASSWORD" env:"REDIS_PASSWORD"`
}

type KafkaConfig struct {
	KafkaBrokers     []string `yaml:"KAFKA_BROKERS" env:"KAFKA_BROKERS"`
	KafkaTopic       string   `yaml:"KAFKA_TOPIC" env:"KAFKA_TOPIC"`
	FlushIntervalSec int      `yaml:"FLUSH_INTERVAL_SEC" env:"FLUSH_INTERVAL_SEC"`
	KafkaGroupID     string   `yaml:"KAFKA_GROUP_ID" env:"KAFKA_GROUP_ID"`
}

type HttpServer struct {
	Host string `yaml:"HTTP_HOSTNAME" env:"HTTP_HOSTNAME"`
	Port uint16 `yaml:"HTTP_PORT" env:"HTTP_PORT"`
}

type GrpcServer struct {
	Host string `yaml:"GRPC_HOSTNAME" env:"GRPC_HOSTNAME"`
	Port uint16 `yaml:"GRPC_PORT" env:"GRPC_PORT"`
}

func getEnvFileNames() []string {
	return []string{".env.local", ".env", "bid-engine.env", "clickhouse-loader.env", "kafka-loader.env", "dsp1.env", "dsp2.env", "dsp3.env", "orchestrator.env", "router.env", "spp-adapter.env"}
}

func LoadConfig[
	T BiddingEngineConfig |
		RouterConfig |
		SppAdapterConfig |
		OrchestratorConfig |
		KafkaLoaderConfig |
		ClickhouseLoaderConfig |
		MockDspConfig |
		PercenterConfig |
		AdmAdapterConfig,
](ctx context.Context) (*T, error) {
	for _, fileName := range getEnvFileNames() {
		err := godotenv.Load(fileName)
		if err != nil {
			log.Printf("error loading %s fileName : %v", fileName, err)
		}
	}

	var cfg T
	err := cleanenv.ReadEnv(&cfg)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}
