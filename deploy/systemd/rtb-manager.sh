#!/bin/bash
set -e

PROJECT_DIR="/root/RTB/gpt_test_3"

REDIS_SERVICES=(
    "rtb-redis-7000"
    "rtb-redis-7001"
    "rtb-redis-7002"
    "rtb-redis-7003"
    "rtb-redis-7004"
    "rtb-redis-7005"
)

# Сервисы Kafka data pipeline
KAFKA_SERVICES=(
    "rtb-kafka-loader"
)

DATA_PIPELINE_SERVICES=(
    "${KAFKA_SERVICES[@]}"
    "rtb-clickhouse-loader"
)

# Core RTB. ADV поднимается первым, затем percenter и остальные сервисы.
CORE_SERVICES=(
    "rtb-adv"
    "rtb-percenter"
    "rtb-bid-engine"
    "rtb-orchestrator"
    "rtb-router"
    "rtb-spp-adapter"
    "rtb-adm-adapter"
)

# HTTP listener ADV. Любой HTTP-ответ, включая 404 на /, означает, что порт уже слушается.
ADV_HTTP_URL="${ADV_HTTP_URL:-http://127.0.0.1:8101/}"
ADV_WAIT_TIMEOUT_SECONDS="${ADV_WAIT_TIMEOUT_SECONDS:-120}"
SERVICE_STOP_TIMEOUT_SECONDS="${SERVICE_STOP_TIMEOUT_SECONDS:-20}"

ALL_SERVICES=(
    "${DATA_PIPELINE_SERVICES[@]}"
    "${CORE_SERVICES[@]}"
)

# ====================

check_project_dir() {
    if [ ! -d "$PROJECT_DIR" ]; then
        echo "❌ Project directory not found: $PROJECT_DIR"
        exit 1
    fi
}

start_services() {
    for service in "$@"; do
        echo "Starting $service..."
        systemctl start "$service"
    done
}

stop_service() {
    local service="$1"

    echo "Stopping $service..."

    if timeout "${SERVICE_STOP_TIMEOUT_SECONDS}s" systemctl stop "$service"; then
        return 0
    fi

    echo "⚠️ $service did not stop within ${SERVICE_STOP_TIMEOUT_SECONDS}s; forcing SIGKILL..."
    systemctl kill --kill-who=all --signal=SIGKILL "$service" 2>/dev/null || true
    timeout 5s systemctl stop "$service" 2>/dev/null || true
    systemctl reset-failed "$service" 2>/dev/null || true
}

stop_services() {
    for service in "$@"; do
        stop_service "$service"
    done
}

wait_for_adv() {
    local deadline=$((SECONDS + ADV_WAIT_TIMEOUT_SECONDS))

    echo "⏳ Waiting for ADV HTTP listener: $ADV_HTTP_URL"

    while (( SECONDS < deadline )); do
        if systemctl is-active --quiet rtb-adv; then
            # Не используем --fail: 404 на корневом пути тоже подтверждает,
            # что HTTP listener ADV уже поднялся и принимает соединения.
            if curl --silent --show-error --max-time 2 \
                --output /dev/null "$ADV_HTTP_URL" 2>/dev/null; then
                echo "✅ ADV HTTP listener is ready"
                return 0
            fi
        fi

        sleep 1
    done

    echo "❌ ADV HTTP listener did not become ready within ${ADV_WAIT_TIMEOUT_SECONDS}s"
    systemctl status rtb-adv --no-pager || true
    journalctl -u rtb-adv -n 50 --no-pager || true
    return 1
}

show_status() {
    for service in "$@"; do
        systemctl is-active "$service" >/dev/null 2>&1 \
            && echo "✅ $service ACTIVE" \
            || echo "❌ $service INACTIVE"
    done
}

wait_for_loaders() {
    echo "⏳ Waiting for loaders to initialize..."
    sleep 5

    if systemctl is-active rtb-kafka-loader >/dev/null 2>&1; then
        echo "✅ Kafka-loader ready"
    else
        echo "⚠️ Kafka-loader not running"
    fi

    if systemctl is-active rtb-clickhouse-loader >/dev/null 2>&1; then
        echo "✅ ClickHouse-loader ready"
    else
        echo "⚠️ ClickHouse-loader not running"
    fi
}

# ==================== COMMANDS ====================

