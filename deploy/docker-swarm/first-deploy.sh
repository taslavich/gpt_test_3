#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
STACK_FILE="${SCRIPT_DIR}/docker-stack.yaml"
STACK_NAME="rtb-exchange"

echo "🚀 RTB Exchange Docker Swarm First Deployment"
echo "=============================================="

if ! docker info | grep -q "Swarm: active"; then
    echo "❌ Docker Swarm not initialized. Initializing..."
    docker swarm init
fi

if [ ! -f "${SCRIPT_DIR}/env/redis.env" ]; then
    echo "❌ Environment files are missing. Please check ${SCRIPT_DIR}/env"
    exit 1
fi

echo "📦 Deploying RTB Stack..."
docker stack deploy -c "${STACK_FILE}" "${STACK_NAME}"

echo
sleep 5

echo "📋 Service Status:"
docker service ls --filter name="${STACK_NAME}"
