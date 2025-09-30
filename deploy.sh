#!/bin/bash

set -e  # Останавливаться при ошибках

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
K8S_DIR="$SCRIPT_DIR/deploy/k8s"
ASSETS_DIR="$SCRIPT_DIR/deploy/assets"
NAMESPACE="exchange"

METALLB_VERSION="${METALLB_VERSION:-v0.13.12}"
METALLB_MANIFEST_URL="${METALLB_MANIFEST_URL:-https://raw.githubusercontent.com/metallb/metallb/$METALLB_VERSION/config/manifests/metallb-native.yaml}"
METALLB_MANIFEST_PATH="${METALLB_MANIFEST_PATH:-$ASSETS_DIR/metallb/metallb-native.yaml}"
METALLB_IP_POOL_NAME="${METALLB_IP_POOL_NAME:-rtb-exchange-pool}"
METALLB_L2_ADVERTISEMENT_NAME="${METALLB_L2_ADVERTISEMENT_NAME:-rtb-exchange-l2}"

CERT_MANAGER_VERSION="${CERT_MANAGER_VERSION:-v1.14.4}"
CERT_MANAGER_MANIFEST_URL="${CERT_MANAGER_MANIFEST_URL:-https://github.com/cert-manager/cert-manager/releases/download/$CERT_MANAGER_VERSION/cert-manager.yaml}"
CERT_MANAGER_MANIFEST_PATH="${CERT_MANAGER_MANIFEST_PATH:-$ASSETS_DIR/cert-manager/cert-manager.yaml}"
CERT_MANAGER_NAMESPACE="${CERT_MANAGER_NAMESPACE:-cert-manager}"

INGRESS_NGINX_VERSION="${INGRESS_NGINX_VERSION:-controller-v1.10.1}"
INGRESS_NGINX_MANIFEST_URL="${INGRESS_NGINX_MANIFEST_URL:-https://raw.githubusercontent.com/kubernetes/ingress-nginx/$INGRESS_NGINX_VERSION/deploy/static/provider/cloud/deploy.yaml}"
INGRESS_NGINX_MANIFEST_PATH="${INGRESS_NGINX_MANIFEST_PATH:-$ASSETS_DIR/ingress-nginx/deploy.yaml}"
DEFAULT_INGRESS_CLASS="${DEFAULT_INGRESS_CLASS:-nginx}"

echo "=== RTB Exchange Deployment ==="

usage() {
    echo "Usage: $0 [all|configs|redis|kafka|clickhouse|loaders|services|gateway|ingress|metallb|status|logs|test|clean|destroy]"
    echo "  all         - Full deployment (default)"
    echo "  configs     - Apply only configs"
    echo "  redis       - Deploy only Redis"
    echo "  kafka       - Deploy only Kafka cluster"
    echo "  clickhouse  - Configure ClickHouse Cloud connection"
    echo "  loaders     - Deploy only Kafka and ClickHouse loaders"
    echo "  services    - Deploy only microservices"
    echo "  gateway     - Deploy only external gateway"
    echo "  ingress     - Deploy only ingress"
    echo "  metallb     - Install/refresh MetalLB load balancer"
    echo "  status      - Check deployment status"
    echo "  logs        - Show logs"
    echo "  test        - Test endpoints"
    echo "  clean       - Remove all resources but keep namespace"
    echo "  destroy     - COMPLETELY remove everything including namespace"
}

ensure_metallb_manifest() {
    local manifest_dir
    manifest_dir="$(dirname "$METALLB_MANIFEST_PATH")"

    if [ -f "$METALLB_MANIFEST_PATH" ]; then
        return 0
    fi

    if [ ! -d "$manifest_dir" ]; then
        mkdir -p "$manifest_dir"
    fi

    if ! command -v curl >/dev/null 2>&1; then
        echo "❌ curl is required to download MetalLB manifest. Install curl or place manifest at $METALLB_MANIFEST_PATH manually"
        return 1
    fi

    echo "🌐 Downloading MetalLB manifest ($METALLB_VERSION)..."
    if ! curl -fsSL "$METALLB_MANIFEST_URL" -o "$METALLB_MANIFEST_PATH.tmp"; then
        echo "❌ Failed to download MetalLB manifest from $METALLB_MANIFEST_URL"
        echo "   Please download it manually and save as $METALLB_MANIFEST_PATH"
        rm -f "$METALLB_MANIFEST_PATH.tmp"
        return 1
    fi

    mv "$METALLB_MANIFEST_PATH.tmp" "$METALLB_MANIFEST_PATH"
    echo "✅ MetalLB manifest cached at $METALLB_MANIFEST_PATH"
}

