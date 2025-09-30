#!/bin/bash
set -e

echo "🚀 RTB Exchange Docker Swarm First Deployment"
echo "=============================================="

# Проверка Docker Swarm
if ! docker info | grep -q "Swarm: active"; then
    echo "❌ Docker Swarm not initialized. Initializing..."
    docker swarm init --advertise-addr 142.93.239.222
fi

# Проверка домена
DOMAIN="twinbidexchange.com"
echo "🌐 Domain: $DOMAIN"

# Деплой
echo "📦 Deploying RTB Stack..."
docker stack deploy -c docker-stack.yaml rtb

echo ""
echo "✅ Deployment started!"
echo "📊 Check status: docker service ls"
echo "🔍 View logs: docker service logs rtb_certbot-setup"
echo "🌐 Test in 2-3 minutes: curl https://$DOMAIN/health"
echo ""
echo "ℹ️  SSL certificates will be obtained automatically on first run."
echo "🔄 Certificate renewal runs every 12 hours automatically."

# Мониторинг запуска
echo ""
echo "🕐 Waiting for services to start..."
sleep 10

# Проверка статуса
echo ""
echo "📋 Service Status:"
docker service ls --filter name=rtb_