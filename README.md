# WebSocket Chat Server - Backend

A scalable, real-time chat server built with Go, Fiber, Socket.IO, and Redis. Designed for high performance and horizontal scaling.

## 🚀 Features

- **Socket.IO Support**: Real-time bidirectional event-based communication.
- **Redis Adapter**: Horizontal scaling support (clustering).
- **Broadcast API**: REST endpoint to push messages to all connected clients in a room.
- **Health Check**: Simple endpoint for load balancers.
- **Dockerized**: Ready for production deployment.

## 🏗 Architecture

```
cmd/server/          - Application entry point
config/              - Configuration management
internal/
  ├── delivery/
  │   ├── http/      - REST handlers (Health Check, Broadcast)
  │   └── websocket/ - Socket.IO Hub & Logic
  └── infrastructure/- Redis integration
```

## 🛠 Prerequisites

- Go 1.18 or higher
- Redis 6+ (Optional, for scaling)
- Docker (Recommended)

## 📦 Setup Instructions

### 1. Clone the Repository

```bash
git clone <repository-url>
cd backend
```

### 2. Configure Environment Variables

Copy the example configuration:

```bash
cp .env.example .env
```

Edit `.env`:

```env
# Server
SERVER_PORT=8080
SERVER_HOST=0.0.0.0

# Redis (Required for scaling, optional for single node)
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
```

### 3. Run with Docker (Recommended)

```bash
docker compose up --build -d
```

The server will be available at `http://localhost:8080`.

## 📚 API Documentation

### REST Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Server health check (Returns `{"status": "ok"}`) |
| POST | `/messages` | Broadcast message to a room |

#### Broadcast Message

**POST** `/messages`

Body:

```json
{
  "room_id": "room-123",
  "content": "Hello World",
  "user_id": "user-1",
  "username": "Alice",
  "type": "text"
}
```

### Socket.IO Events

Connect using any Socket.IO client (v2/v3/v4).

**Namespace:** `/socket.io/`

**Client -> Server Events:**

- `join`: Join a specific room.
- `leave`: Leave a room.
- `message`: Send a message to the room.

**Server -> Client Events:**

- `message`: Receive a new message.

## 🚀 Deployment (Jenkins & Nginx)

This repository includes deployment scripts (`deploy.sh`, `generate-env.sh`) for CI/CD pipelines and an `nginx.conf` template for reverse proxy setup.

See [deploy.sh](deploy.sh) for details.

## 📄 License

MIT
