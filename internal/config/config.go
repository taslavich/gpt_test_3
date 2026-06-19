package config

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/ilyakaznacheev/cleanenv"
	"github.com/joho/godotenv"
)

type MapStringToString map[string]string

func (m *MapStringToString) SetValue(value string) error {
	*m = make(MapStringToString)
	if value == "" {
		return nil
	}

	pairs := strings.Split(value, ",")
	for _, pair := range pairs {
		idx := strings.Index(pair, "|")
		if idx == -1 {
			continue
		}

		key := strings.TrimSpace(pair[:idx])
		valueStr := strings.TrimSpace(pair[idx+1:])
		if key == "" || valueStr == "" {
			continue
		}
		(*m)[key] = valueStr
	}
	return nil
}

type HttpServer struct {
	Host string `yaml:"HTTP_HOSTNAME" env:"HTTP_HOSTNAME" env-default:"0.0.0.0"`
	Port uint16 `yaml:"HTTP_PORT" env:"HTTP_PORT" env-default:"8055"`
}

type ClickhouseConfig struct {
	Username string `yaml:"CLICKHOUSE_USERNAME" env:"CLICKHOUSE_USERNAME"`
	Password string `yaml:"CLICKHOUSE_PASSWORD" env:"CLICKHOUSE_PASSWORD"`
	Host     string `yaml:"CLICKHOUSE_HOST" env:"CLICKHOUSE_HOST" env-default:"hntzp0jsnf.europe-west4.gcp.clickhouse.cloud"`
	Port     string `yaml:"CLICKHOUSE_PORT" env:"CLICKHOUSE_PORT" env-default:"9440"`
	Database string `yaml:"CLICKHOUSE_DB" env:"CLICKHOUSE_DB" env-default:"rtb"`
}

type WmAPIConfig struct {
	HttpServer
	ClickhouseConfig

	FactClicksTable      string `yaml:"CLICKHOUSE_FACT_CLICKS_TABLE" env:"CLICKHOUSE_FACT_CLICKS_TABLE" env-default:"fact_clicks"`
	FactImpressionsTable string `yaml:"CLICKHOUSE_FACT_IMPRESSIONS_TABLE" env:"CLICKHOUSE_FACT_IMPRESSIONS_TABLE" env-default:"fact_impressions"`

	SspPopAdlFeeds MapStringToString `yaml:"SSP_POP_ADL_FEEDS" env:"SSP_POP_ADL_FEEDS"`
	SspPopMcFeeds  MapStringToString `yaml:"SSP_POP_MC_FEEDS" env:"SSP_POP_MC_FEEDS"`
	SspBanAdlFeeds MapStringToString `yaml:"SSP_BAN_ADL_FEEDS" env:"SSP_BAN_ADL_FEEDS"`
	SspBanMcFeeds  MapStringToString `yaml:"SSP_BAN_MC_FEEDS" env:"SSP_BAN_MC_FEEDS"`
	SspNatAdlFeeds MapStringToString `yaml:"SSP_NAT_ADL_FEEDS" env:"SSP_NAT_ADL_FEEDS"`
	SspNatMcFeeds  MapStringToString `yaml:"SSP_NAT_MC_FEEDS" env:"SSP_NAT_MC_FEEDS"`
	SspIppAdlFeeds MapStringToString `yaml:"SSP_IPP_ADL_FEEDS" env:"SSP_IPP_ADL_FEEDS"`
	SspIppMcFeeds  MapStringToString `yaml:"SSP_IPP_MC_FEEDS" env:"SSP_IPP_MC_FEEDS"`
}

func getEnvFileNames() []string {
	return []string{".env.local", ".env", "wm-api.env", "spp-adapter.env"}
}

func LoadConfig(ctx context.Context) (*WmAPIConfig, error) {
	for _, fileName := range getEnvFileNames() {
		if err := godotenv.Load(fileName); err != nil {
			log.Printf("error loading %s fileName : %v", fileName, err)
		}
	}

	var cfg WmAPIConfig
	if err := cleanenv.ReadEnv(&cfg); err != nil {
		return nil, fmt.Errorf("read env: %w", err)
	}

	return &cfg, nil
}
