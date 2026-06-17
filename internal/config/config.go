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
		// Ищем только ПЕРВЫЙ знак | как разделитель ключ-значение
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
	SspGeoDspPercentsAdultFilePath      string  `yaml:"SSP_GEO_DSP_PERCENTS_ADULT_FILE_PATH" env:"SSP_GEO_DSP_PERCENTS_ADULT_FILE_PATH"`
	SspGeoDspPercentsMainstreamFilePath string  `yaml:"SSP_GEO_DSP_PERCENTS_MAINSTREAM_FILE_PATH" env:"SSP_GEO_DSP_PERCENTS_MAINSTREAM_FILE_PATH"`
	AdmDomain                           string  `yaml:"ADM_DOMAIN" env:"ADM_DOMAIN"`
	BotBaseURL                          string  `yaml:"BOT_BASE_URL" env:"BOT_BASE_URL"`
	BotInternalSecret                   string  `yaml:"BOT_INTERNAL_SECRET" env:"BOT_INTERNAL_SECRET"`

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

	CidSspDspLinksAdultFilePath      string `yaml:"CID_SSP_DSP_LINKS_ADULT_FILE_PATH" env:"CID_SSP_DSP_LINKS_ADULT_FILE_PATH"`
	CidSspDspLinksMainstreamFilePath string `yaml:"CID_SSP_DSP_LINKS_MAINSTREAM_FILE_PATH" env:"CID_SSP_DSP_LINKS_MAINSTREAM_FILE_PATH"`

	BidResponsesTimeout time.Duration `yaml:"BID_RESPONSES_TIMEOUT" env:"BID_RESPONSES_TIMEOUT"`

	DspFiltersAdlFilePath string `yaml:"DSP_FILTERS_ADULT_FILE_PATH" env:"DSP_FILTERS_ADULT_FILE_PATH"`
	DspFiltersMcFilePath  string `yaml:"DSP_FILTERS_MAINSTREAM_FILE_PATH" env:"DSP_FILTERS_MAINSTREAM_FILE_PATH"`

	DspChangersAdlFilePath string `yaml:"DSP_CHANGERS_ADULT_FILE_PATH" env:"DSP_CHANGERS_ADULT_FILE_PATH"`
	DspChangersMcFilePath  string `yaml:"DSP_CHANGERS_MAINSTREAM_FILE_PATH" env:"DSP_CHANGERS_MAINSTREAM_FILE_PATH"`

	SspHttpClientTimeouts MapStringToDuration `yaml:"SSP_HTTP_CLIENT_TIMEOUT" env:"SSP_HTTP_CLIENT_TIMEOUT"`

	MaxParallelRequests int    `yaml:"MAX_PARALLEL_REQUESTS" env:"MAX_PARALLEL_REQUESTS" env-default:"64"`
	Debug               bool   `yaml:"DEBUG" env:"DEBUG" env-default:"false"`
	BotBaseURL          string `yaml:"BOT_BASE_URL" env:"BOT_BASE_URL"`
	BotInternalSecret   string `yaml:"BOT_INTERNAL_SECRET" env:"BOT_INTERNAL_SECRET"`

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
	HttpServer HttpServer
	ClickhouseConfig
	UriOfOrchestrator   string        `yaml:"URI_OF_ORCHESTRATOR" env:"URI_OF_ORCHESTRATOR"`
	AdmTimeout          time.Duration `yaml:"ADM_TIMEOUT" env:"ADM_TIMEOUT"`
	NurlTimeout         time.Duration `yaml:"NURL_TIMEOUT" env:"NURL_TIMEOUT"`
	GetWinnerBidTimeout time.Duration `yaml:"GET_WINNER_BID_TIMEOUT" env:"GET_WINNER_BID_TIMEOUT"`
	GeoIpDbPath         string        `yaml:"GEO_IP_DB_PATH" env:"GEO_IP_DB_PATH"`

	// POP
	SspPopAdlFeeds MapStringToString `yaml:"SSP_POP_ADL_FEEDS" env:"SSP_POP_ADL_FEEDS"`
	SspPopMcFeeds  MapStringToString `yaml:"SSP_POP_MC_FEEDS" env:"SSP_POP_MC_FEEDS"`

	// BAN
	SspBanAdlFeeds MapStringToString `yaml:"SSP_BAN_ADL_FEEDS" env:"SSP_BAN_ADL_FEEDS"`
	SspBanMcFeeds  MapStringToString `yaml:"SSP_BAN_MC_FEEDS" env:"SSP_BAN_MC_FEEDS"`

	// NAT
	SspNatAdlFeeds MapStringToString `yaml:"SSP_NAT_ADL_FEEDS" env:"SSP_NAT_ADL_FEEDS"`
	SspNatMcFeeds  MapStringToString `yaml:"SSP_NAT_MC_FEEDS" env:"SSP_NAT_MC_FEEDS"`

	// IPP
	SspIppAdlFeeds MapStringToString `yaml:"SSP_IPP_ADL_FEEDS" env:"SSP_IPP_ADL_FEEDS"`
	SspIppMcFeeds  MapStringToString `yaml:"SSP_IPP_MC_FEEDS" env:"SSP_IPP_MC_FEEDS"`

	SiteIdDomainPath              string `yaml:"SITE_ID_DOMAIN_PATH" env:"SITE_ID_DOMAIN_PATH"`
	Domains1LevelPath             string `yaml:"DOMAINS_1_LEVEL_PATH" env:"DOMAINS_1_LEVEL_PATH"`
	Domains23LevelPath            string `yaml:"DOMAINS_23_LEVEL_PATH" env:"DOMAINS_23_LEVEL_PATH"`
	GeoToLangPath                 string `yaml:"GEO_TO_LANG_PATH" env:"GEO_TO_LANG_PATH"`
	SspAdapterWorkStatusURL       string `yaml:"SSP_ADAPTER_WORK_STATUS_URL" env:"SSP_ADAPTER_WORK_STATUS_URL"`
	IPLimitFullReloadMinutes      int    `yaml:"IP_LIMIT_FULL_RELOAD_MINUTES" env:"IP_LIMIT_FULL_RELOAD_MINUTES" env-default:"60"`
	IPLimitLatestBatchIntervalSec int    `yaml:"IP_LIMIT_LATEST_BATCH_INTERVAL_SEC" env:"IP_LIMIT_LATEST_BATCH_INTERVAL_SEC" env-default:"60"`
	IPLimitIPv4Table              string `yaml:"IP_LIMIT_IPV4_TABLE" env:"IP_LIMIT_IPV4_TABLE" env-default:"ip_limit_ipv4"`
	IPLimitIPv6Table              string `yaml:"IP_LIMIT_IPV6_TABLE" env:"IP_LIMIT_IPV6_TABLE" env-default:"ip_limit_ipv6"`
	BotBaseURL                    string `yaml:"BOT_BASE_URL" env:"BOT_BASE_URL"`
	BotInternalSecret             string `yaml:"BOT_INTERNAL_SECRET" env:"BOT_INTERNAL_SECRET"`

	RedisConfig
}

