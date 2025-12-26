#!/bin/bash
set -e

PROJECT_DIR="/root/RTB/gpt_test_3"

# ===== НАСТРОЙКИ =====
SERVICE_ROUTER="rtb-router"
ROUTER_INSTANCES=2

# Инфраструктурные сервисы
INFRA_SERVICES=(
    "rtb-redis"
    "rtb-kafka"
)

# Основные RTB сервисы (БЕЗ router!)
CORE_SERVICES=(
    "rtb-bid-engine"
    "rtb-orchestrator"
    "rtb-spp-adapter"
    "rtb-clickhouse-loader"
    "rtb-kafka-loader"
    "rtb-adm-adapter"
)

# ====================

check_project_dir() {
    if [ ! -d "$PROJECT_DIR" ]; then
        echo "❌ Project directory not found: $PROJECT_DIR"
        exit 1
    fi
}

start_router() {
    echo "🚀 Starting ROUTER instances ($ROUTER_INSTANCES)"
    systemctl daemon-reload
    for i in $(seq 1 "$ROUTER_INSTANCES"); do
        systemctl enable ${SERVICE_ROUTER}@${i} >/dev/null
        systemctl start  ${SERVICE_ROUTER}@${i}
    done
}

stop_router() {
    echo "🛑 Stopping ROUTER instances"
    for i in $(seq 1 "$ROUTER_INSTANCES"); do
        systemctl stop ${SERVICE_ROUTER}@${i} || true
    done
}

status_router() {
    echo "📊 ROUTER status"
    for i in $(seq 1 "$ROUTER_INSTANCES"); do
        systemctl status ${SERVICE_ROUTER}@${i} --no-pager
    done
}

start_services() {
    for service in "$@"; do
        echo "Starting $service..."
        systemctl start "$service"
    done
}

stop_services() {
    for service in "$@"; do
        echo "Stopping $service..."
        systemctl stop "$service" || true
    done
}

show_status() {
    for service in "$@"; do
        systemctl is-active "$service" >/dev/null 2>&1 \
            && echo "✅ $service ACTIVE" \
            || echo "❌ $service INACTIVE"
    done
}

wait_for_infra() {
    echo "⏳ Waiting for Redis..."
    while ! redis-cli ping >/dev/null 2>&1; do sleep 2; done
    echo "✅ Redis ready"

    echo "⏳ Waiting for Kafka..."
    local t=0
    while ! /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --list >/dev/null 2>&1; do
        sleep 5
        t=$((t+5))
        [ $t -gt 180 ] && break
    done
    echo "✅ Kafka ready"
}

