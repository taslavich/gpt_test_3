#!/bin/bash
set -e

echo "🚀 RTB Exchange Docker Swarm First Deployment"
echo "=============================================="

# Проверка Docker Swarm
if ! docker info | grep -q "Swarm: active"; then
    echo "❌ Docker Swarm not initialized. Initializing..."
    docker swarm init --advertise-addr 142.93.239.222
fi

echo "🔐 Let's Encrypt SSL certificates are ready"

# Деплой основного стека
echo "📦 Deploying RTB Stack..."
docker stack deploy -c docker-stack.yaml rtb

echo ""
echo "🎉 RTB Exchange successfully deployed!"
echo "🌐 Live at: https://twinbidexchange.com"
echo "🔒 Using: Let's Encrypt SSL certificates"
echo "📊 Check status: docker service ls"
echo "🔍 View logs: docker service logs rtb_nginx-gateway"

# Мониторинг запуска
sleep 10
echo ""
echo "📋 Service Status:"
docker service ls --filter name=rtb_