ensure_ingress_manifest() {
    local manifest_path="$INGRESS_NGINX_MANIFEST_PATH"
    local manifest_dir
    manifest_dir="$(dirname "$manifest_path")"

    if [ -f "$manifest_path" ]; then
        return 0
    fi

    mkdir -p "$manifest_dir"

    if ! command -v curl >/dev/null 2>&1; then
        echo "❌ curl is required to download ingress-nginx manifest. Install curl or place manifest at $manifest_path manually"
        return 1
    fi

    echo "🌐 Downloading ingress-nginx manifest ($INGRESS_NGINX_VERSION)..."
    if ! curl -fsSL "$INGRESS_NGINX_MANIFEST_URL" -o "$manifest_path.tmp"; then
        echo "❌ Failed to download ingress-nginx manifest from $INGRESS_NGINX_MANIFEST_URL"
        rm -f "$manifest_path.tmp"
        return 1
    fi

    mv "$manifest_path.tmp" "$manifest_path"
    echo "✅ ingress-nginx manifest cached at $manifest_path"
}

ensure_cert_manager_manifest() {
    local manifest_path="$CERT_MANAGER_MANIFEST_PATH"
    local manifest_dir
    manifest_dir="$(dirname "$manifest_path")"

    if [ -f "$manifest_path" ]; then
        return 0
    fi

    mkdir -p "$manifest_dir"

    if ! command -v curl >/dev/null 2>&1; then
        echo "❌ curl is required to download cert-manager manifest. Install curl or place manifest at $manifest_path manually"
        return 1
    fi

    echo "🌐 Downloading cert-manager manifest ($CERT_MANAGER_VERSION)..."
    if ! curl -fsSL "$CERT_MANAGER_MANIFEST_URL" -o "$manifest_path.tmp"; then
        echo "❌ Failed to download cert-manager manifest from $CERT_MANAGER_MANIFEST_URL"
        rm -f "$manifest_path.tmp"
        return 1
    fi

    mv "$manifest_path.tmp" "$manifest_path"
    echo "✅ cert-manager manifest cached at $manifest_path"
}

ensure_ingress_nginx() {
    if [[ "${SKIP_INGRESS_INSTALL:-0}" == "1" ]]; then
        echo "⏭️  Skipping ingress-nginx installation (SKIP_INGRESS_INSTALL=1)"
        return 0
    fi

    if ! command -v kubectl >/dev/null 2>&1; then
        echo "❌ kubectl is required to install ingress-nginx"
        return 1
    fi

    ensure_ingress_manifest || return 1

    echo "📦 Applying ingress-nginx manifests..."
    kubectl apply -f "$INGRESS_NGINX_MANIFEST_PATH"

    echo "⏳ Waiting for ingress-nginx controller to be ready..."
    kubectl rollout status deployment/ingress-nginx-controller -n ingress-nginx --timeout=240s
    echo "✅ ingress-nginx is ready"
}

ensure_cert_manager() {
    if [[ "${SKIP_CERT_MANAGER_INSTALL:-0}" == "1" ]]; then
        echo "⏭️  Skipping cert-manager installation (SKIP_CERT_MANAGER_INSTALL=1)"
        return 0
    fi

    if ! command -v kubectl >/dev/null 2>&1; then
        echo "❌ kubectl is required to install cert-manager"
        return 1
    fi

    ensure_cert_manager_manifest || return 1

    echo "📦 Applying cert-manager manifests..."
    kubectl apply -f "$CERT_MANAGER_MANIFEST_PATH"

    echo "⏳ Waiting for cert-manager components..."
    kubectl rollout status deployment/cert-manager -n "$CERT_MANAGER_NAMESPACE" --timeout=240s
    kubectl rollout status deployment/cert-manager-webhook -n "$CERT_MANAGER_NAMESPACE" --timeout=240s
    kubectl rollout status deployment/cert-manager-cainjector -n "$CERT_MANAGER_NAMESPACE" --timeout=240s
    echo "✅ cert-manager is ready"
}

