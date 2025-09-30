# RTB Exchange

Платформа RTB-аукционов из набора Go-сервисов (Router, Orchestrator, Bid Engine, SSP Adapter, Kafka/Redis/ClickHouse loaders). Репозитория достаточно, чтобы развернуть весь стек в Kubernetes, включая ingress-nginx, cert-manager и MetalLB.

## Внешний доступ по единственному IP

Все сервисы доступны извне через публичный IP узла Kubernetes (например, `142.93.239.222`). Для этого внешние сервисы переведены в `NodePort`; каждому назначен фиксированный порт из диапазона 30000-32767. Таблица соответствия:

| Сервис | Назначение | Внутренний порт | NodePort |
|--------|------------|-----------------|----------|
| `spp-adapter-service` | HTTP API SSP | 8083 | **30083** |
| `router-service-external` | gRPC Router | 8082 | **30082** |
| `orchestrator-service-external` | gRPC Orchestrator | 8081 | **30081** |
| `bid-engine-service-external` | gRPC Bid Engine | 8080 | **30080** |
| `kafka-loader-service-external` | Loader HTTP/metrics | 8085 | **30085** |
| `clickhouse-loader-service-external` | Loader HTTP/metrics | 8084 | **30084** |
| `kafka-service-external` | Kafka broker | 9092 | **30902** |
| `redis-service-external` | Redis | 6379 | **31379** |

> 📌 Подключение: `http://142.93.239.222:<NodePort>` или `grpc://142.93.239.222:<NodePort>` для gRPC. Внутренние `ClusterIP`-сервисы сохранены и продолжают работать для межсервисного обмена внутри кластера.

## HTTPS и TLS

Для доменного доступа по HTTPS задействован ingress `gateway-ingress` с автоматически выпускаемыми сертификатами Let\'s Encrypt (`gateway-tls`). Развёртывание описано в `deploy/k8s/ingress/*.yaml.tpl`. HTTP-запросы перенаправляются на HTTPS, а для gRPC используется HTTP/2 поверх TLS.

SSL и TLS — это протоколы шифрования, при этом TLS является эволюцией SSL. В проекте используется современный TLS (через nginx ingress). Дополнительно NodePort-порты предоставляют незашифрованный доступ для отладки. Для боевого доступа рекомендуется HTTPS через домен.

## HTTP API SSP Adapter (порт 30083)

Базовый URL: `http://142.93.239.222:30083`

| Метод | Путь | Описание |
|-------|------|----------|
| `POST` | `/bid_v_2_4` | Принимает OpenRTB 2.4 запрос |
| `POST` | `/bid_v_2_5` | Принимает OpenRTB 2.5 запрос |
| `GET` | `/nurl?id=<GLOBAL_ID>&url=<DSP_URL>` | Отправка win-notice (nurl) |
| `GET` | `/burl?id=<GLOBAL_ID>&url=<DSP_URL>` | Отправка bill-notice (burl) |
| `GET` | `/health` | Проверка готовности |

### Примеры запросов

```bash
curl -X POST http://142.93.239.222:30083/bid_v_2_4 \
  -H 'Content-Type: application/json' \
  -d '{
        "id": "req-1",
        "imp": [{
          "id": "1",
          "bidfloor": 0.01,
          "banner": {"w": 300, "h": 250}
        }],
        "site": {"id": "site-1", "domain": "example.com"},
        "device": {
          "ip": "203.0.113.42",
          "ua": "ExampleBrowser/1.0",
          "geo": {}
        },
        "user": {"id": "user-1"}
      }'
```

```bash
curl -X POST http://142.93.239.222:30083/bid_v_2_5 \
  -H 'Content-Type: application/json' \
  -d '{
        "id": "req-2",
        "imp": [{
          "id": "1",
          "metric": [{"type": "viewability", "value": 0.75}],
          "video": {"w": 1920, "h": 1080, "mimes": ["video/mp4"]}
        }],
        "app": {"id": "app-42", "name": "Sample App"},
        "device": {
          "ip": "198.51.100.77",
          "ua": "ExampleMobile/2.0",
          "geo": {}
        },
        "user": {"id": "user-2"}
      }'
```

