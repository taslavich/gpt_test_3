#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
STACK_FILE="${SCRIPT_DIR}/docker-stack.yaml"
STACK_NAME="rtb"

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

missing=0
for required in \
    "${PROJECT_ROOT}/dsp_rules.json" \
    "${PROJECT_ROOT}/spp_rules.json" \
    "${SCRIPT_DIR}/ssl-certs/fullchain.pem" \
    "${SCRIPT_DIR}/ssl-certs/privkey.pem"; do
    if [ ! -f "${required}" ]; then
        echo "❌ Required file not found: ${required}"
        missing=1
    fi
done

if [ "${missing}" -ne 0 ]; then
    echo "❌ Please make sure all required configuration and certificate files are in place before deploying."
    exit 1
fi

echo "📦 Deploying RTB Stack..."
docker stack deploy -c "${STACK_FILE}" "${STACK_NAME}"

echo
sleep 5

echo "📋 Service Status:"
docker service ls --filter name="${STACK_NAME}"
