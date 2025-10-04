#!/bin/bash

PROJECT_DIR="/root/RTB/gpt_test_3"
CORE_SERVICES=(
    "rtb-bid-engine"
    "rtb-router" 
    "rtb-orchestrator"
    "rtb-spp-adapter"
)

MOCK_SERVICES=(
    "rtb-dsp1"
    "rtb-dsp2"
    "rtb-dsp3"
)

ALL_SERVICES=("${CORE_SERVICES[@]}" "${MOCK_SERVICES[@]}")

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
        sudo systemctl start "$service"
    done
}

stop_services() {
    local services=("$@")
    for service in "${services[@]}"; do
        echo "Stopping $service..."
        sudo systemctl stop "$service"
    done
}

show_status() {
    local services=("$@")
    for service in "${services[@]}"; do
        status=$(systemctl is-active "$service")
        if [ "$status" = "active" ]; then
            echo "✅ $service: ACTIVE"
        else
            echo "❌ $service: $status"
        fi
    done
}

case "$1" in
    start)
        check_project_dir
        echo "🚀 Starting ALL RTB services from $PROJECT_DIR..."
        start_services "${ALL_SERVICES[@]}"
        echo "✅ All RTB services started"
        ;;
    start-core)
        check_project_dir
        echo "🚀 Starting CORE RTB services (without mocks)..."
        start_services "${CORE_SERVICES[@]}"
        echo "✅ Core RTB services started"
        ;;
    start-mocks)
        check_project_dir
        echo "🚀 Starting MOCK DSP services..."
        start_services "${MOCK_SERVICES[@]}"
        echo "✅ Mock DSP services started"
        ;;
    stop)
        echo "🛑 Stopping ALL RTB services..."
        stop_services "${ALL_SERVICES[@]}"
        echo "✅ All RTB services stopped"
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
    restart)
        echo "🔄 Restarting ALL RTB services..."
        $0 stop
        sleep 2
        $0 start
        ;;
    restart-core)
        echo "🔄 Restarting CORE RTB services..."
        $0 stop-core
        sleep 2
        $0 start-core
        ;;
    status)
        echo "📊 RTB Cluster Status:"
        show_status "${ALL_SERVICES[@]}"
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
        sudo tail -f /var/log/rtb/${service#rtb-}.log
        ;;
    errors)
        service="$2"
        if [ -z "$service" ]; then
            echo "Usage: $0 errors <service-name>"
            echo "Available services: ${ALL_SERVICES[*]}"
            exit 1
        fi
        sudo tail -f /var/log/rtb/${service#rtb-}.error.log
        ;;
    enable)
        echo "🔧 Enabling ALL RTB services..."
        for service in "${ALL_SERVICES[@]}"; do
            sudo systemctl enable "$service"
        done
        echo "✅ All services enabled to start on boot"
        ;;
    enable-core)
        echo "🔧 Enabling CORE RTB services..."
        for service in "${CORE_SERVICES[@]}"; do
            sudo systemctl enable "$service"
        done
        echo "✅ Core services enabled to start on boot"
        ;;
    disable)
        echo "🔧 Disabling ALL RTB services..."
        for service in "${ALL_SERVICES[@]}"; do
            sudo systemctl disable "$service"
        done
        echo "✅ All services disabled from starting on boot"
        ;;
    build)
        check_project_dir
        echo "🔨 Building all services..."
        cd "$PROJECT_DIR"
        
        # Собираем сервисы с правильными путями
        go build -o ./cmd/bid-engine ./cmd/bid-engine
        go build -o ./cmd/orchestrator ./cmd/orchestrator
        go build -o ./cmd/router ./cmd/router
        go build -o ./cmd/spp-adapter ./cmd/spp-adapter
        go build -o ./cmd/dsp1 ./cmd/dsp1
        go build -o ./cmd/dsp2 ./cmd/dsp2
        go build -o ./cmd/dsp3 ./cmd/dsp3
        
	 # Делаем бинарники исполняемыми!
    	chmod +x ./cmd/bid-engine/bid-engine
    	chmod +x ./cmd/orchestrator/orchestrator
    	chmod +x ./cmd/router/router
    	chmod +x ./cmd/spp-adapter/spp-adapter
    	chmod +x ./cmd/dsp1/dsp1
    	chmod +x ./cmd/dsp2/dsp2
    	chmod +x ./cmd/dsp3/dsp3

        # Копируем конфиги в корень для удобства
        cp ./cmd/router/dsp_rules.json ./
        cp ./cmd/router/spp_rules.json ./
        cp ./cmd/spp-adapter/GeoIP2_City.mmdb ./
        
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
    *)
        echo "Usage: $0 {start|start-core|start-mocks|stop|stop-core|stop-mocks|restart|restart-core|status|status-core|status-mocks|logs|errors|enable|enable-core|disable|build|update|deploy}"
        echo ""
        echo "Commands:"
        echo "  start         - Start ALL services (core + mocks)"
        echo "  start-core    - Start only CORE services (without mocks)"
        echo "  start-mocks   - Start only MOCK DSP services"
        echo "  stop          - Stop ALL services"
        echo "  stop-core     - Stop only CORE services"
        echo "  stop-mocks    - Stop only MOCK DSP services"
        echo "  restart       - Restart ALL services"
        echo "  restart-core  - Restart only CORE services"
        echo "  status        - Show status of ALL services"
        echo "  status-core   - Show status of CORE services"
        echo "  status-mocks  - Show status of MOCK services"
        echo "  logs          - Show logs for specific service"
        echo "  errors        - Show error logs for specific service"
        echo "  enable        - Enable ALL services to start on boot"
        echo "  enable-core   - Enable only CORE services to start on boot"
        echo "  disable       - Disable ALL services from starting on boot"
        echo "  build         - Rebuild all services from source"
        echo "  update        - Git pull + build + restart"
        echo "  deploy        - Full deployment (build + enable + start)"
        echo ""
        echo "Core Services: ${CORE_SERVICES[*]}"
        echo "Mock Services: ${MOCK_SERVICES[*]}"
        echo ""
        echo "Examples:"
        echo "  $0 start-core        # Start only core services"
        echo "  $0 start             # Start all services"
        echo "  $0 status-core       # Check core services status"
        echo "  $0 logs rtb-bid-engine"
        echo "  $0 build"
        exit 1
        ;;
esac