type AdmAdapterConfig struct {
	HttpServer              HttpServer
	AdmTimeout              time.Duration `yaml:"ADM_TIMEOUT" env:"ADM_TIMEOUT"`
	NurlTimeout             time.Duration `yaml:"NURL_TIMEOUT" env:"NURL_TIMEOUT"`
	FullChain               string        `yaml:"FULLCHAIN_PEM" env:"FULLCHAIN_PEM"`
	PrivKey                 string        `yaml:"PRIVKEY_PEM" env:"PRIVKEY_PEM"`
	RsaFullChain            string        `yaml:"RSA_FULLCHAIN_PEM" env:"RSA_FULLCHAIN_PEM"`
	RsaPrivKey              string        `yaml:"RSA_PRIVKEY_PEM" env:"RSA_PRIVKEY_PEM"`
	SspAdapterWorkStatusURL string        `yaml:"SSP_ADAPTER_WORK_STATUS_URL" env:"SSP_ADAPTER_WORK_STATUS_URL"`
	BotBaseURL              string        `yaml:"BOT_BASE_URL" env:"BOT_BASE_URL"`
	BotInternalSecret       string        `yaml:"BOT_INTERNAL_SECRET" env:"BOT_INTERNAL_SECRET"`

	RedisConfig
}
type KafkaLoaderConfig struct {
	RedisConfig
	KafkaConfig
	ClickhouseConfig
	BatchRatioConfig
	EmptyLoopPauseMS         int      `yaml:"EMPTY_LOOP_PAUSE_MS" env:"EMPTY_LOOP_PAUSE_MS" env-default:"200"`
	SspAdapterWorkStatusURLs []string `yaml:"SSP_ADAPTER_WORK_STATUS_URLS" env:"SSP_ADAPTER_WORK_STATUS_URLS" env-default:"server1.twinbidexchange.com:8050,server2.twinbidexchange.com:8050,server3.twinbidexchange.com:8050,server4.twinbidexchange.com:8050"`
	BotBaseURL               string   `yaml:"BOT_BASE_URL" env:"BOT_BASE_URL"`
	BotInternalSecret        string   `yaml:"BOT_INTERNAL_SECRET" env:"BOT_INTERNAL_SECRET"`
}

