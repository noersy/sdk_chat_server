# WebSocket Chat Server - Backend

A scalable, real-time chat server built with Go, Fiber, Socket.IO, and Redis. Designed for high performance and horizontal scaling.

## 🚀 Features

- **Socket.IO Support**: Real-time bidirectional event-based communication.
- **Redis Adapter**: Horizontal scaling support (clustering) and pub/sub for cross-instance communication.
- **Broadcast API**: REST endpoint to push messages to all connected clients in a room.
- **User Status Tracking**: Real-time online/offline status and "last seen" tracking.
- **Room Management**: Dynamic room creation and membership tracking.
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
- Redis 6+ (Required for state management and scaling)
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

# Redis (Required)
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
| POST | `/messages` | Broadcast message to a room via HTTP |

#### Broadcast Message

**POST** `/messages`

Body:

```json
{
  "room_id": "room-123",
  "content": "Hello World",
  "user_id": "system",
  "username": "System",
  "type": "text", 
  "title": "Announcement",
  "payload": { "priority": "high" },
  "attachment_urls": ["https://example.com/file.pdf"]
}
```

### Socket.IO Events

Connect using any Socket.IO client (v2/v3/v4).

**Namespace:** `/socket.io/`

#### Client -> Server Events

1. **`authenticate`**
    - **Description**: Authenticate the socket connection with a user identity.
    - **Payload**:

        ```json
        {
          "user_id": "user-123",
          "username": "John Doe"
        }
        ```

2. **`join`**
    - **Description**: Join a specific chat room.
    - **Payload**:

        ```json
        {
          "room_id": "room-abc"
        }
        ```

3. **`leave`**
    - **Description**: Leave a chat room.
    - **Payload**:

        ```json
        {
          "room_id": "room-abc"
        }
        ```

4. **`message`**
    - **Description**: Send a message to a room.
    - **Payload**:

        ```json
        {
          "room_id": "room-abc",
          "content": "Hello team!",
          "type": "text",
          "title": "Optional Title",
          "payload": { "custom": "data" },
          "attachment_urls": []
        }
        ```

5. **`subscribe_status`**
    - **Description**: Subscribe to real-time status updates for a specific user.
    - **Payload**:

        ```json
        {
          "target_user_id": "user-456"
        }
        ```

6. **`unsubscribe_status`**
    - **Description**: Stop receiving status updates for a user.
    - **Payload**:

        ```json
        {
          "target_user_id": "user-456"
        }
        ```

#### Server -> Client Events

1. **`authenticated`**
    - **Description**: Confirmation of successful authentication.
    - **Payload**: User details (`user_id`, `username`).

2. **`message`**
    - **Description**: Received a new message in a joined room.
    - **Payload**: Full message object (id, content, sender info, timestamp, etc.).

3. **`user_online` / `user_offline`**
    - **Description**: Status update for a subscribed user.
    - **Payload**:

        ```json
        {
          "type": "user_online",
          "user_id": "user-456",
          "username": "Jane Doe",
          "is_online": true,
          "last_seen": "2023-10-27T10:00:00Z"
        }
        ```

4. **`room_users`**
    - **Description**: sent after joining a room, lists current members.
    - **Payload**: `{"room_id": "...", "users": ["id1", "id2"]}`

5. **`user_joined` / `user_left`**
    - **Description**: Notification when another user joins or leaves a room you are in.
    - **Payload**: `{"room_id": "...", "user_id": "...", "username": "..."}`

## 🚀 Deployment (CI/CD)

This repository includes deployment scripts:

- `deploy.sh`: Main deployment script.
- `generate-env.sh`: Helper to generate `.env` from CI environment variables.

See [deploy.sh](deploy.sh) for details.

## 📄 License

MIT
