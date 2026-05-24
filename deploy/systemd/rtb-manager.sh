#!/bin/bash
set -e

PROJECT_DIR="/root/RTB/gpt_test_3"

# Data Pipeline сервисы (хранилище и загрузчики)
DATA_PIPELINE_SERVICES=(
    "rtb-redis"
    "rtb-kafka"
    "rtb-kafka-loader"
    "rtb-clickhouse-loader"
)

# Core RTB сервисы (всё остальное, без загрузчиков)
CORE_SERVICES=(
    "rtb-bid-engine"
    "rtb-orchestrator"
    "rtb-spp-adapter"
    "rtb-adm-adapter"
    "rtb-router"
)

# Все сервисы вместе
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

wait_for_redis() {
    echo "⏳ Waiting for Redis..."
    while ! redis-cli -a redis121 ping >/dev/null 2>&1; do
        sleep 2
        echo -n "."
    done
    echo " ✅ Redis ready"
}

wait_for_kafka() {
    echo "⏳ Waiting for Kafka..."
    local t=0
    while ! /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --list >/dev/null 2>&1; do
        sleep 5
        t=$((t+5))
        echo -n "."
        if [ $t -gt 180 ]; then
            echo " ⚠️ Kafka timeout"
            break
        fi
    done
    echo " ✅ Kafka ready"
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
        echo "🚀 Starting Data Pipeline (Redis, Kafka, Kafka-loader, ClickHouse-loader)..."
        start_services "${DATA_PIPELINE_SERVICES[@]}"
        wait_for_redis
        wait_for_kafka
        wait_for_loaders
        echo "✅ Data pipeline fully started"
        ;;

    stopD|stop-data-pipeline)
        echo "🛑 Stopping Data Pipeline..."
        stop_services "rtb-clickhouse-loader"
        stop_services "rtb-kafka-loader"
        stop_services "rtb-kafka"
        stop_services "rtb-redis"
        echo "✅ Data pipeline stopped"
        ;;

    restartD|restart-data-pipeline)
        echo "🔄 Restarting Data Pipeline..."
        $0 stopD
        sleep 3
        $0 startD
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
        echo "🔨 Building Data Pipeline services (kafka-loader, clickhouse-loader)..."
        go build -o ./cmd/clickhouse-loader/clickhouse-loader ./cmd/clickhouse-loader
        go build -o ./cmd/kafka-loader/kafka-loader ./cmd/kafka-loader
        chmod +x ./cmd/clickhouse-loader/clickhouse-loader
        chmod +x ./cmd/kafka-loader/kafka-loader
        echo "✅ Data Pipeline build done"
        $0 restartD
        ;;

    # --- Core RTB Services (C) ---
    startC|start-core)
        check_project_dir
        echo "🚀 Starting Core RTB Services..."
        start_services "${CORE_SERVICES[@]}"
        echo "✅ Core services started"
        ;;

    stopC|stop-core)
        echo "🛑 Stopping Core RTB Services..."
        stop_services "${CORE_SERVICES[@]}"
        echo "✅ Core services stopped"
        ;;

    restartC|restart-core)
        echo "🔄 Restarting Core RTB Services..."
        $0 stopC
        sleep 2
        $0 startC
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
        echo "🔨 Building Core services (bid-engine, orchestrator, spp-adapter, adm-adapter, router)..."
        go build -o ./cmd/router/router ./cmd/router
        go build -o ./cmd/orchestrator/orchestrator ./cmd/orchestrator
        go build -o ./cmd/bid-engine/bid-engine ./cmd/bid-engine
        go build -o ./cmd/spp-adapter/spp-adapter ./cmd/spp-adapter
        go build -o ./cmd/adm-adapter/adm-adapter ./cmd/adm-adapter
        chmod +x ./cmd/*/*

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

        echo "✅ Core services build done"
        $0 restartC
        ;;

    # --- Full System (всё вместе) ---
    start-all)
        $0 startD
        sleep 3
        $0 startC
        ;;

    stop-all)
        $0 stopC
        sleep 2
        $0 stopD
        ;;

    restart-all)
        $0 stop-all
        sleep 3
        $0 start-all
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
        go build -o ./cmd/spp-adapter/spp-adapter ./cmd/spp-adapter
        go build -o ./cmd/adm-adapter/adm-adapter ./cmd/adm-adapter
        go build -o ./cmd/clickhouse-loader/clickhouse-loader ./cmd/clickhouse-loader
        go build -o ./cmd/kafka-loader/kafka-loader ./cmd/kafka-loader
        chmod +x ./cmd/*/*

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
        $0 build
        $0 start-all
        $0 status
        ;;

    update)
        check_project_dir
        echo "📥 Updating all services from git..."
        cd "$PROJECT_DIR"
        git pull
        $0 build
        $0 restart-all
        ;;

    # --- Help ---
    help|--help|-h)
        echo "Usage: $0 <command>"
        echo ""
        echo "Data Pipeline (D):"
        echo "  startD              Start Redis, Kafka, Kafka-loader, ClickHouse-loader"
        echo "  stopD               Stop Data Pipeline services"
        echo "  restartD            Restart Data Pipeline"
        echo "  statusD             Show Data Pipeline status"
        echo "  updateD             Git pull + build only loaders + restart Data Pipeline"
        echo ""
        echo "Core RTB Services (C):"
        echo "  startC              Start bid-engine, orchestrator, spp-adapter, adm-adapter, router"
        echo "  stopC               Stop Core services"
        echo "  restartC            Restart Core services"
        echo "  statusC             Show Core services status"
        echo "  updateC             Git pull + build only Core + restart Core"
        echo ""
        echo "Full System:"
        echo "  start-all           Start everything"
        echo "  stop-all            Stop everything"
        echo "  restart-all         Restart everything"
        echo "  status              Show all services status"
        echo "  update              Git pull + build all + restart all"
        echo ""
        echo "Build & Deploy:"
        echo "  build               Build all Go services"
        echo "  deploy              Build and start everything"
        echo ""
        echo "Examples:"
        echo "  $0 startD           # Запустить только данные"
        echo "  $0 startC           # Запустить только RTB"
        echo "  $0 restartD         # Перезапустить данные"
        echo "  $0 restartC         # Перезапустить RTB"
        echo "  $0 updateD          # Обновить только загрузчики"
        echo "  $0 updateC          # Обновить только RTB ядро"
        ;;

    *)
        echo "❌ Unknown command: $1"
        echo "Use '$0 help' for available commands"
        exit 1
        ;;
esac