```bash
curl "http://142.93.239.222:30083/nurl?id=<GLOBAL_ID>&url=$(python3 -c 'import urllib.parse; print(urllib.parse.quote("https://dsp.example.com/win"))')"
```

```bash
curl "http://142.93.239.222:30083/burl?id=<GLOBAL_ID>&url=$(python3 -c 'import urllib.parse; print(urllib.parse.quote("https://dsp.example.com/bill"))')"
```

## gRPC-сервисы (порты 30080-30082)

Команды `grpcurl` можно запускать как по NodePort, так и по HTTPS-домену. Ниже пример прямого подключения по IP.

### Router (`30082`)

```bash
grpcurl -plaintext \
  -import-path proto \
  -proto proto/services/dspRouter.proto \
  -d '{
        "bidRequest": {"id": "req-1", "imp": [{"id": "1", "bidfloor": 0.01}]},
        "sppEndpoint": "http://142.93.239.222:30083",
        "globalId": "test-123"
      }' \
  142.93.239.222:30082 \
  dspRouter.DspRouterService/GetBids_V2_4
```

### Orchestrator (`30081`)

```bash
grpcurl -plaintext \
  -import-path proto \
  -proto proto/services/orchestrator.proto \
  -d '{
        "bidRequest": {"id": "req-1", "imp": [{"id": "1", "bidfloor": 0.01}]},
        "sppEndpoint": "http://142.93.239.222:30083",
        "globalId": "test-123"
      }' \
  142.93.239.222:30081 \
  orchestrator.OrchestratorService/getWinnerBid_V2_4
```

### Bid Engine (`30080`)

```bash
grpcurl -plaintext \
  -import-path proto \
  -proto proto/services/bidEngine.proto \
  -d '{
        "bidRequest": {"id": "req-1", "imp": [{"id": "1", "bidfloor": 0.01}]},
        "bidResponses": [{"id": "resp-1", "seat": "dsp-1"}],
        "globalId": "test-123"
      }' \
  142.93.239.222:30080 \
  bidEngine.BidEngineService/getWinnerBid_V2_4
```

Все методы доступны и для версии 2.5 (`GetBids_V2_5`, `getWinnerBid_V2_5`).

## Kafka и Redis

### Подключение к Kafka (`30902`)

```bash
kafkacat -b 142.93.239.222:30902 -L              # Проверить метаданные
kafkacat -b 142.93.239.222:30902 -t rtb -P <<<'{"event":"example"}'
```

### Подключение к Redis (`31379`)

```bash
redis-cli -h 142.93.239.222 -p 31379 PING
redis-cli -h 142.93.239.222 -p 31379 HGETALL test-global-id
```

## Исходящие соединения

* **Router** — gRPC-сервис, но внутри делает исходящие HTTP(S)-запросы к DSP. Kubernetes по умолчанию разрешает исходящие соединения, поэтому ответы от внешних DSP возвращаются без дополнительной настройки.
* **ClickHouse Loader** — отправляет HTTP(S)-запросы в ClickHouse Cloud. Пока кластер имеет доступ в интернет, соединение устанавливается независимо от страны расположения облака.
* Все остальные сервисы (Kafka loader, Bid Engine и др.) также могут инициировать внешние соединения; ответы вернутся на тот же Pod.

## Тестирование с удалённой машины

1. Убедитесь, что публичный IP узла (`142.93.239.222`) открыт во внешнем firewall (порты 30080-30085, 30902, 31379, 80, 443).
2. Выполните `curl`/`grpcurl`/`kafkacat`/`redis-cli` команды из разделов выше.
3. Для HTTPS используйте домен, привязанный к ingress (`gateway-ingress`), чтобы задействовать TLS.

Для полного списка gRPC методов и типов обращайтесь к файлам в каталоге `proto/services` и `proto/types`.