case "$1" in
    # --- Data Pipeline (D) ---
    startD|start-data-pipeline)
        echo "🚀 Starting Data Pipeline services (Kafka-loader, ClickHouse-loader)..."
        start_services "${DATA_PIPELINE_SERVICES[@]}"
        wait_for_loaders
        echo "✅ Data pipeline fully started"
        ;;

    stopD|stop-data-pipeline)
        echo "🛑 Stopping Data Pipeline..."
        stop_services "rtb-clickhouse-loader"
        stop_services "rtb-kafka-loader"
        echo "✅ Data pipeline stopped"
        ;;

    restartD|restart-data-pipeline)
        check_project_dir
        cd "$PROJECT_DIR"

        echo "🔨 Rebuilding Data Pipeline services..."

        go build -o ./cmd/clickhouse-loader/clickhouse-loader ./cmd/clickhouse-loader
        go build -o ./cmd/kafka-loader/kafka-loader ./cmd/kafka-loader

        chmod +x \
            ./cmd/clickhouse-loader/clickhouse-loader \
            ./cmd/kafka-loader/kafka-loader

        echo "✅ Data Pipeline rebuild completed"

        echo "🔄 Restarting Data Pipeline..."
        "$0" stopD
        sleep 3
        "$0" startD
        ;;

    statusD|status-data-pipeline)
        echo "=== DATA PIPELINE STATUS ==="
        show_status "${DATA_PIPELINE_SERVICES[@]}"
        ;;

    updateD|update-data-pipeline)
        check_project_dir
        cd "$PROJECT_DIR"
        echo "📥 Updating Data Pipeline from git..."
        git pull
        "$0" restartD
        ;;

    # --- Core RTB Services (C) ---
    startC|start-core)
        check_project_dir
        echo "🚀 Starting Core RTB Services..."

        # ADV первым поднимает HTTP control endpoint :8101.
        start_services "rtb-adv"
        wait_for_adv

        # Percenter использует Redis/ClickHouse и должен жить вместе с Core.
        start_services "rtb-percenter"

        # После готовности ADV запускаем остальные сервисы по зависимостям.
        start_services "rtb-bid-engine"
        start_services "rtb-orchestrator"
        start_services "rtb-router"
        start_services "rtb-spp-adapter"
        start_services "rtb-adm-adapter"

        echo "✅ Core services started"
        ;;

    stopC|stop-core)
        echo "🛑 Stopping Core RTB Services..."

        # Останавливаем в обратном порядке. ADV — последним.
        stop_services "rtb-adm-adapter"
        stop_services "rtb-spp-adapter"
        stop_services "rtb-router"
        stop_services "rtb-orchestrator"
        stop_services "rtb-bid-engine"
        stop_services "rtb-percenter"
        stop_services "rtb-adv"

        echo "✅ Core services stopped"
        ;;

    restartC|restart-core)
        check_project_dir
        cd "$PROJECT_DIR"

        echo "🔨 Rebuilding Core RTB Services..."

        go build -o ./cmd/router/router ./cmd/router
        go build -o ./cmd/orchestrator/orchestrator ./cmd/orchestrator
        go build -o ./cmd/bid-engine/bid-engine ./cmd/bid-engine
        go build -o ./cmd/adv/adv ./cmd/adv
        go build -o ./cmd/percenter/percenter ./cmd/percenter
        go build -o ./cmd/spp-adapter/spp-adapter ./cmd/spp-adapter
        go build -o ./cmd/adm-adapter/adm-adapter ./cmd/adm-adapter

        chmod +x \
            ./cmd/router/router \
            ./cmd/orchestrator/orchestrator \
            ./cmd/bid-engine/bid-engine \
            ./cmd/adv/adv \
            ./cmd/percenter/percenter \
            ./cmd/spp-adapter/spp-adapter \
            ./cmd/adm-adapter/adm-adapter

        echo "✅ Core rebuild completed"

        echo "🔄 Restarting Core RTB Services..."
        "$0" stopC
        sleep 2
        "$0" startC
        ;;

    statusC|status-core)
        echo "=== CORE RTB SERVICES STATUS ==="
        show_status "${CORE_SERVICES[@]}"
        ;;

    updateC|update-core)
        check_project_dir
        cd "$PROJECT_DIR"
        echo "📥 Updating Core RTB Services from git..."
        git pull

        # Копируем конфиги в корень для удобства
        if [ -f "./cmd/router/dsp_rules_v25.json" ]; then
            cp ./cmd/router/dsp_rules_v25.json ./
            echo "✅ Copied dsp_rules_v25.json"
        else
            echo "⚠️ dsp_rules_v25.json not found"
        fi

        if [ -f "./cmd/router/spp_rules_v25.json" ]; then
            cp ./cmd/router/spp_rules_v25.json ./
            echo "✅ Copied spp_rules_v25.json"
        else
            echo "⚠️ spp_rules_v25.json not found"
        fi

        if [ -f "./cmd/router/firehol_level1.netset" ]; then
            cp ./cmd/router/firehol_level1.netset ./
            echo "✅ Copied firehol_level1.netset"
        else
            echo "⚠️ firehol_level1.netset not found"
        fi

        "$0" restartC
        ;;

    # --- Full System ---
    start-all)
        "$0" startD
        sleep 3
        "$0" startC
        ;;

    stop-all)
        "$0" stopC
        sleep 2
        "$0" stopD
        ;;

    restart-all)
        echo "🔄 Rebuilding and restarting all services..."
        "$0" restartD
        sleep 3
        "$0" restartC
        ;;

    status)
        echo "=== DATA PIPELINE ==="
        show_status "${DATA_PIPELINE_SERVICES[@]}"
        echo
        echo "=== CORE RTB SERVICES ==="
        show_status "${CORE_SERVICES[@]}"
        ;;

    # --- Build & Deploy ---
    build)
        check_project_dir
        cd "$PROJECT_DIR"
        echo "🔨 Building all services..."

        go build -o ./cmd/router/router ./cmd/router
        go build -o ./cmd/orchestrator/orchestrator ./cmd/orchestrator
        go build -o ./cmd/bid-engine/bid-engine ./cmd/bid-engine
        go build -o ./cmd/adv/adv ./cmd/adv
        go build -o ./cmd/percenter/percenter ./cmd/percenter
        go build -o ./cmd/spp-adapter/spp-adapter ./cmd/spp-adapter
        go build -o ./cmd/adm-adapter/adm-adapter ./cmd/adm-adapter
        go build -o ./cmd/clickhouse-loader/clickhouse-loader ./cmd/clickhouse-loader
        go build -o ./cmd/kafka-loader/kafka-loader ./cmd/kafka-loader

        chmod +x \
            ./cmd/router/router \
            ./cmd/orchestrator/orchestrator \
            ./cmd/bid-engine/bid-engine \
            ./cmd/adv/adv \
            ./cmd/percenter/percenter \
            ./cmd/spp-adapter/spp-adapter \
            ./cmd/adm-adapter/adm-adapter \
            ./cmd/clickhouse-loader/clickhouse-loader \
            ./cmd/kafka-loader/kafka-loader

        # Копируем конфиги в корень для удобства
        if [ -f "./cmd/router/dsp_rules_v25.json" ]; then
            cp ./cmd/router/dsp_rules_v25.json ./
            echo "✅ Copied dsp_rules_v25.json"
        else
            echo "⚠️ dsp_rules_v25.json not found"
        fi

        if [ -f "./cmd/router/spp_rules_v25.json" ]; then
            cp ./cmd/router/spp_rules_v25.json ./
            echo "✅ Copied spp_rules_v25.json"
        else
            echo "⚠️ spp_rules_v25.json not found"
        fi

        if [ -f "./cmd/router/firehol_level1.netset" ]; then
            cp ./cmd/router/firehol_level1.netset ./
            echo "✅ Copied firehol_level1.netset"
        else
            echo "⚠️ firehol_level1.netset not found"
        fi

        echo "✅ Build done"
        ;;

    deploy)
        "$0" build
        "$0" start-all
        "$0" status
        ;;

    update)
        check_project_dir
        echo "📥 Updating all services from git..."
        cd "$PROJECT_DIR"
        git pull
        "$0" build
        "$0" restart-all
        ;;

    # --- Help ---
    help|--help|-h)
        echo "Usage: $0 <command>"
        echo ""
        echo "Data Pipeline (D):"
        echo "  startD              Start Kafka-loader and ClickHouse-loader"
        echo "  stopD               Stop Data Pipeline services"
        echo "  restartD            Restart Data Pipeline"
        echo "  statusD             Show Data Pipeline status"
        echo "  updateD             Git pull + build only loaders + restart Data Pipeline"
        echo ""
        echo "Core RTB Services (C):"
        echo "  startC              Start ADV, percenter, bid-engine, router, orchestrator, spp-adapter and adm-adapter"
        echo "  stopC               Stop Core services"
        echo "  restartC            Rebuild + restart Core services, including percenter"
        echo "  statusC             Show Core services status"
        echo "  updateC             Git pull + build Core + restart Core"
        echo ""
        echo "Full System:"
        echo "  start-all           Start everything"
        echo "  stop-all            Stop everything"
        echo "  restart-all         Restart everything"
        echo "  status              Show all services status"
        echo "  update              Git pull + build all + restart all"
        echo ""
        echo "Build & Deploy:"
        echo "  build               Build all Go services, including percenter"
        echo "  deploy              Build and start everything"
        echo ""
        echo "Examples:"
        echo "  $0 startD           # Запустить только данные"
        echo "  $0 startC           # Запустить только RTB + percenter"
        echo "  $0 restartD         # Перезапустить данные"
        echo "  $0 restartC         # Пересобрать и перезапустить RTB + percenter"
        echo "  $0 updateD          # Обновить только загрузчики"
        echo "  $0 updateC          # Обновить RTB ядро + percenter"
        ;;

    *)
        echo "❌ Unknown command: $1"
        echo "Use '$0 help' for available commands"
        exit 1
        ;;
esac
