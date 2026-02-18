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

# 2. Pull latest changes (optional, usually handled by Jenkins Git plugin)
# git pull origin main

# 3. Build and restart services
echo "📦 Building and restarting services..."
docker compose down
docker compose up --build -d

# 4. Wait for services to be ready
echo "⏳ Waiting for services to be ready..."
sleep 5

# 5. Check container status
echo "🔍 Checking container status..."
docker compose ps

# 6. Basic Health Check (Adjust port if necessary)
echo "🏥 Performing health check..."
# Assuming there's a health endpoint or just check if port is listening
if command -v curl &> /dev/null; then
    STATUS=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/health || echo "failed")
    if [ "$STATUS" == "200" ] || [ "$STATUS" == "404" ]; then
        echo "✅ Deployment successful! Service is reachable."
    else
        echo "⚠️  Health check returned status: $STATUS. Please check logs."
    fi
else
    echo "ℹ️  curl not found, skipping health check."
fi

echo "✨ Deployment finished!"