apply_template() {
    local template_path="$1"

    if [ ! -f "$template_path" ]; then
        echo "❌ Template not found: $template_path"
        return 1
    fi

    if ! command -v envsubst >/dev/null 2>&1; then
        echo "❌ envsubst is required to render templates. Install gettext or provide manifests manually."
        return 1
    fi

    local tmp_file
    tmp_file=$(mktemp)
    envsubst < "$template_path" > "$tmp_file"
    kubectl apply -f "$tmp_file"
    rm -f "$tmp_file"
}

resolve_ingress_host() {
    local ingress_service="${INGRESS_SERVICE_NAME:-ingress-nginx-controller}"
    local ingress_namespace="${INGRESS_NAMESPACE:-ingress-nginx}"
    local lb_host=""

    if command -v kubectl >/dev/null 2>&1; then
        lb_host=$(kubectl get svc "$ingress_service" -n "$ingress_namespace" -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || true)
        if [ -z "$lb_host" ] || [ "$lb_host" = "<no value>" ]; then
            lb_host=$(kubectl get svc "$ingress_service" -n "$ingress_namespace" -o jsonpath='{.status.loadBalancer.ingress[0].hostname}' 2>/dev/null || true)
        fi
    fi

    if [ -z "$lb_host" ] || [ "$lb_host" = "<no value>" ]; then
        lb_host="${RTB_DOMAIN:-}"
    fi

    if [ -z "$lb_host" ]; then
        lb_host="127.0.0.1"
    fi

    echo "$lb_host"
}

