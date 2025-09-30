# RTB Exchange

Платформа RTB-аукционов из набора Go-сервисов (Router, Orchestrator, Bid Engine, SSP Adapter, Kafka/Redis/ClickHouse loaders). Репозитория достаточно, чтобы развернуть весь стек в Kubernetes, включая ingress-nginx, cert-manager и MetalLB.

## Единый внешний IP и nginx-gateway

Все внешние подключения проходят через `gateway-service` (тип `LoadBalancer`). Он получает **один** публичный IP-адрес (например, `142.93.239.222`) и проксирует трафик к остальным микросервисам по путям и портам.

| Протокол | Внешний порт | Назначение | Как маршрутизируется |
|----------|--------------|------------|----------------------|
| HTTP     | 80           | Health-check, редирект на HTTPS | `/healthz`, `/` |
| HTTPS    | 443          | Основные HTTP и gRPC вызовы | см. таблицу ниже |
| TCP      | 9092         | Kafka broker | stream-прокси на `kafka-service:9092` |
| TCP      | 6379         | Redis | stream-прокси на `redis-service:6379` |

### Как задаётся внешний IP MetalLB

Пул MetalLB и сервис `gateway-service` используют один и тот же статический адрес. По умолчанию в репозитории он жёстко задан как `142.93.239.222` (см. `deploy.sh` и шаблон `deploy/k8s/services/gateway-service.yaml.tpl`).

Если вам нужно использовать другой IP, измените значение переменной `METALLB_IP_ADDRESS` и заново примените деплой:

```bash
export METALLB_IP_ADDRESS=203.0.113.10
./deploy.sh metallb
./deploy.sh gateway
```

Можно также задать диапазон адресов через `METALLB_IP_RANGE`, если требуется несколько IP, но по умолчанию используется ровно один адрес.

> ℹ️ Если указанный IP уже назначен сетевому интерфейсу узла (например, у вас единственный публичный адрес сервера), `deploy.sh` автоматически пропустит конфигурацию пула MetalLB и создаст `gateway-service` с полем `externalIPs`. В этом режиме Kubernetes начинает слушать запросы на выбранном IP без участия MetalLB, а команда `kubectl get svc gateway-service` сразу покажет внешний адрес без статуса `<pending>`.

### HTTP/HTTPS маршрутизация

nginx внутри gateway разбирает URL-путь и отправляет запрос в нужный сервис. Все HTTP API и gRPC эндпоинты доступны по `http://<EXTERNAL_IP>/...` или `https://<DOMAIN>/...` (при наличии TLS-секрета `gateway-tls`).

| Путь | Назначение | Проксируется в |
|------|------------|----------------|
| `/bidRequest/*` | Основное SSP API (`bid`, `nurl`, `burl`, health) | `spp-adapter-service:8083` |
| `/spp-adapter/*` | Технический доступ к SSP Adapter | `spp-adapter-service:8083` |
| `/bid-engine/*` | REST/metrics Bid Engine | `bid-engine-service:8080` |
| `/orchestrator/*` | REST/metrics Orchestrator | `orchestrator-service:8081` |
| `/router/*` | REST/metrics Router | `router-service:8082` |
| `/kafka-loader/*` | HTTP-интерфейс Kafka loader | `kafka-loader-service:8085` |
| `/clickhouse-loader/*` | HTTP-интерфейс ClickHouse loader | `clickhouse-loader-service:8084` |

#### Примеры HTTP-запросов

```bash
curl -X POST http://142.93.239.222/bidRequest/bid \
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
curl "http://142.93.239.222/bidRequest/nurl?id=<GLOBAL_ID>&url=$(python3 -c 'import urllib.parse; print(urllib.parse.quote("https://dsp.example.com/win"))')"
```

### gRPC через тот же IP

gRPC-вызовы работают поверх HTTP/2 и идут через те же порты 80/443. nginx направляет их в нужный сервис по имени метода (`/dspRouter.DspRouterService/...`, `/orchestrator.OrchestratorService/...`, `/bidEngine.BidEngineService/...`). Для отладки можно использовать `grpcurl` без TLS:

```bash
grpcurl -plaintext \
  -import-path proto \
  -proto proto/services/dspRouter.proto \
  -d '{
        "bidRequest": {"id": "req-1", "imp": [{"id": "1", "bidfloor": 0.01}]},
        "sppEndpoint": "http://142.93.239.222/bidRequest",
        "globalId": "test-123"
      }' \
  142.93.239.222:80 \
  dspRouter.DspRouterService/GetBids_V2_4
```

Для защищённого доступа используйте домен, настроенный на тот же IP, и команду `grpcurl -d ... -proto ... -import-path ... -authority <DOMAIN> <DOMAIN>:443 <Service>/<Method>`.

### Kafka и Redis

TCP-подключения проходят через stream-секцию nginx и попадают в соответствующие `ClusterIP`-сервисы.

```bash
# Kafka (порт 9092 на том же IP)
kafkacat -b 142.93.239.222:9092 -L

# Redis (порт 6379)
redis-cli -h 142.93.239.222 -p 6379 PING
```

## HTTPS и TLS

Для доменного доступа по HTTPS задействован ingress `gateway-ingress` с автоматически выпускаемыми сертификатами Let\'s Encrypt (`gateway-tls`). HTTP-запросы перенаправляются на HTTPS, а для gRPC используется HTTP/2 поверх TLS. При отсутствии домена можно работать по IP через `http://` и незашифрованный gRPC.

## Исходящие соединения

* **Router** — gRPC-сервис, но внутри делает исходящие HTTP(S)-запросы к DSP. Kubernetes по умолчанию разрешает исходящие соединения, поэтому ответы от внешних DSP возвращаются без дополнительной настройки.
* **ClickHouse Loader** — отправляет HTTP(S)-запросы в ClickHouse Cloud. Пока кластер имеет доступ в интернет, соединение устанавливается независимо от страны расположения облака.
* Все остальные сервисы (Kafka loader, Bid Engine и др.) также могут инициировать внешние соединения; ответы вернутся на тот же Pod.

## Тестирование с удалённой машины

1. Убедитесь, что публичный IP узла (`142.93.239.222`) открыт во внешнем firewall (порты 80, 443, 9092, 6379).
2. Выполните `curl`/`grpcurl`/`kafkacat`/`redis-cli` команды из разделов выше.
3. Для HTTPS используйте домен, привязанный к ingress (`gateway-ingress`), чтобы задействовать TLS.

Для полного списка gRPC методов и типов обращайтесь к файлам в каталоге `proto/services` и `proto/types`.
