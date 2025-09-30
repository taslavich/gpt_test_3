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
- `services/gateway-service.yaml` – сервис типа `ClusterIP`, к которому обращается Ingress.
- `ingress/gateway-ingress.yaml.tpl` – шаблон Ingress, перенаправляющий внешний трафик на gateway.

Gateway остаётся единой точкой маршрутизации для микросервисов, но теперь он доступен только внутри кластера. Внешний IP выдаёт `ingress-nginx-controller` (Service типа `LoadBalancer`), которому MetalLB назначает адрес из пула.

### HTTP/HTTPS-маршрутизация

Ingress принимает входящие соединения только на портах `80` (HTTP) и `443` (HTTPS) и проксирует их в gateway. Доступны следующие префиксы:

| Префикс                | Целевой сервис |
|------------------------|----------------|
| `/bid-engine/`         | bid-engine     |
| `/orchestrator/`       | orchestrator   |
| `/router/`             | router         |
| `/spp-adapter/`        | spp-adapter    |
| `/kafka-loader/`       | kafka-loader   |
| `/clickhouse-loader/`  | clickhouse-loader |

Проверка: `curl http://<domain>/healthz` или `curl https://<domain>/router/health -k` (если сертификат тестовый).

### Доступ к SPP Adapter

`spp-adapter` обслуживается через gateway и ingress. Отдельного `LoadBalancer` для него нет намеренно, чтобы весь входящий трафик проходил по портам `80/443`. Для локальных проверок используйте IP/hostname, который выдаёт `ingress-nginx-controller`:

```bash
./deploy.sh status   # в конце появится блок "External ingress entrypoint"
curl -k https://<ingress-ip>/spp-adapter/health
```

Если настроен домен (`RTB_DOMAIN`), можно обращаться по имени: `https://<домен>/spp-adapter/...`. Для HTTP достаточно заменить схему и порт: `http://<ingress-ip>/spp-adapter/...`.

### gRPC-доступ к Router, Orchestrator и Bid Engine

Для gRPC сервисов создаются отдельные ingress-объекты (`<service>-grpc-ingress`), которые публикуют их на том же внешнем IP. Каждый сервис получает собственный hostname:

| Сервис        | Hostname                        | Порт | Kubernetes Service |
|---------------|---------------------------------|------|--------------------|
| Router        | `router.<домен>`                | 443  | `router-service`   |
| Orchestrator  | `orchestrator.<домен>`          | 443  | `orchestrator-service` |
| Bid Engine    | `bid-engine.<домен>`            | 443  | `bid-engine-service` |

Ingress завершает TLS (секрет `gateway-tls`) и проксирует HTTP/2 прямо на соответствующий сервис.

* Для выпуска сертификата Let's Encrypt требуется реальный DNS с A/AAAA-записями для `RTB_DOMAIN` и перечисленных поддоменов.
* В локальных окружениях можно добавить все записи в `/etc/hosts`.
* Если `RTB_DOMAIN` указывает на IP-адрес, gRPC ingress-ы не создаются автоматически: воспользуйтесь `kubectl port-forward deployment/<service>-deployment <порт>:<порт>`.

Быстрая проверка (после настройки DNS):

```bash
grpcurl -insecure router.$RTB_DOMAIN:443 list
grpcurl -insecure orchestrator.$RTB_DOMAIN:443 list
grpcurl -insecure bid-engine.$RTB_DOMAIN:443 list
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
curl -k https://<domain>/spp-adapter/health
```

## MetalLB (автоматическая установка)

- При запуске `./deploy.sh all` (или любой команды, которая вызывает `auto_setup_before_deploy`) скрипт автоматически скачивает и применяет манифест MetalLB.
- Манифест кэшируется в `deploy/assets/metallb/metallb-native.yaml`, чтобы не обращаться в интернет на каждом запуске.
- Диапазон выдаваемых IP выбирается автоматически на основе `InternalIP` первой ноды: берётся подсеть и диапазон `*.240-*.250`. Чтобы задать диапазон явно, установите переменную окружения `METALLB_IP_RANGE`, например `METALLB_IP_RANGE=192.168.88.240-192.168.88.250`.
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