type ClickhouseConfig struct {
	BatchSize       int    `yaml:"CLICKHOUSE_BATCH_SIZE" env:"CLICKHOUSE_BATCH_SIZE"`
	Username        string `yaml:"CLICKHOUSE_USERNAME" env:"CLICKHOUSE_USERNAME"`
	Password        string `yaml:"CLICKHOUSE_PASSWORD" env:"CLICKHOUSE_PASSWORD"`
	Host            string `yaml:"CLICKHOUSE_HOST" env:"CLICKHOUSE_HOST" env-default:"hntzp0jsnf.europe-west4.gcp.clickhouse.cloud"`
	Port            string `yaml:"CLICKHOUSE_PORT" env:"CLICKHOUSE_PORT" env-default:"9440"`
	Database        string `yaml:"CLICKHOUSE_DB" env:"CLICKHOUSE_DB" env-default:"rtb"`
	DatabaseDefault string `yaml:"CLICKHOUSE_DEFAULT_DB" env:"CLICKHOUSE_DEFAULT_DB" env-default:"default"`

	// ClickHouse tables and batches by event type
	TableOrtb                   string  `yaml:"CLICKHOUSE_TABLE_ORTB" env:"CLICKHOUSE_TABLE_ORTB" env-default:"ortb"`
	TableImpressions            string  `yaml:"CLICKHOUSE_TABLE_IMPRESSIONS" env:"CLICKHOUSE_TABLE_IMPRESSIONS" env-default:"impressions_in"`
	TableClicks                 string  `yaml:"CLICKHOUSE_TABLE_CLICKS" env:"CLICKHOUSE_TABLE_CLICKS" env-default:"clicks_in"`
	BatchSizeOrtb               int     `yaml:"CLICKHOUSE_BATCH_SIZE_ORTB" env:"CLICKHOUSE_BATCH_SIZE_ORTB"`
	BatchSizeImpressions        int     `yaml:"CLICKHOUSE_BATCH_SIZE_IMPRESSIONS" env:"CLICKHOUSE_BATCH_SIZE_IMPRESSIONS"`
	BatchSizeClicks             int     `yaml:"CLICKHOUSE_BATCH_SIZE_CLICKS" env:"CLICKHOUSE_BATCH_SIZE_CLICKS"`
	BatchSizeImpressionsPercent float64 `yaml:"CLICKHOUSE_BATCH_SIZE_IMPRESSIONS_PERCENT" env:"CLICKHOUSE_BATCH_SIZE_IMPRESSIONS_PERCENT"`
	BatchSizeClicksPercent      float64 `yaml:"CLICKHOUSE_BATCH_SIZE_CLICKS_PERCENT" env:"CLICKHOUSE_BATCH_SIZE_CLICKS_PERCENT"`
	BatchTimeoutMS              int     `yaml:"CLICKHOUSE_BATCH_TIMEOUT_MS" env:"CLICKHOUSE_BATCH_TIMEOUT_MS" env-default:"800"`
}

