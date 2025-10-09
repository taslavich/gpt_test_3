#!/bin/bash

PROJECT_DIR="/root/RTB/gpt_test_3"

# Инфраструктурные сервисы
INFRA_SERVICES=(
    "rtb-redis"
    "rtb-kafka" 
    "rtb-nginx"
)

# Основные RTB сервисы
CORE_SERVICES=(
    "rtb-bid-engine"
    "rtb-router" 
    "rtb-orchestrator"
    "rtb-spp-adapter"
    "rtb-clickhouse-loader"
    "rtb-kafka-loader"
)

# Mock сервисы
MOCK_SERVICES=(
    "rtb-dsp1"
    "rtb-dsp2"
    "rtb-dsp3"
)

# Все сервисы
ALL_SERVICES=("${INFRA_SERVICES[@]}" "${CORE_SERVICES[@]}" "${MOCK_SERVICES[@]}")

check_project_dir() {
    if [ ! -d "$PROJECT_DIR" ]; then
        echo "❌ Project directory not found: $PROJECT_DIR"
        echo "Please update PROJECT_DIR in /usr/local/bin/rtb-manager"
        exit 1
    fi
}

start_services() {
    local services=("$@")
    for service in "${services[@]}"; do
        echo "Starting $service..."
        sudo systemctl start "$service" 2>/dev/null || echo "⚠️  Failed to start $service (may not exist)"
    done
}

stop_services() {
    local services=("$@")
    for service in "${services[@]}"; do
        echo "Stopping $service..."
        sudo systemctl stop "$service" 2>/dev/null || echo "⚠️  Failed to stop $service (may not exist)"
    done
}

show_status() {
    local services=("$@")
    for service in "${services[@]}"; do
        if systemctl is-active "$service" >/dev/null 2>&1; then
            status=$(systemctl is-active "$service")
            echo "✅ $service: ACTIVE"
        else
            echo "❌ $service: INACTIVE or NOT FOUND"
        fi
    done
}

wait_for_infra() {
    echo "⏳ Waiting for infrastructure services to be ready..."
    
    # Ждем Redis
    while ! redis-cli ping >/dev/null 2>&1; do
        echo "Waiting for Redis..."
        sleep 2
    done
    echo "✅ Redis is ready"
    
    # Ждем Kafka через localhost (рабочий вариант)
    echo "Waiting for Kafka (this may take a few minutes)..."
    local kafka_timeout=180
    local kafka_waited=0
    
    while ! /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --list >/dev/null 2>&1; do
        if [ $kafka_waited -ge $kafka_timeout ]; then
            echo "❌ Kafka timeout after ${kafka_timeout}s"
            break
        fi
        echo "Waiting for Kafka... (${kafka_waited}s)"
        sleep 5
        kafka_waited=$((kafka_waited + 5))
    done
    
    if /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --list >/dev/null 2>&1; then
        echo "✅ Kafka is ready"
    else
        echo "⚠️  Kafka is still initializing..."
    fi
    
    # Ждем Nginx
    while ! curl -s http://localhost >/dev/null; do
        echo "Waiting for Nginx..."
        sleep 2
    done
    echo "✅ Nginx is ready"
}

