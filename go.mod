module gitlab.com/twinbid-exchange/RTB-exchange

go 1.25

toolchain go1.25.11

require (
	github.com/ggicci/httpin v0.17.0
	github.com/go-chi/chi v1.5.5
	github.com/go-chi/chi/v5 v5.2.2
	github.com/unrolled/render v1.7.0
	google.golang.org/grpc v1.75.0
	google.golang.org/protobuf v1.36.7
)

require (
	github.com/BurntSushi/toml v1.2.1 // indirect
	github.com/ClickHouse/ch-go v0.68.0 // indirect
	github.com/andybalholm/brotli v1.2.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/go-faster/city v1.0.1 // indirect
	github.com/go-faster/errors v0.7.1 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/labstack/echo/v4 v4.13.1 // indirect
	github.com/labstack/gommon v0.4.2 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/modern-go/concurrent v0.0.0-20180228061459-e0a39a4cb421 // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/paulmach/orb v0.11.1 // indirect
	github.com/pierrec/lz4/v4 v4.1.22 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/rogpeppe/go-internal v1.9.0 // indirect
	github.com/segmentio/asm v1.2.0 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasttemplate v1.2.2 // indirect
	go.opentelemetry.io/otel v1.38.0 // indirect
	go.opentelemetry.io/otel/trace v1.38.0 // indirect
	go.uber.org/atomic v1.9.0 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/crypto v0.42.0 // indirect
	golang.org/x/net v0.44.0 // indirect
	golang.org/x/text v0.29.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250818200422-3122310a409c // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250818200422-3122310a409c // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	olympos.io/encoding/edn v0.0.0-20201019073823-d3554ca0b0a3 // indirect
)

require (
	github.com/ClickHouse/clickhouse-go/v2 v2.40.3
	github.com/fsnotify/fsnotify v1.9.0
	github.com/ggicci/owl v0.7.0 // indirect
	github.com/go-co-op/gocron v1.37.0
	github.com/google/uuid v1.6.0
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.27.2
	github.com/ilyakaznacheev/cleanenv v1.5.0
	github.com/joho/godotenv v1.5.1
	github.com/json-iterator/go v1.1.12
	github.com/lib/pq v1.10.9
	github.com/mileusna/useragent v1.3.5
	github.com/orcaman/concurrent-map/v2 v2.0.1
	github.com/oschwald/maxminddb-golang v1.12.0
	github.com/redis/go-redis/v9 v9.14.0
	github.com/segmentio/kafka-go v0.4.49
	github.com/shopspring/decimal v1.4.0 // indirect
	github.com/yl2chen/cidranger v1.0.2
	go.etcd.io/bbolt v1.4.0
	golang.org/x/sys v0.36.0 // indirect
)

replace github.com/lib/pq => ./internal/third_party/libpqstub

replace go.etcd.io/bbolt => ./internal/third_party/bboltstub
