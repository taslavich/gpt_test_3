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

LATENCY_NODE_INPUT="${1:-${LATENCY_NODE:-}}"

if [ -z "${LATENCY_NODE_INPUT}" ]; then
    SELF_NODE_ID="$(docker info --format '{{.Swarm.NodeID}}' 2>/dev/null || true)"
    if [ -n "${SELF_NODE_ID}" ] && docker node inspect "${SELF_NODE_ID}" >/dev/null 2>&1; then
        LATENCY_NODE="${SELF_NODE_ID}"
    else
        LATENCY_NODE="$(docker node ls --filter role=manager --format '{{.ID}}' | head -n1)"
    fi
else
    if docker node inspect "${LATENCY_NODE_INPUT}" >/dev/null 2>&1; then
        LATENCY_NODE="${LATENCY_NODE_INPUT}"
    else
        LATENCY_NODE="$(docker node ls --format '{{.ID}} {{.Hostname}}' | awk -v target="${LATENCY_NODE_INPUT}" '$2 == target {print $1; exit}')"
    fi
fi

if [ -z "${LATENCY_NODE}" ]; then
    echo "❌ Не удалось определить ноду для метки rtb_latency. Укажите идентификатор или имя ноды в качестве аргумента или переменной LATENCY_NODE."
    exit 1
fi

LATENCY_NODE_NAME="$(docker node inspect -f '{{.Description.Hostname}}' "${LATENCY_NODE}" 2>/dev/null || echo "${LATENCY_NODE}")"
CURRENT_LABEL="$(docker node inspect -f '{{ index .Spec.Labels "rtb_latency" }}' "${LATENCY_NODE}" 2>/dev/null || true)"
if [ "${CURRENT_LABEL}" != "true" ]; then
    echo "🏷️  Добавляем метку rtb_latency=true на ноду ${LATENCY_NODE_NAME} (${LATENCY_NODE})"
    docker node update --label-add rtb_latency=true "${LATENCY_NODE}"
else
    echo "🏷️  Метка rtb_latency=true уже установлена на ноде ${LATENCY_NODE_NAME} (${LATENCY_NODE})"
fi

echo "📦 Deploying RTB Stack..."
docker stack deploy -c "${STACK_FILE}" "${STACK_NAME}"

echo
sleep 5

echo "📋 Service Status:"
docker service ls --filter name="${STACK_NAME}"