case "$1" in
    start-infra)
        echo "🏗️  Starting INFRASTRUCTURE services..."
        start_services "${INFRA_SERVICES[@]}"
        wait_for_infra
        echo "✅ Infrastructure services started and ready"
        ;;
    start-core)
        check_project_dir
        echo "🚀 Starting CORE RTB services..."
        start_services "${CORE_SERVICES[@]}"
        echo "✅ Core RTB services started"
        ;;
    start-mocks)
        check_project_dir
        echo "🚀 Starting MOCK DSP services..."
        start_services "${MOCK_SERVICES[@]}"
        echo "✅ Mock DSP services started"
        ;;
    start)
        echo "🚀 Starting ALL services (infra + core + mocks)..."
        #$0 start-infra
        sleep 3
        $0 start-core
        sleep 2
        $0 start-mocks
        echo "✅ All services started"
        ;;
    stop-infra)
        echo "🛑 Stopping INFRASTRUCTURE services..."
        stop_services "${INFRA_SERVICES[@]}"
        echo "✅ Infrastructure services stopped"
        ;;
    stop-core)
        echo "🛑 Stopping CORE RTB services..."
        stop_services "${CORE_SERVICES[@]}"
        echo "✅ Core RTB services stopped"
        ;;
    stop-mocks)
        echo "🛑 Stopping MOCK DSP services..."
        stop_services "${MOCK_SERVICES[@]}"
        echo "✅ Mock DSP services stopped"
        ;;
    stop)
        echo "🛑 Stopping ALL services..."
        $0 stop-mocks
        sleep 1
        $0 stop-core
        sleep 1
        #$0 stop-infra
        echo "✅ All services stopped"
        ;;
    restart)
        echo "🔄 Restarting ALL services..."
        $0 stop
        sleep 3
        $0 start
        ;;
    restart-core)
        echo "🔄 Restarting CORE RTB services..."
        $0 stop-core
        sleep 2
        $0 start-core
        ;;
    status)
        echo "📊 ALL Services Status:"
        echo "=== INFRASTRUCTURE ==="
        show_status "${INFRA_SERVICES[@]}"
        echo ""
        echo "=== CORE RTB ==="
        show_status "${CORE_SERVICES[@]}"
        echo ""
        echo "=== MOCK DSP ==="
        show_status "${MOCK_SERVICES[@]}"
        ;;
    status-infra)
        echo "📊 INFRASTRUCTURE Services Status:"
        show_status "${INFRA_SERVICES[@]}"
        ;;
    status-core)
        echo "📊 CORE RTB Services Status:"
        show_status "${CORE_SERVICES[@]}"
        ;;
    status-mocks)
        echo "📊 MOCK DSP Services Status:"
        show_status "${MOCK_SERVICES[@]}"
        ;;
    logs)
        service="$2"
        if [ -z "$service" ]; then
            echo "Usage: $0 logs <service-name>"
            echo "Available services: ${ALL_SERVICES[*]}"
            exit 1
        fi
        
        # Определяем имя файла лога
        case "$service" in
            redis-server) log_file="redis.log" ;;
            kafka-server) log_file="kafka.log" ;;
            nginx) log_file="nginx.log" ;;
            *) log_file="${service#rtb-}.log" ;;
        esac
        
        sudo tail -f "/var/log/rtb/$log_file"
        ;;
    errors)
        service="$2"
        if [ -z "$service" ]; then
            echo "Usage: $0 errors <service-name>"
            echo "Available services: ${ALL_SERVICES[*]}"
            exit 1
        fi
        
        # Определяем имя файла ошибок
        case "$service" in
            redis-server) error_file="redis.error.log" ;;
            kafka-server) error_file="kafka.error.log" ;;
            nginx) error_file="nginx.error.log" ;;
            *) error_file="${service#rtb-}.error.log" ;;
        esac
        
        sudo tail -f "/var/log/rtb/$error_file"
        ;;
    enable-infra)
        echo "🔧 Enabling INFRASTRUCTURE services..."
        for service in "${INFRA_SERVICES[@]}"; do
            sudo systemctl enable "$service"
        done
        echo "✅ Infrastructure services enabled to start on boot"
        ;;
    enable-core)
        echo "🔧 Enabling CORE RTB services..."
        for service in "${CORE_SERVICES[@]}"; do
            sudo systemctl enable "$service"
        done
        echo "✅ Core services enabled to start on boot"
        ;;
    enable-mocks)
        echo "🔧 Enabling MOCK DSP services..."
        for service in "${MOCK_SERVICES[@]}"; do
            sudo systemctl enable "$service"
        done
        echo "✅ Mock services enabled to start on boot"
        ;;
    enable)
        echo "🔧 Enabling ALL services..."
        $0 enable-infra
        $0 enable-core
        $0 enable-mocks
        echo "✅ All services enabled to start on boot"
        ;;
    disable)
        echo "🔧 Disabling ALL services..."
        for service in "${ALL_SERVICES[@]}"; do
            sudo systemctl disable "$service" 2>/dev/null || true
        done
        echo "✅ All services disabled from starting on boot"
        ;;
    build)
        check_project_dir
        echo "🔨 Building all services..."
        cd "$PROJECT_DIR"
        
        # Собираем сервисы с правильными путями
        go build -o ./cmd/bid-engine/bid-engine ./cmd/bid-engine
        go build -o ./cmd/orchestrator/orchestrator ./cmd/orchestrator
        go build -o ./cmd/router/router ./cmd/router
        go build -o ./cmd/spp-adapter/spp-adapter ./cmd/spp-adapter
        go build -o ./cmd/clickhouse-loader/clickhouse-loader ./cmd/clickhouse-loader
        go build -o ./cmd/kafka-loader/kafka-loader ./cmd/kafka-loader
        go build -o ./cmd/dsp1/dsp1 ./cmd/dsp1
        go build -o ./cmd/dsp2/dsp2 ./cmd/dsp2
        go build -o ./cmd/dsp3/dsp3 ./cmd/dsp3
        
        # Делаем бинарники исполняемыми!
        chmod +x ./cmd/bid-engine/bid-engine
        chmod +x ./cmd/orchestrator/orchestrator
        chmod +x ./cmd/router/router
        chmod +x ./cmd/spp-adapter/spp-adapter
        chmod +x ./cmd/clickhouse-loader/clickhouse-loader
        chmod +x ./cmd/kafka-loader/kafka-loader
        chmod +x ./cmd/dsp1/dsp1
        chmod +x ./cmd/dsp2/dsp2
        chmod +x ./cmd/dsp3/dsp3

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
        
        if [ -f "./cmd/spp-adapter/GeoIP2_City.mmdb" ]; then
            cp ./cmd/spp-adapter/GeoIP2_City.mmdb ./
            echo "✅ Copied GeoIP2_City.mmdb"
        else
            echo "⚠️  GeoIP2_City.mmdb not found"
        fi
        
        echo "✅ All services built and made executable"
        ;;
    update)
        check_project_dir
        echo "📥 Updating from git..."
        cd "$PROJECT_DIR"
        git pull
        $0 build
        $0 restart
        ;;
    deploy)
        echo "🚀 Full deployment process..."
        check_project_dir
        $0 build
        $0 enable
        $0 start
        $0 status
        ;;
    test-infra)
        echo "🧪 Testing infrastructure connections..."
        
        # Test Redis
        if redis-cli ping >/dev/null 2>&1; then
            echo "✅ Redis: OK"
        else
            echo "❌ Redis: FAILED"
        fi
        
        # Test Kafka (используем localhost)
        if /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --list >/dev/null 2>&1; then
            echo "✅ Kafka: OK"
        else
            echo "❌ Kafka: FAILED"
        fi
        
        # Test Nginx
        if curl -s http://localhost >/dev/null; then
            echo "✅ Nginx: OK"
        else
            echo "❌ Nginx: FAILED"
        fi
        ;;
    *)
        echo "Usage: $0 {start|start-infra|start-core|start-mocks|stop|stop-infra|stop-core|stop-mocks|restart|restart-core|status|status-infra|status-core|status-mocks|logs|errors|enable|enable-infra|enable-core|enable-mocks|disable|build|update|deploy|test-infra}"
        echo ""
        echo "Commands:"
        echo "  start         - Start ALL services (infra + core + mocks)"
        echo "  start-infra   - Start only INFRASTRUCTURE services"
        echo "  start-core    - Start only CORE RTB services"
        echo "  start-mocks   - Start only MOCK DSP services"
        echo "  stop          - Stop ALL services"
        echo "  stop-infra    - Stop only INFRASTRUCTURE services"
        echo "  stop-core     - Stop only CORE RTB services"
        echo "  stop-mocks    - Stop only MOCK DSP services"
        echo "  restart       - Restart ALL services"
        echo "  restart-core  - Restart only CORE RTB services"
        echo "  status        - Show status of ALL services"
        echo "  status-infra  - Show status of INFRASTRUCTURE services"
        echo "  status-core   - Show status of CORE RTB services"
        echo "  status-mocks  - Show status of MOCK DSP services"
        echo "  logs          - Show logs for specific service"
        echo "  errors        - Show error logs for specific service"
        echo "  enable        - Enable ALL services to start on boot"
        echo "  enable-infra  - Enable only INFRASTRUCTURE services"
        echo "  enable-core   - Enable only CORE RTB services"
        echo "  enable-mocks  - Enable only MOCK DSP services"
        echo "  disable       - Disable ALL services from starting on boot"
        echo "  build         - Rebuild all services from source"
        echo "  update        - Git pull + build + restart"
        echo "  deploy        - Full deployment (build + enable + start)"
        echo "  test-infra    - Test infrastructure connections"
        echo ""
        echo "Infrastructure: ${INFRA_SERVICES[*]}"
        echo "Core Services: ${CORE_SERVICES[*]}"
        echo "Mock Services: ${MOCK_SERVICES[*]}"
        echo ""
        echo "Examples:"
        echo "  $0 start-infra       # Start only infrastructure"
        echo "  $0 start-core        # Start only core services"
        echo "  $0 start             # Start all services"
        echo "  $0 status            # Check all services status"
        echo "  $0 logs rtb-bid-engine"
        echo "  $0 logs redis-server"
        echo "  $0 test-infra        # Test infrastructure"
        echo "  $0 build             # Rebuild services"
        exit 1
        ;;
esac