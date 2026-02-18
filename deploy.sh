#!/bin/bash

# Exit on error
set -e

# Configuration
APP_NAME="websocket-chat-backend"
# Move to the directory where the script is located
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
cd "$SCRIPT_DIR"
ENV_FILE=".env"

echo "🚀 Starting deployment for $APP_NAME..."

# 1. Pre-deployment checks
if [ -z "$SERVER_PORT" ]; then
    echo "⚠️  SERVER_PORT is not set. Defaulting to 8080."
    export SERVER_PORT=8080
fi

# 2. Check if .env exists
if [ ! -f "$ENV_FILE" ]; then
    echo "⚠️  $ENV_FILE not found. Using .env.example as template."
    cp .env.example .env
fi

# 3. Cleanup old containers and volumes (force remove)
echo "🧹 Cleaning up old containers..."
docker compose down -v --remove-orphans 2>/dev/null || true
sleep 2

# 4. Wait for ports to be released
echo "⏳ Waiting for ports to be released..."
MAX_RETRIES=10
RETRY_COUNT=0
while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
    if ! netstat -tlnp 2>/dev/null | grep -q ":6379\|:5432\|:${SERVER_PORT}"; then
        echo "✅ Ports are available"
        break
    fi
    echo "⏳ Waiting for ports to be released... ($((RETRY_COUNT + 1))/$MAX_RETRIES)"
    sleep 1
    RETRY_COUNT=$((RETRY_COUNT + 1))
done

# 5. Build and start services
echo "📦 Building and starting services..."
docker compose up --build -d

# 6. Wait for services to be ready
echo "⏳ Waiting for services to be ready..."
sleep 8

# 7. Check container status
echo "🔍 Checking container status..."
docker compose ps

# 8. Basic Health Check
echo "🏥 Performing health check..."
if command -v curl &> /dev/null; then
    MAX_HEALTH_RETRIES=10
    HEALTH_RETRY=0
    while [ $HEALTH_RETRY -lt $MAX_HEALTH_RETRIES ]; do
        STATUS=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:${SERVER_PORT}/health 2>/dev/null || echo "failed")
        if [ "$STATUS" == "200" ] || [ "$STATUS" == "404" ]; then
            echo "✅ Deployment successful! Service is reachable on port $SERVER_PORT"
            break
        fi
        echo "⏳ Health check retry... ($((HEALTH_RETRY + 1))/$MAX_HEALTH_RETRIES)"
        sleep 1
        HEALTH_RETRY=$((HEALTH_RETRY + 1))
    done
    if [ "$STATUS" != "200" ] && [ "$STATUS" != "404" ]; then
        echo "⚠️  Health check returned status: $STATUS. Please check logs."
        docker compose logs
    fi
else
    echo "ℹ️  curl not found, skipping health check."
fi

echo "✨ Deployment finished!"