type BatchRatioConfig struct {
	TableOrtb               string  `yaml:"BATCH_RATIO_TABLE_ORTB" env:"BATCH_RATIO_TABLE_ORTB" env-default:"ortb"`
	TableImpressions        string  `yaml:"BATCH_RATIO_TABLE_IMPRESSIONS" env:"BATCH_RATIO_TABLE_IMPRESSIONS" env-default:"impressions_in"`
	TableClicks             string  `yaml:"BATCH_RATIO_TABLE_CLICKS" env:"BATCH_RATIO_TABLE_CLICKS" env-default:"clicks_in"`
	TickerEnabled           bool    `yaml:"BATCH_RATIO_TICKER_ENABLED" env:"BATCH_RATIO_TICKER_ENABLED" env-default:"true"`
	TickerIntervalSec       int     `yaml:"BATCH_RATIO_TICKER_INTERVAL_SEC" env:"BATCH_RATIO_TICKER_INTERVAL_SEC" env-default:"30"`
	TickerRequestTimeoutMS  int     `yaml:"BATCH_RATIO_TICKER_REQUEST_TIMEOUT_MS" env:"BATCH_RATIO_TICKER_REQUEST_TIMEOUT_MS" env-default:"2000"`
	TickerRetryAttempts     int     `yaml:"BATCH_RATIO_TICKER_RETRY_ATTEMPTS" env:"BATCH_RATIO_TICKER_RETRY_ATTEMPTS" env-default:"3"`
	ImpressionsDiffLeftSec  int     `yaml:"BATCH_RATIO_IMPRESSIONS_DIFF_LEFT_SEC" env:"BATCH_RATIO_IMPRESSIONS_DIFF_LEFT_SEC" env-default:"-300"`
	ImpressionsDiffRightSec int     `yaml:"BATCH_RATIO_IMPRESSIONS_DIFF_RIGHT_SEC" env:"BATCH_RATIO_IMPRESSIONS_DIFF_RIGHT_SEC" env-default:"300"`
	ClicksDiffLeftSec       int     `yaml:"BATCH_RATIO_CLICKS_DIFF_LEFT_SEC" env:"BATCH_RATIO_CLICKS_DIFF_LEFT_SEC" env-default:"-300"`
	ClicksDiffRightSec      int     `yaml:"BATCH_RATIO_CLICKS_DIFF_RIGHT_SEC" env:"BATCH_RATIO_CLICKS_DIFF_RIGHT_SEC" env-default:"300"`
	AdjustFactor            float64 `yaml:"BATCH_RATIO_ADJUST_FACTOR" env:"BATCH_RATIO_ADJUST_FACTOR" env-default:"2"`
	HTTPHost                string  `yaml:"BATCH_RATIO_HTTP_HOST" env:"BATCH_RATIO_HTTP_HOST" env-default:"0.0.0.0"`
	HTTPPort                uint16  `yaml:"BATCH_RATIO_HTTP_PORT" env:"BATCH_RATIO_HTTP_PORT" env-default:"8090"`
}

type PercenterConfig struct {
	Clickhouse     ClickhouseConfig
	UriOfBidEngine string `yaml:"URI_OF_BID_ENGINE" env:"URI_OF_BID_ENGINE"`
}

type ClickhouseLoaderConfig struct {
	Kafka      KafkaConfig
	Clickhouse ClickhouseConfig
	BatchRatioConfig

	TimeoutSec     int `yaml:"TIMEOUT_SEC" env:"TIMEOUT_SEC"`
	BatchTimeoutMS int `yaml:"CLICKHOUSE_BATCH_TIMEOUT_MS" env:"CLICKHOUSE_BATCH_TIMEOUT_MS" env-default:"800"`

	ImpressionClickFlushIntervalSec int    `yaml:"IMPRESSION_CLICK_FLUSH_INTERVAL_SEC" env:"IMPRESSION_CLICK_FLUSH_INTERVAL_SEC" env-default:"30"`
	EmptyLoopPauseMS                int    `yaml:"EMPTY_LOOP_PAUSE_MS" env:"EMPTY_LOOP_PAUSE_MS" env-default:"200"`
	BotBaseURL                      string `yaml:"BOT_BASE_URL" env:"BOT_BASE_URL"`
	BotInternalSecret               string `yaml:"BOT_INTERNAL_SECRET" env:"BOT_INTERNAL_SECRET"`
}

type MockDspConfig struct {
	HttpServer HttpServer
	DspName    string  `env:"DSP_NAME"`
	Price      float32 `env:"PRICE"`
	Adid       string  `env:"ADID"`
	Adm        string  `env:"ADM"`
}

