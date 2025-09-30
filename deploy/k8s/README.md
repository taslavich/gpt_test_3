# Kubernetes деплой RTB Exchange

Документ описывает порядок развертывания проекта в Kubernetes, настройку внешнего шлюза и подготовку DNS/доменов.

## Компоненты

- **Redis** – деплоймент + service (`redis-service`).
- **Kafka (KRaft)** – statefulset + headless service (`kafka-headless`) и клиентский service (`kafka-service`).
- **ClickHouse/Kafka loaders** – отдельные деплойменты с ClusterIP сервисами.
- **Микросервисы** – `bid-engine`, `orchestrator`, `router`, `spp-adapter`.
- **Gateway** – NGINX-балансировщик, который принимает внешние HTTP(S) вызовы и проксирует их в сервисы по портам/путям.
- **Ingress (ingress-nginx)** – внешний слой, через который проходит весь HTTP(S)-трафик; устанавливается автоматически скриптом `deploy.sh`.

## Gateway и входящий трафик

Файлы:

- `configs/gateway-config.yaml` – конфигурация NGINX.
- `deployments/gateway-deployment.yaml` – деплоймент с 2 репликами и health-чеками.
- `services/gateway-service.yaml.tpl` – шаблон сервиса. По умолчанию это `LoadBalancer` с фиксированным IP, но если адрес уже занят узлом, он автоматически переключается на `ClusterIP` + `externalIPs`.
- `ingress/gateway-ingress.yaml.tpl` – шаблон Ingress, перенаправляющий внешний трафик на gateway.

Gateway остаётся единой точкой маршрутизации для микросервисов. Когда MetalLB доступен, он получает внешний IP как `LoadBalancer`. Если выбранный адрес уже закреплён за узлом, `deploy.sh` добавляет `externalIPs`, и Kubernetes начинает слушать соединения на этом IP напрямую.

### HTTP/HTTPS-маршрутизация

Ingress принимает входящие соединения только на портах `80` (HTTP) и `443` (HTTPS) и проксирует их в gateway. Доступны следующие префиксы:

| Префикс                | Целевой сервис |
|------------------------|----------------|
| `/bid-engine/`         | bid-engine     |
| `/orchestrator/`       | orchestrator   |
| `/router/`             | router         |
| `/bidRequest/`         | spp-adapter (основной путь) |
| `/spp-adapter/`        | spp-adapter (обратная совместимость) |
| `/kafka-loader/`       | kafka-loader   |
| `/clickhouse-loader/`  | clickhouse-loader |

Проверка: `curl http://<domain>/healthz` или `curl https://<domain>/bidRequest/health -k` (если сертификат тестовый).

### Доступ к SPP Adapter

`spp-adapter` обслуживается через gateway и ingress. Отдельного `LoadBalancer` для него нет намеренно, чтобы весь входящий трафик проходил по портам `80/443`. Для локальных проверок используйте IP/hostname, который выдаёт `ingress-nginx-controller`:

```bash
./deploy.sh status   # в конце появится блок "External ingress entrypoint"
curl -k https://<ingress-ip>/bidRequest/health
```

Если настроен домен (`RTB_DOMAIN`), можно обращаться по имени: `https://<домен>/bidRequest/...`. Для HTTP достаточно заменить схему и порт: `http://<ingress-ip>/bidRequest/...`.

### gRPC-доступ к Router, Orchestrator и Bid Engine

Отдельные ingress-объекты больше не требуются: gRPC-запросы принимаются тем же ingress-nginx по HTTPS (порт 443) и маршрутизируются по путям `/<package>.<Service>/<Method>`. Достаточно одной DNS-записи `RTB_DOMAIN`.

* Для Let's Encrypt нужен публичный DNS для самого домена (`RTB_DOMAIN`). Поддомены не требуются.
* В локальной среде можно добавить запись в `/etc/hosts` (см. `deploy/setup-domain.sh`).
* Для plaintext gRPC используйте `kubectl port-forward`, если ingress доступен только по TLS.

Проверка через `grpcurl` (замените домен на свой):

```bash
grpcurl -insecure -authority $RTB_DOMAIN $RTB_DOMAIN:443 list dspRouter.DspRouterService
grpcurl -insecure -authority $RTB_DOMAIN $RTB_DOMAIN:443 list orchestrator.OrchestratorService
grpcurl -insecure -authority $RTB_DOMAIN $RTB_DOMAIN:443 list bidEngine.BidEngineService
```

### Kafka и Redis из внешней сети

Для Kafka/Redis внешние IP не выдаются: требования безопасности предписывают оставлять единственную точку входа (ingress) на портах `80/443`. Для отладки используйте port-forward:

```bash
# Redis
kubectl port-forward -n exchange deployment/redis-deployment 6379:6379

# Kafka (порт клиента 9092)
kubectl port-forward -n exchange svc/kafka-service 9092:9092
```

После запуска port-forward клиенты могут подключаться к `localhost:<порт>`.

## Настройка домена

1. Выполните `./deploy/setup-domain.sh <domain>` – скрипт выведет IP или hostname балансировщика и подсказки.
2. Чтобы автоматически дописать `/etc/hosts`, используйте `./deploy/setup-domain.sh <domain> --apply` (потребуется `sudo`).
3. Для боевого DNS добавьте A/AAAA-запись у провайдера, указывая на полученный IP.

