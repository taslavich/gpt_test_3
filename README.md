# RTB Exchange

Платформа для RTB-аукционов с микросервисной архитектурой (Kafka, Redis, ClickHouse loaders, Router, Orchestrator, Bid Engine и SPP Adapter). Репозиторий содержит полноценные манифесты Kubernetes и скрипты для автоматического развёртывания вместе с ingress-nginx, MetalLB и TLS-сертификатами Let's Encrypt.

## Состав

| Компонент         | Назначение |
|-------------------|------------|
| `spp-adapter`     | HTTP API для SSP. Принимает bid-запросы, проксирует win/bill уведомления, общается с Orchestrator и Redis. |
| `router`          | gRPC-сервис, который запрашивает DSP и применяет правила. Должен иметь доступ в интернет по HTTP/HTTPS. |
| `orchestrator`    | gRPC-сервис, определяющий победителя среди ответов DSP. |
| `bid-engine`      | gRPC-сервис, рассчитывающий финальный отклик. |
| `kafka-loader`    | Пишет события из Redis в Kafka. |
| `clickhouse-loader`| Загружает события из Kafka в ClickHouse Cloud. |
| `redis`           | Хранение промежуточных данных (bid request, burl/nurl статусы). |
| `kafka`           | Очередь событий (топик `rtb`). |
| `gateway`         | Внутренний NGINX, через который проходит HTTP-трафик (SPP Adapter, health). |
| `ingress-nginx`   | Единственная внешняя точка входа (порты 80/443). |
| `cert-manager`    | Автоматический выпуск сертификатов Let's Encrypt (при указании почты). |

## Быстрый старт

1. Соберите и загрузите образы в локальный реестр:
   ```bash
   ./build.sh push-local
   ```
2. Запустите полный деплой (установит MetalLB, ingress-nginx, проверит registry, применит манифесты):
   ```bash
   ./deploy.sh all
   ```
3. Для повторной выдачи сертификата/ingress:
   ```bash
   RTB_DOMAIN=rtb.example.com \
   LETSENCRYPT_EMAIL=you@example.com \
   LETSENCRYPT_ENVIRONMENT=prod \
   ./deploy.sh ingress
   ```

> ❗ Для сертификата Let's Encrypt необходим публичный DNS (A/AAAA-записи для `rtb.example.com`, `router.rtb.example.com`, `orchestrator.rtb.example.com`, `bid-engine.rtb.example.com`). В тестовом окружении можно воспользоваться `deploy/setup-domain.sh` для обновления `/etc/hosts`.

## Сетевые потоки и безопасность

* **Входящий HTTP/HTTPS** — только через `ingress-nginx` → `gateway-service`. Внешний IP один, порты 80/443.
* **gRPC доступ** — ingress создаёт отдельные хосты `router.<домен>`, `orchestrator.<домен>`, `bid-engine.<домен>` (порт 443, HTTP/2) и проксирует напрямую на соответствующие сервисы. При `RTB_DOMAIN`, заданном как IP-адрес, эти ingress-ы не создаются (используйте `kubectl port-forward`).
* **Внешние исходящие вызовы** — `router` имеет `NetworkPolicy`, разрешающую HTTP/HTTPS и DNS-запросы во внешние сети, поэтому ответы от DSP и ClickHouse Cloud успешно возвращаются.
* **Kafka/Redis** не получают отдельный внешний IP; для отладки используйте `kubectl port-forward`.

## Примеры HTTP API SPP Adapter

Базовый URL: `https://rtb.example.com/spp-adapter` (замените домен). Для локального теста добавьте `-k` к `curl`, если сертификат самоподписанный/тестовый.

### POST `/bid_v_2_4`

```bash
curl -k -X POST https://rtb.example.com/spp-adapter/bid_v_2_4 \
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

### POST `/bid_v_2_5`

```bash
curl -k -X POST https://rtb.example.com/spp-adapter/bid_v_2_5 \
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

### GET `/nurl`

```bash
curl -k "https://rtb.example.com/spp-adapter/nurl?id=<GLOBAL_ID>&url=$(python3 -c 'import urllib.parse; print(urllib.parse.quote("https://dsp.example.com/win"))')"
```

### GET `/burl`

```bash
curl -k "https://rtb.example.com/spp-adapter/burl?id=<GLOBAL_ID>&url=$(python3 -c 'import urllib.parse; print(urllib.parse.quote("https://dsp.example.com/bill"))')"
```

### GET `/health`

```bash
curl -k https://rtb.example.com/spp-adapter/health -i
```

## Примеры gRPC вызовов

Используйте [`grpcurl`](https://github.com/fullstorydev/grpcurl). Файлы `.proto` лежат в каталоге `proto/`.

### Router (`router.<домен>:443`)

```bash
grpcurl -insecure \
  -import-path proto \
  -proto proto/services/dspRouter.proto \
  -d '{
        "bidRequest": {"id": "req-1", "imp": [{"id": "1", "bidfloor": 0.01}]},
        "sppEndpoint": "https://rtb.example.com/spp-adapter",
        "globalId": "test-123"
      }' \
  router.rtb.example.com:443 \
  dspRouter.DspRouterService/GetBids_V2_4
```

### Orchestrator (`orchestrator.<домен>:443`)

```bash
grpcurl -insecure \
  -import-path proto \
  -proto proto/services/orchestrator.proto \
  -d '{
        "bidRequest": {"id": "req-1", "imp": [{"id": "1", "bidfloor": 0.01}]},
        "sppEndpoint": "https://rtb.example.com/spp-adapter",
        "globalId": "test-123"
      }' \
  orchestrator.rtb.example.com:443 \
  orchestrator.OrchestratorService/getWinnerBid_V2_4
```

### Bid Engine (`bid-engine.<домен>:443`)

```bash
grpcurl -insecure \
  -import-path proto \
  -proto proto/services/bidEngine.proto \
  -d '{
        "bidRequest": {"id": "req-1", "imp": [{"id": "1", "bidfloor": 0.01}]},
        "bidResponses": [{"id": "resp-1", "seat": "dsp-1"}],
        "globalId": "test-123"
      }' \
  bid-engine.rtb.example.com:443 \
  bidEngine.BidEngineService/getWinnerBid_V2_4
```

## Kafka и Redis

### Port-forward

```bash
# Redis
kubectl port-forward -n exchange deployment/redis-deployment 6379:6379

# Kafka (порт клиента)
kubectl port-forward -n exchange svc/kafka-service 9092:9092
```

### Примеры команд

*Kafka*: отправка тестового сообщения в топик `rtb`.
```bash
KAFKA_BROKER=localhost:9092
kafka-console-producer --bootstrap-server $KAFKA_BROKER --topic rtb
> {"type":"test","payload":"demo"}
```

*Redis*: проверка статуса по `GLOBAL_ID`.
```bash
redis-cli -h 127.0.0.1 -p 6379
127.0.0.1:6379> HGETALL test-123
1) "bid_request"
2) "{...json...}"
3) "geo"
4) "US"
5) "result"
6) "SUCCESS"
```

## ClickHouse Loader и внешние сервисы

`clickhouse-loader` использует переменные из `deploy/k8s/configs/clickhouse-loader.yaml` для подключения к ClickHouse Cloud по HTTPS. Router и лоадеры работают в `NetworkPolicy`, разрешающей исходящий трафик на 80/443, поэтому запросы к внешним DSP и облачному ClickHouse доходят и получают ответы. При необходимости укажите собственные креденшелы через `kubectl create secret` (см. `deploy.sh clickhouse`).

