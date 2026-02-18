# WebSocket Chat Application - Backend

A production-ready real-time chat application built with Go 1.24+, WebSockets, and PostgreSQL. This backend provides a robust REST API for user and room management alongside a high-performance WebSocket server for real-time messaging.

## 🚀 Features

### Core

- **Clean Architecture**: Separation of concerns (Domain, UseCase, Repository, Delivery).
- **RESTful API**: comprehensive endpoints for users, rooms, and messages.
- **Real-time Messaging**: Low-latency WebSocket communication.
- **Database**: PostgreSQL with automatic schema migration and relational integrity.

### User Management

- ✅ User Registration & Login
- ✅ Profile Management (Update details, Change password)
- ✅ Secure Password Storage (Bcrypt hashing)
- ✅ Account Deletion (Cascading)

### Room Management

- ✅ Create/Delete Rooms (Group & Private)
- ✅ Manage Members (Add/Remove users)
- ✅ List User's Rooms
- ✅ Room Details & Updates

### Messaging

- ✅ Real-time message broadcasting
- ✅ Message History (Paginated REST API)
- ✅ Message Status (Sent, Delivered, Read)
- ✅ User Online/Offline Status

## 🏗 Architecture

This project follows Clean Architecture and Domain-Driven Design (DDD) principles:

```
cmd/server/          - Application entry point
config/              - Configuration management
internal/
  ├── domain/        - Business entities (User, Room, Message) & Repository interfaces
  ├── usecase/       - Business logic (UserUC, RoomUC, ChatUC)
  ├── delivery/      - Transport layer
  │   ├── http/      - REST handlers & DTOs
  │   └── websocket/ - WebSocket handlers & Hub
  └── infrastructure/- Database implementations (PostgreSQL)
pkg/utils/           - Utility functions (Password hash, UUID)
```

## 🛠 Prerequisites

- Go 1.24 or higher
- PostgreSQL 12 or higher
- Git

## 📦 Setup Instructions

### 1. Clone the Repository

```bash
git clone <repository-url>
cd backend
```

### 2. Install Dependencies

```bash
go mod download
```

### 3. Configure Environment Variables

Copy the example configuration:

```bash
cp .env.example .env
```

Edit `.env` with your local configuration:

```env
# Server Configuration
SERVER_PORT=8080
SERVER_HOST=0.0.0.0

# Database Configuration
# Ensure these match your local PostgreSQL setup
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=your_password
DB_NAME=chat_db
```

### 4. Create Database

```bash
psql -U postgres -c "CREATE DATABASE chat_db;"
```

### 5. Run the Server

```bash
go run ./cmd/server/main.go
```

The server will automatically run migrations to create all required tables on startup.

**Server URL:** `http://localhost:8080`

## 📚 API Documentation

The backend provides a complete set of REST endpoints.

👉 **[See Full API Documentation](../API_ENDPOINTS.md)**

### Quick Overview

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/users` | Register new user |
| POST | `/api/auth/login` | User login |
| POST | `/api/rooms` | Create new room |
| GET | `/api/rooms/{id}/messages` | Get message history |
| GET | `/health` | Server health check |

## 🔌 WebSocket API

Connect to the WebSocket endpoint for real-time events.

**URL:** `ws://localhost:8080/ws?user_id=<UUID>&username=<NAME>`

### Event Types

- `join` / `leave`: Room presence
- `message`: Sending text messages
- `message_delivered` / `message_read`: Read receipts
- `user_status`: Online/Offline updates

## 🧪 Development

### Run Tests

```bash
go test ./...
```

### Code Formatting

```bash
go fmt ./...
```

## 📝 Roadmap

- [x] Basic REST API (CRUD)
- [x] WebSocket Chat
- [x] Message History & Pagination
- [x] User & Room Management
- [x] Message Status (Sent/Delivered/Read)
- [ ] JWT Authentication Middleware
- [ ] Push Notifications
- [ ] File/Image Uploads
- [ ] Redis Adapter for Horizontal Scaling

## 📄 License

MIT