print_ingress_usage() {
    if ! command -v kubectl >/dev/null 2>&1; then
        return
    fi

    local ingress_host
    ingress_host=$(resolve_ingress_host)

    echo ""
    echo "=== External ingress entrypoint ==="
    echo "HTTP : http://$ingress_host/"
    echo "HTTPS: https://$ingress_host/"
    echo "Bid requests (v2.5): https://$ingress_host/bidRequest/bid"
    echo "Health check: https://$ingress_host/bidRequest/health"
    local domain="${RTB_DOMAIN:-rtb.local}"

    if [[ "$domain" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        echo "gRPC (router/orchestrator/bid-engine): требуется DNS-имя для TLS. При IP используйте 'kubectl port-forward' или grpcurl --insecure"
    else
        echo "gRPC (router/orchestrator/bid-engine): ${domain}:443 (HTTP/2, путь /<package>.<Service>/<Method>)"
    fi
    echo ""
    echo "ℹ️  Все входящие HTTP/HTTPS запросы проходят через ingress-nginx по портам 80/443."
}

detect_metallb_range() {
    if [ -n "${METALLB_IP_RANGE:-}" ]; then
        echo "$METALLB_IP_RANGE"
        return 0
    fi

    if ! command -v kubectl >/dev/null 2>&1; then
        return 1
    fi

    local node_ip
    node_ip=$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}' 2>/dev/null || true)

    if [[ "$node_ip" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
        local prefix
        prefix="${BASH_REMATCH[1]}.${BASH_REMATCH[2]}.${BASH_REMATCH[3]}"
        echo "${prefix}.240-${prefix}.250"
    fi
}

apply_metallb_config() {
    local ip_range
    ip_range=$(detect_metallb_range)

    if [ -z "$ip_range" ]; then
        echo "⚠️  Could not detect IP range for MetalLB automatically."
        echo "   Set METALLB_IP_RANGE (e.g. 192.168.1.240-192.168.1.250) and re-run the script."
        return 1
    fi

    echo "📝 Configuring MetalLB IP pool with range $ip_range"
    cat <<EOF | kubectl apply -f -
apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata:
  name: ${METALLB_IP_POOL_NAME}
  namespace: metallb-system
spec:
  addresses:
    - $ip_range
---
apiVersion: metallb.io/v1beta1
kind: L2Advertisement
metadata:
  name: ${METALLB_L2_ADVERTISEMENT_NAME}
  namespace: metallb-system
spec:
  ipAddressPools:
    - ${METALLB_IP_POOL_NAME}
EOF
}

ensure_metallb() {
    if [[ "${SKIP_METALLB_INSTALL:-0}" == "1" ]]; then
        echo "⏭️  Skipping MetalLB installation (SKIP_METALLB_INSTALL=1)"
        return 0
    fi

    if ! command -v kubectl >/dev/null 2>&1; then
        echo "❌ kubectl is required to install MetalLB"
        return 1
    fi

    ensure_metallb_manifest || return 1

    if kubectl get namespace metallb-system >/dev/null 2>&1; then
        echo "✅ MetalLB namespace already exists"
    else
        echo "🛠️  Installing MetalLB components..."
    fi

    echo "📦 Applying MetalLB core manifests..."
    kubectl apply -f "$METALLB_MANIFEST_PATH"

    echo "⏳ Waiting for MetalLB CRDs to be established..."
    kubectl wait --for=condition=Established crd/ipaddresspools.metallb.io --timeout=120s >/dev/null
    kubectl wait --for=condition=Established crd/l2advertisements.metallb.io --timeout=120s >/dev/null

    echo "⏳ Waiting for MetalLB controller..."
    kubectl rollout status deployment/controller -n metallb-system --timeout=180s
    echo "⏳ Waiting for MetalLB speaker..."
    kubectl rollout status daemonset/speaker -n metallb-system --timeout=180s

    apply_metallb_config || return 1

    echo "✅ MetalLB is ready"
}

# Функция для настройки k3s
setup_k3s_registry() {
    echo "🔧 Configuring k3s for local registry..."
    
    # Проверяем, работает ли k3s
    if ! systemctl is-active k3s >/dev/null 2>&1; then
        echo "ℹ️ k3s is not running, skipping k3s configuration"
        return 0
    fi
    
    # Проверяем, не настроен ли уже k3s
    if [ -f "/etc/rancher/k3s/registries.yaml" ]; then
        if grep -q "localhost:5000" /etc/rancher/k3s/registries.yaml; then
            echo "✅ k3s registry already configured"
            return 0
        fi
    fi
    
    # Создаем backup текущих конфигов
    local backup_dir="/tmp/rtb-k3s-backup-$(date +%Y%m%d-%H%M%S)"
    mkdir -p "$backup_dir"
    
    echo "📦 Creating backup in $backup_dir..."
    
    # Backup существующих конфигов
    if [ -f "/etc/docker/daemon.json" ]; then
        cp /etc/docker/daemon.json "$backup_dir/docker-daemon-backup.json"
        echo "✅ Docker daemon config backed up"
    fi
    
    if [ -f "/etc/rancher/k3s/registries.yaml" ]; then
        cp /etc/rancher/k3s/registries.yaml "$backup_dir/k3s-registries-backup.yaml"
        echo "✅ k3s registries config backed up"
    fi
    
    # 1. Настраиваем Docker daemon
    echo "📝 Configuring Docker daemon..."
    sudo mkdir -p /etc/docker
    sudo tee /etc/docker/daemon.json <<EOF
{
  "insecure-registries": ["localhost:5000", "127.0.0.1:5000", "registry.local:5000"]
}
EOF
    
    # Перезапускаем Docker
    echo "🔄 Restarting Docker..."
    sudo systemctl restart docker
    sleep 3
    
    # 2. Настраиваем k3s
    echo "📝 Configuring k3s registry settings..."
    sudo mkdir -p /etc/rancher/k3s
    sudo tee /etc/rancher/k3s/registries.yaml <<EOF
mirrors:
  "localhost:5000":
    endpoint:
      - "http://localhost:5000"
  "127.0.0.1:5000":
    endpoint:
      - "http://127.0.0.1:5000"
EOF
    
    # Перезапускаем k3s
    echo "🔄 Restarting k3s..."
    sudo systemctl stop k3s
    sleep 5
    sudo systemctl start k3s
    
    # Ждем пока k3s запустится
    echo "⏳ Waiting for k3s to start..."
    for i in {1..30}; do
        if kubectl get nodes >/dev/null 2>&1; then
            echo "✅ k3s is running"
            break
        fi
        sleep 2
        if [ $i -eq 30 ]; then
            echo "❌ k3s failed to start within 60 seconds"
            return 1
        fi
    done
    
    echo "✅ k3s registry configuration completed successfully!"
    echo "📋 Backup files saved in: $backup_dir"
}

# Автоматическая настройка перед деплоем
auto_setup_before_deploy() {
    echo "🔧 Auto-setting up environment for deployment..."

    # Настраиваем k3s если он установлен
    setup_k3s_registry

    # Проверяем и запускаем registry
    if ! curl -s http://localhost:5000/v2/_catalog >/dev/null; then
        echo "🚀 Starting local registry..."
        local build_script="$SCRIPT_DIR/build.sh"
        if [ -f "$build_script" ]; then
            "$build_script" registry-start
        else
            echo "❌ build.sh not found. Please start registry manually."
            return 1
        fi
    else
        echo "✅ Local registry is running"
    fi

    # Устанавливаем MetalLB для автоматической выдачи внешних IP
    ensure_metallb

    # Проверяем, что образы существуют в registry
    echo "🔍 Checking if images are available in registry..."
    local images_missing=0
    local services=("bid-engine" "orchestrator" "router" "spp-adapter" "kafka-loader" "clickhouse-loader")
    
    for service in "${services[@]}"; do
        if ! curl -s http://localhost:5000/v2/exchange/$service/tags/list | grep -q "latest"; then
            echo "❌ Image for $service not found in registry"
            images_missing=1
        fi
    done
    
    if [ $images_missing -eq 1 ]; then
        echo "⚠️ Some images missing in registry. Please run './build.sh push-local' first."
        return 1
    fi
    
    echo "✅ All images available in registry"
}

# Функция очистки ресурсов
clean_resources() {
    echo "🧹 Cleaning all resources in namespace $NAMESPACE..."
    
    if kubectl get namespace "$NAMESPACE" >/dev/null 2>&1; then
        kubectl delete all --all -n "$NAMESPACE"
        kubectl delete configmap,secret,ingress --all -n "$NAMESPACE"
        echo "✅ All resources cleaned in namespace $NAMESPACE"
    else
        echo "ℹ️ Namespace $NAMESPACE does not exist"
    fi
}

# Функция полного удаления
destroy_namespace() {
    echo "💥 COMPLETELY destroying namespace $NAMESPACE..."
    
    if kubectl get namespace "$NAMESPACE" >/dev/null 2>&1; then
        kubectl delete namespace "$NAMESPACE"
        echo "✅ Namespace $NAMESPACE destroyed"
        
        # Также останавливаем registry
        local build_script="$SCRIPT_DIR/build.sh"
        if [ -f "$build_script" ]; then
            echo "🛑 Stopping registry..."
            "$build_script" registry-stop
        fi
    else
        echo "ℹ️ Namespace $NAMESPACE does not exist"
    fi
}

# Функция деплоя конфигов
deploy_configs() {
    echo "📄 Deploying configs..."

    if [ -f "$K8S_DIR/namespace.yaml" ]; then
        kubectl apply -f "$K8S_DIR/namespace.yaml"
    fi

    if [ -d "$K8S_DIR/configs" ]; then
        kubectl apply -f "$K8S_DIR/configs/"
        echo "✅ Configs deployed"
    else
        echo "❌ Configs directory not found: $K8S_DIR/configs/"
        return 1
    fi

    if [ -d "$K8S_DIR/secrets" ]; then
        kubectl apply -f "$K8S_DIR/secrets/"
        echo "✅ Secrets deployed"
    fi
}

# Функция деплоя Redis
deploy_redis() {
    echo "🔴 Deploying Redis..."

    local redis_files=(
        "$K8S_DIR/deployments/redis-deployment.yaml"
        "$K8S_DIR/services/redis-service.yaml"
        "$K8S_DIR/services/redis-service-external.yaml"
    )

    for file in "${redis_files[@]}"; do
        if [ -f "$file" ]; then
            kubectl apply -f "$file"
        else
            echo "❌ Redis file not found: $file"
            return 1
        fi
    done

    echo "⏳ Waiting for Redis to be ready..."
    kubectl rollout status deployment/redis-deployment -n "$NAMESPACE" --timeout=120s
    echo "✅ Redis deployed and ready"
}

# Функция деплоя Kafka
deploy_kafka() {
    echo "📊 Deploying Kafka cluster..."

    local kafka_files=(
        "$K8S_DIR/services/kafka-service.yaml"
        "$K8S_DIR/services/kafka-service-external.yaml"
        "$K8S_DIR/deployments/kafka-deployment.yaml"
    )

    for file in "${kafka_files[@]}"; do
        if [ -f "$file" ]; then
            kubectl apply -f "$file"
            echo "✅ Applied: $(basename "$file")"
        else
            echo "❌ Kafka file not found: $file"
            return 1
        fi
    done

    echo "⏳ Waiting for Kafka to be ready..."
    kubectl rollout status statefulset/kafka -n "$NAMESPACE" --timeout=300s
    kubectl wait --for=condition=ready pod/kafka-0 -n "$NAMESPACE" --timeout=120s

    echo "✅ Kafka deployed"
}

# Функция настройки ClickHouse Cloud
setup_clickhouse_cloud() {
    echo "☁️ Configuring ClickHouse Cloud connection..."

    # Проверяем наличие секрета с данными ClickHouse Cloud
    if kubectl get secret clickhouse-cloud-secret -n $NAMESPACE >/dev/null 2>&1; then
        echo "✅ ClickHouse Cloud secret already exists"
        return 0
    fi

    echo "📝 Please provide ClickHouse Cloud connection details:"
    read -p "ClickHouse Cloud Host: " ch_host
    read -p "ClickHouse Cloud Port (default: 9440): " ch_port
    read -p "ClickHouse Cloud Username: " ch_username
    read -s -p "ClickHouse Cloud Password: " ch_password
    echo
    read -p "ClickHouse Cloud Database (default: default): " ch_database

    ch_port=${ch_port:-9440}
    ch_database=${ch_database:-default}

    # Создаем секрет с данными подключения
    kubectl create secret generic clickhouse-cloud-secret \
        --namespace "$NAMESPACE" \
        --from-literal=host="$ch_host" \
        --from-literal=port="$ch_port" \
        --from-literal=username="$ch_username" \
        --from-literal=password="$ch_password" \
        --from-literal=database="$ch_database" \
        --dry-run=client -o yaml | kubectl apply -f -

    echo "✅ ClickHouse Cloud configuration saved as secret"

    # Обновляем конфиг clickhouse-loader для использования Cloud
    if [ -f "$K8S_DIR/configs/clickhouse-loader-config.yaml" ]; then
        echo "🔧 Updating clickhouse-loader config for Cloud..."
        # Создаем патч для использования Cloud соединения
        kubectl patch configmap clickhouse-loader-config -n "$NAMESPACE" --type merge \
            -p "{\"data\":{\"CLICK_HOUSE_DSN\":\"https://${ch_host}:${ch_port}?username=${ch_username}&password=${ch_password}&database=${ch_database}&secure=true\"}}"
    fi
}

# Функция деплоя лоадеров
deploy_loaders() {
    echo "📥 Deploying loaders..."

    local loader_files=(
        "$K8S_DIR/deployments/kafka-loader-deployment.yaml"
        "$K8S_DIR/services/kafka-loader-service.yaml"
        "$K8S_DIR/services/kafka-loader-service-external.yaml"
        "$K8S_DIR/deployments/clickhouse-loader-deployment.yaml"
        "$K8S_DIR/services/clickhouse-loader-service.yaml"
        "$K8S_DIR/services/clickhouse-loader-service-external.yaml"
    )

    for file in "${loader_files[@]}"; do
        if [ -f "$file" ]; then
            kubectl apply -f "$file"
            echo "✅ Applied: $(basename "$file")"
        else
            echo "⚠️ Loader file not found: $file"
        fi
    done

    echo "⏳ Waiting for loaders to start..."
    kubectl rollout status deployment/kafka-loader -n "$NAMESPACE" --timeout=180s
    kubectl rollout status deployment/clickhouse-loader -n "$NAMESPACE" --timeout=180s
    echo "✅ Loaders deployed"
}

# Функция деплоя сервисов
deploy_services() {
    echo "🚀 Deploying microservices..."

    local services=("bid-engine" "orchestrator" "router" "spp-adapter")

    for service in "${services[@]}"; do
        echo "📦 Deploying $service..."

        local deployment_file="$K8S_DIR/deployments/${service}-deployment.yaml"
        local service_file="$K8S_DIR/services/${service}-service.yaml"
        local deployment_name=${service}-deployment

        if [ -f "$deployment_file" ]; then
            kubectl apply -f "$deployment_file"
        else
            echo "❌ Deployment file not found: $deployment_file"
            return 1
        fi

        if [ -f "$service_file" ]; then
            kubectl apply -f "$service_file"
        else
            echo "❌ Service file not found: $service_file"
            return 1
        fi

        local external_service_file="$K8S_DIR/services/${service}-service-external.yaml"
        if [ -f "$external_service_file" ]; then
            kubectl apply -f "$external_service_file"
        fi

        kubectl rollout status "deployment/$deployment_name" -n "$NAMESPACE" --timeout=180s
    done

    echo "📊 Services status:"
    kubectl get pods -n "$NAMESPACE"
    echo "✅ Services deployed"
}

# Функция деплоя внешнего шлюза
deploy_gateway() {
    echo "🌉 Deploying external gateway..."

    local config_file="$K8S_DIR/configs/gateway-config.yaml"
    local deployment_file="$K8S_DIR/deployments/gateway-deployment.yaml"
    local service_file="$K8S_DIR/services/gateway-service.yaml"

    for file in "$config_file" "$deployment_file" "$service_file"; do
        if [ ! -f "$file" ]; then
            echo "❌ Gateway file not found: $file"
            return 1
        fi
    done

    kubectl apply -f "$config_file"
    kubectl apply -f "$deployment_file"
    kubectl apply -f "$service_file"

    kubectl rollout status deployment/gateway-deployment -n "$NAMESPACE" --timeout=180s
    echo "✅ External gateway is ready"
}

# Функция деплоя ingress
deploy_ingress() {
    echo "🌐 Deploying ingress..."

    ensure_ingress_nginx

    local domain="${RTB_DOMAIN:-rtb.local}"
    export RTB_DOMAIN="$domain"
    export LETSENCRYPT_INGRESS_CLASS="${LETSENCRYPT_INGRESS_CLASS:-$DEFAULT_INGRESS_CLASS}"
    export LETSENCRYPT_CLUSTER_ANNOTATION_LINE='    # cert-manager disabled'

    export TLS_DNS_ADDITIONAL_LINES="    # дополнительные DNS-имена не требуются"

    local enable_acme=0
    if [ -n "${LETSENCRYPT_EMAIL:-}" ]; then
        if [[ "$domain" =~ \.local$ ]]; then
            echo "⚠️  Domain '$domain' выглядит как локальный. Let's Encrypt пропущен."
        else
            enable_acme=1
        fi
    else
        echo "ℹ️  LETSENCRYPT_EMAIL не задан, сертификат Let's Encrypt не будет выпрошен."
    fi

    if [ $enable_acme -eq 1 ]; then
        ensure_cert_manager

        local env_stage
        case "${LETSENCRYPT_ENVIRONMENT:-staging}" in
            prod|production)
                env_stage="prod"
                export LETSENCRYPT_SERVER="https://acme-v02.api.letsencrypt.org/directory"
                ;;
            *)
                env_stage="staging"
                export LETSENCRYPT_SERVER="https://acme-staging-v02.api.letsencrypt.org/directory"
                ;;
        esac

        export LETSENCRYPT_ENVIRONMENT="$env_stage"
        export LETSENCRYPT_CLUSTER_ISSUER="letsencrypt-${env_stage}"
        export LETSENCRYPT_CLUSTER_ANNOTATION_LINE="    cert-manager.io/cluster-issuer: ${LETSENCRYPT_CLUSTER_ISSUER}"

        apply_template "$K8S_DIR/cert-manager/cluster-issuer.yaml.tpl"
        apply_template "$K8S_DIR/cert-manager/certificate.yaml.tpl"
    fi

    apply_template "$K8S_DIR/ingress/gateway-ingress.yaml.tpl"
    echo "✅ Ingress манифест применён"
}

# Функция проверки статуса
check_status() {
    echo "📊 Deployment status:"
    echo ""
    echo "=== Namespaces ==="
    kubectl get namespaces | grep -E "(NAME|$NAMESPACE)"
    echo ""
    echo "=== Pods ==="
    kubectl get pods -n "$NAMESPACE"
    echo ""
    echo "=== Services ==="
    kubectl get services -n "$NAMESPACE"
    echo ""
    echo "=== Deployments ==="
    kubectl get deployments -n "$NAMESPACE"
    echo ""
    echo "=== StatefulSets ==="
    kubectl get statefulsets -n "$NAMESPACE" 2>/dev/null || echo "No statefulsets found"
    echo ""
    echo "=== Ingress ==="
    kubectl get ingress -n "$NAMESPACE" 2>/dev/null || echo "No ingress found"

    print_ingress_usage
}

# Функция показа логов
show_logs() {
    local service="${1:-}"
    local services=("bid-engine" "orchestrator" "router" "spp-adapter" "redis" "kafka" "kafka-loader" "clickhouse-loader" "gateway")
    
    if [ -z "$service" ]; then
        echo "Available services: ${services[*]}"
        echo "Usage: $0 logs [service-name]"
        return 1
    fi
    
    echo "📋 Logs for $service:"
    if [ "$service" = "kafka" ]; then
        kubectl logs -l app=kafka -n "$NAMESPACE" --tail=50 --prefix=true
    elif [ "$service" = "gateway" ]; then
        kubectl logs -l app=gateway -n "$NAMESPACE" --tail=50
    else
        kubectl logs -l app="$service" -n "$NAMESPACE" --tail=50
    fi
}

# Функция тестирования endpoints
test_endpoints() {
    echo "🧪 Testing endpoints..."

    if ! command -v kubectl >/dev/null 2>&1; then
        echo "❌ kubectl не найден. Установите kubectl, чтобы использовать тесты."
        return 1
    fi

    if ! kubectl get nodes >/dev/null 2>&1; then
        echo "❌ Не удалось подключиться к кластеру Kubernetes"
        return 1
    fi

    local gateway_host
    gateway_host=$(resolve_ingress_host)

    echo "Ingress endpoint: $gateway_host"
    echo ""

    local endpoints=(
        "gateway:http://$gateway_host/healthz"
        "spp-adapter:http://$gateway_host/bidRequest/health"
        "https-gateway:https://$gateway_host/healthz"
    )

    for endpoint in "${endpoints[@]}"; do
        local name=$(echo "$endpoint" | cut -d: -f1)
        local url=$(echo "$endpoint" | cut -d: -f2-)

        echo "Testing $name ($url)..."
        if [[ "$url" =~ ^https ]]; then
            curl -sk --connect-timeout 5 "$url" >/dev/null
        else
            curl -s --connect-timeout 5 "$url" >/dev/null
        fi

        if [ $? -eq 0 ]; then
            echo "✅ $name is accessible"
        else
            echo "❌ $name is not accessible"
        fi
    done
}

# Основная функция деплоя
deploy_all() {
    echo "🚀 Starting full deployment..."
    
    auto_setup_before_deploy

    if ! kubectl get namespace "$NAMESPACE" >/dev/null 2>&1; then
        echo "📦 Creating namespace $NAMESPACE..."
        kubectl create namespace "$NAMESPACE"
    fi
    
    # Деплоим компоненты в правильном порядке
    deploy_configs
    deploy_redis
    deploy_kafka
    
    # Настройка ClickHouse Cloud по запросу
    if [[ "${CONFIGURE_CLICKHOUSE_CLOUD:-0}" == "1" ]]; then
        echo "ℹ️ Auto-configuring ClickHouse Cloud from environment"
        setup_clickhouse_cloud
    else
        echo "ℹ️ Skipping ClickHouse Cloud configuration (set CONFIGURE_CLICKHOUSE_CLOUD=1 to enable)"
    fi
    
    deploy_loaders
    deploy_services
    deploy_gateway
    deploy_ingress
    
    echo "✅ Full deployment completed!"
    echo ""
    check_status
    echo ""
    echo "🎉 Deployment ready! Use './deploy.sh test' to test endpoints"
}

# Обработка команд
case "${1:-all}" in
    "all")
        deploy_all
        ;;
    "configs")
        auto_setup_before_deploy
        deploy_configs
        ;;
    "redis")
        auto_setup_before_deploy
        deploy_redis
        ;;
    "kafka")
        auto_setup_before_deploy
        deploy_kafka
        ;;
    "clickhouse")
        auto_setup_before_deploy
        setup_clickhouse_cloud
        ;;
    "loaders")
        auto_setup_before_deploy
        deploy_loaders
        ;;
    "services")
        auto_setup_before_deploy
        deploy_services
        ;;
    "gateway")
        auto_setup_before_deploy
        deploy_gateway
        ;;
    "ingress")
        auto_setup_before_deploy
        deploy_ingress
        ;;
    "metallb")
        ensure_metallb
        ;;
    "status")
        check_status
        ;;
    "logs")
        show_logs "$2"
        ;;
    "test")
        test_endpoints
        ;;
    "clean")
        clean_resources
        ;;
    "destroy")
        destroy_namespace
        ;;
    "help"|"-h"|"--help")
        usage
        ;;
    *)
        echo "Unknown option: $1"
        usage
        exit 1
        ;;
esac