type RedisConfig struct {
	RedisPassword string `yaml:"REDIS_PASSWORD" env:"REDIS_PASSWORD"`

	RedisShardAddrs    []string `yaml:"REDIS_SHARD_ADDRS" env:"REDIS_SHARD_ADDRS"`
	RedisShardTLSAddrs []string `yaml:"REDIS_SHARD_TLS_ADDRS" env:"REDIS_SHARD_TLS_ADDRS"`
	RedisUseTLS        bool     `yaml:"REDIS_USE_TLS" env:"REDIS_USE_TLS" env-default:"false"`
	RedisPoolSize      int      `yaml:"REDIS_POOL_SIZE" env:"REDIS_POOL_SIZE" env-default:"64"`
	RedisMinIdleConns  int      `yaml:"REDIS_MIN_IDLE_CONNS" env:"REDIS_MIN_IDLE_CONNS" env-default:"16"`

	// Separate Redis for fast ADM/NURL UUID guards.
	RedisUUIDAddr   string        `yaml:"REDIS_UUID_ADDR" env:"REDIS_UUID_ADDR"`
	RedisDBAdm      int           `yaml:"REDIS_DB_ADM" env:"REDIS_DB_ADM" env-default:"1"`
	RedisDBNurl     int           `yaml:"REDIS_DB_NURL" env:"REDIS_DB_NURL" env-default:"0"`
	RedisUUIDKeyTTL time.Duration `yaml:"REDIS_UUID_KEY_TTL" env:"REDIS_UUID_KEY_TTL" env-default:"45m"`

	// Redis ORTB
	RedisDBOrtb   int    `yaml:"REDIS_DB_ORTB" env:"REDIS_DB_ORTB"`
	BatchSizeOrtb int64  `yaml:"BATCH_SIZE_ORTB" env:"BATCH_SIZE_ORTB"`
	RedisSetOrtb  string `yaml:"REDIS_SET_ORTB" env:"REDIS_SET_ORTB" env-default:"ortb:ready"`

	// Redis Impressions
	RedisDBImpressions          int     `yaml:"REDIS_DB_IMPRESSIONS" env:"REDIS_DB_IMPRESSIONS"`
	BatchSizeImpressions        int64   `yaml:"BATCH_SIZE_IMPRESSIONS" env:"BATCH_SIZE_IMPRESSIONS"`
	BatchSizeImpressionsPercent float64 `yaml:"BATCH_SIZE_IMPRESSIONS_PERCENT" env:"BATCH_SIZE_IMPRESSIONS_PERCENT"`
	RedisSetImpressions         string  `yaml:"REDIS_SET_IMPRESSIONS" env:"REDIS_SET_IMPRESSIONS" env-default:"impressions:ready"`

	// Redis Clicks
	RedisDBClicks          int     `yaml:"REDIS_DB_CLICKS" env:"REDIS_DB_CLICKS"`
	BatchSizeClicks        int64   `yaml:"BATCH_SIZE_CLICKS" env:"BATCH_SIZE_CLICKS"`
	BatchSizeClicksPercent float64 `yaml:"BATCH_SIZE_CLICKS_PERCENT" env:"BATCH_SIZE_CLICKS_PERCENT"`
	RedisSetClicks         string  `yaml:"REDIS_SET_CLICKS" env:"REDIS_SET_CLICKS" env-default:"clicks:ready"`
}

type KafkaConfig struct {
	KafkaBrokers     []string `yaml:"KAFKA_BROKERS" env:"KAFKA_BROKERS"`
	FlushIntervalSec int      `yaml:"FLUSH_INTERVAL_SEC" env:"FLUSH_INTERVAL_SEC"`

	ImpressionClickFlushIntervalSec int `yaml:"IMPRESSION_CLICK_FLUSH_INTERVAL_SEC" env:"IMPRESSION_CLICK_FLUSH_INTERVAL_SEC" env-default:"30"`

	// Kafka topics
	KafkaTopicOrtb        string `yaml:"KAFKA_TOPIC_ORTB" env:"KAFKA_TOPIC_ORTB" env-default:"ortb"`
	KafkaTopicImpressions string `yaml:"KAFKA_TOPIC_IMPRESSIONS" env:"KAFKA_TOPIC_IMPRESSIONS" env-default:"impressions"`
	KafkaTopicClicks      string `yaml:"KAFKA_TOPIC_CLICKS" env:"KAFKA_TOPIC_CLICKS" env-default:"clicks"`

	// Kafka consumer groups
	KafkaGroupIDOrtb        string `yaml:"KAFKA_GROUP_ID_ORTB" env:"KAFKA_GROUP_ID_ORTB" env-default:"groupIdOrtb"`
	KafkaGroupIDImpressions string `yaml:"KAFKA_GROUP_ID_IMPRESSIONS" env:"KAFKA_GROUP_ID_IMPRESSIONS" env-default:"groupIdImpressions"`
	KafkaGroupIDClicks      string `yaml:"KAFKA_GROUP_ID_CLICKS" env:"KAFKA_GROUP_ID_CLICKS" env-default:"groupIdClicks"`
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
	return []string{".env.local", ".env", "bid-engine.env", "clickhouse-loader.env", "kafka-loader.env", "dsp1.env", "dsp2.env", "dsp3.env", "orchestrator.env", "router.env", "spp-adapter.env", "adm-adapter.env"}
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