По умолчанию скрипт читает адрес сервиса `ingress-nginx/ingress-nginx-controller`. Переопределить можно переменными `K8S_NAMESPACE` и `SERVICE_NAME` (есть и обратная совместимость с `gateway-service` через `FALLBACK_*`).

После обновления DNS проверьте доступность:

```bash
curl http://<domain>/healthz
curl -k https://<domain>/bidRequest/health
```

## MetalLB (автоматическая установка)

- При запуске `./deploy.sh all` (или любой команды, которая вызывает `auto_setup_before_deploy`) скрипт скачивает и применяет манифест MetalLB.
- Манифест кэшируется в `deploy/assets/metallb/metallb-native.yaml`, чтобы не обращаться в интернет на каждом запуске.
- Внешний IP задаётся переменной `METALLB_IP_ADDRESS` (по умолчанию `142.93.239.222`) и используется как единственный адрес пула. При необходимости можно указать диапазон через `METALLB_IP_RANGE`.
- Если этот IP уже назначен сетевому интерфейсу узла (типичный случай с единственным публичным адресом), `deploy.sh` пропустит настройку пула MetalLB и переподнимет `gateway-service` с полем `externalIPs`, чтобы `kubectl get svc` мгновенно показал готовый внешний адрес без статуса `<pending>`.
- Для повторной установки или обновления достаточно запустить `./deploy.sh metallb`.
- Если MetalLB устанавливать не требуется (например, в managed-кластере уже есть внешний балансировщик), установите `SKIP_METALLB_INSTALL=1` перед запуском скрипта.

## Сценарии деплоя

Основной скрипт – `deploy.sh`. Ключевые команды:

```bash
./deploy.sh all        # Полный деплой
./deploy.sh services   # Только микросервисы
./deploy.sh gateway    # Только внешний шлюз
./deploy.sh test       # Проверка доступности через балансировщик
```

Скрипт автоматически применяет ConfigMap/Secret, ожидает readiness и выводит статус.

> ℹ️ Значения подключения к ClickHouse Cloud для `clickhouse-loader` уже прописаны в ConfigMap `clickhouse-loader-config` и автоматически попадают в переменные окружения контейнера. При необходимости пересоздать Kubernetes Secret с учётными данными можно запустить `CONFIGURE_CLICKHOUSE_CLOUD=1 ./deploy.sh all` либо отдельную команду `./deploy.sh clickhouse`.

### GeoIP база для SPP Adapter

`spp-adapter` требует файл `GeoIP2_City.mmdb`. Перед сборкой убедитесь, что база лежит в корне репозитория (или передана через
build-аргумент `GEOIP_DB_FILE`). Dockerfile копирует её внутрь образа по пути `/var/lib/geoip/GeoIP2_City.mmdb`, поэтому
дополнительные Kubernetes Secret'ы для GeoIP не нужны: ConfigMap просто прокидывает путь через переменную окружения
`GEO_IP_DB_PATH`.

## Egress для Router

Файл `configs/router-egress-policy.yaml` задаёт `NetworkPolicy`, разрешающую `router` обращаться к внешним HTTP/HTTPS ресурсам (порт 80/443) и к DNS (порт 53). Если в кластере не используется контроллер сетевых политик, манифест не оказывает влияния, но обеспечивает совместимость с кластерами, где политики включены.

## HTTPS и Let's Encrypt

`deploy.sh` умеет автоматически устанавливать `ingress-nginx` и (при наличии почты) `cert-manager`, генерируя сертификат Let's Encrypt для домена.

Переменные окружения:

- `RTB_DOMAIN` – DNS-имя, по которому будет доступен кластер (пример: `rtb.example.com`).
- `LETSENCRYPT_EMAIL` – почта владельца сертификата. Если не задана, запрос в Let's Encrypt не отправляется и используется существующий secret `gateway-tls` (можно оставить self-signed).
- `LETSENCRYPT_ENVIRONMENT` – `staging` (по умолчанию) или `prod`. Staging безопасно для тестов, `prod` запрашивает боевой сертификат.

Пример:

```bash
RTB_DOMAIN=rtb.example.com \
LETSENCRYPT_EMAIL=user@example.com \
LETSENCRYPT_ENVIRONMENT=prod \
./deploy.sh ingress
```

Скрипт применит:

1. `ClusterIssuer` с нужным ACME-эндпоинтом.
2. `Certificate`, создающий secret `gateway-tls` с полученным сертификатом.
3. Ingress, который использует этот secret и перенаправляет HTTP→HTTPS.

Для локальных окружений или доменов вида `*.local` Let's Encrypt пропускается, но Ingress всё равно применится и будет использовать секрет `gateway-tls` из `deploy/k8s/secrets`.

## Примечания

- `deploy.sh` при запуске `auto_setup_before_deploy` устанавливает и обновляет MetalLB и ingress-nginx (можно отключить через `SKIP_METALLB_INSTALL=1` и `SKIP_INGRESS_INSTALL=1`).
- cert-manager ставится только когда указан `LETSENCRYPT_EMAIL`; отключить можно флагом `SKIP_CERT_MANAGER_INSTALL=1`.
- Для локальных кластеров (k3s/Minikube) MetalLB продолжит раздавать IP-адреса ingress-контроллеру, тесты выполняйте по `http(s)://<выданный-IP>/...`.
- Скрипт `deploy.sh test` использует балансировщик и проверяет `/health` основных сервисов.