case "$1" in
    start-infra)
        start_services "${INFRA_SERVICES[@]}"
        wait_for_infra
        ;;

    start-core)
        check_project_dir
        start_router
        start_services "${CORE_SERVICES[@]}"
        ;;

    start)
        $0 start-infra
        sleep 3
        $0 start-core
        ;;

    stop-core)
        stop_services "${CORE_SERVICES[@]}"
        stop_router
        ;;

    stop-infra)
        stop_services "${INFRA_SERVICES[@]}"
        ;;

    stop)
        $0 stop-core
        sleep 2
        $0 stop-infra
        ;;

    restart-core)
        stop_router
        sleep 1
        start_router
        stop_services "${CORE_SERVICES[@]}"
        start_services "${CORE_SERVICES[@]}"
        ;;

    restart)
        $0 stop
        sleep 3
        $0 start
        ;;

    status)
        echo "=== INFRA ==="
        show_status "${INFRA_SERVICES[@]}"
        echo
        echo "=== ROUTER ==="
        status_router
        echo
        echo "=== CORE ==="
        show_status "${CORE_SERVICES[@]}"
        ;;

    build)
        check_project_dir
        cd "$PROJECT_DIR"
        echo "🔨 Building services..."
        go build -o ./cmd/router/router ./cmd/router
        go build -o ./cmd/orchestrator/orchestrator ./cmd/orchestrator
        go build -o ./cmd/bid-engine/bid-engine ./cmd/bid-engine
        go build -o ./cmd/spp-adapter/spp-adapter ./cmd/spp-adapter
        go build -o ./cmd/adm-adapter/adm-adapter ./cmd/adm-adapter
        go build -o ./cmd/clickhouse-loader/clickhouse-loader ./cmd/clickhouse-loader
        go build -o ./cmd/kafka-loader/kafka-loader ./cmd/kafka-loader
        chmod +x ./cmd/*/*

        # Копируем конфиги в корень для удобства (исправленные пути)
        if [ -f "./cmd/router/dsp_rules_v25.json" ]; then
            cp ./cmd/router/dsp_rules_v25.json ./
            echo "✅ Copied dsp_rules_v25.json"
        else
            echo "⚠️  dsp_rules_v25.json not found"
        fi
        
        if [ -f "./cmd/router/spp_rules_v25.json" ]; then
            cp ./cmd/router/spp_rules_v25.json ./
            echo "✅ Copied spp_rules_v25.json"
        else
            echo "⚠️  spp_rules_v25.json not found"
        fi

        if [ -f "./cmd/router/firehol_level1.netset" ]; then
            cp ./cmd/router/firehol_level1.netset ./
            echo "✅ Copied firehol_level1.netset"
        else
            echo "⚠️  firehol_level1.netset not found"
        fi
        
        if [ -f "./cmd/spp-adapter/GeoIP2_City.mmdb" ]; then
            cp ./cmd/spp-adapter/GeoIP2_City.mmdb ./
            echo "✅ Copied GeoIP2_City.mmdb"
        else
            echo "⚠️  GeoIP2_City.mmdb not found"
        fi

        if [ -f "./cmd/bid-engine/sspGeoDspPersents.json" ]; then
            cp ./cmd/bid-engine/sspGeoDspPersents.json ./
            echo "✅ Copied sspGeoDspPersents.json"
        else
            echo "⚠️  sspGeoDspPersents.json not found"
        fi

        if [ -f "./cmd/router/sspGeoDspLinks.json" ]; then
            cp ./cmd/router/sspGeoDspLinks.json ./
            echo "✅ Copied sspGeoDspLinks.json"
        else
            echo "⚠️  sspGeoDspLinks.json not found"
        fi

        if [ -f "./cmd/router/dspFilters.json" ]; then
            cp ./cmd/router/dspFilters.json ./
            echo "✅ Copied dspFilters.json"
        else
            echo "⚠️  dspFilters.json not found"
        fi

        if [ -f "./cmd/adm-adapter/fullchain.pem" ]; then
            cp ./cmd/adm-adapter/fullchain.pem ./
            echo "✅ Copied fullchain.pem"
        else
            echo "⚠️  fullchain.pem not found"
        fi

        if [ -f "./cmd/adm-adapter/privkey.pem" ]; then
            cp ./cmd/adm-adapter/privkey.pem ./
            echo "✅ Copied privkey.pem"
        else
            echo "⚠️  privkey.pem not found"
        fi

        if [ -f "./cmd/adm-adapter/rsa-fullchain.pem" ]; then
            cp ./cmd/adm-adapter/rsa-fullchain.pem ./
            echo "✅ Copied rsa-fullchain.pem"
        else
            echo "⚠️  rsa-fullchain.pem not found"
        fi

        if [ -f "./cmd/adm-adapter/rsa-privkey.pem" ]; then
            cp ./cmd/adm-adapter/rsa-privkey.pem ./
            echo "✅ Copied rsa-privkey.pem"
        else
            echo "⚠️  rsa-privkey.pem not found"
        fi

        if [ -f "./cmd/adm-adapter/adm-fullchain.pem" ]; then
            cp ./cmd/adm-adapter/adm-fullchain.pem ./
            echo "✅ Copied adm-fullchain.pem"
        else
            echo "⚠️  adm-fullchain.pem not found"
        fi

        if [ -f "./cmd/adm-adapter/adm-privkey.pem" ]; then
            cp ./cmd/adm-adapter/adm-privkey.pem ./
            echo "✅ Copied adm-privkey.pem"
        else
            echo "⚠️  adm-privkey.pem not found"
        fi

        if [ -f "./cmd/adm-adapter/adm-rsa-fullchain.pem" ]; then
            cp ./cmd/adm-adapter/adm-rsa-fullchain.pem ./
            echo "✅ Copied adm-rsa-fullchain.pem"
        else
            echo "⚠️  adm-rsa-fullchain.pem not found"
        fi

        if [ -f "./cmd/adm-adapter/adm-rsa-privkey.pem" ]; then
            cp ./cmd/adm-adapter/adm-rsa-privkey.pem ./
            echo "✅ Copied adm-rsa-privkey.pem"
        else
            echo "⚠️  adm-rsa-privkey.pem not found"
        fi

        echo "✅ Build done"
        ;;

    deploy)
        $0 build
        $0 start
        $0 status
        ;;

    update)
        check_project_dir
        echo "📥 Updating from git..."
        cd "$PROJECT_DIR"
        git pull
        $0 build
        $0 restart
        ;;

    *)
        echo "Usage:"
        echo "  $0 start|stop|restart|status"
        echo "  $0 start-core|stop-core|restart-core"
        echo "  $0 start-infra|stop-infra"
        echo "  $0 logs rtb-router <n>"
        echo "  $0 errors rtb-router <n>"
        exit 1
        ;;
esac