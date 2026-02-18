package websocket

import (
	"context"
	"encoding/json"
	"html"
	"log"
	"strings"
	"sync"
	"time"

	socketio "github.com/doquangtan/gofiber-socket.io"
	"github.com/noersy/websocket-chat/pkg/utils"
	"github.com/redis/go-redis/v9"
)

const (
	channelMessage   = "chat:message"
	channelStatus    = "chat:status"
	maxMessageLength = 10000
	minMessageLength = 1
)

type StatusEvent struct {
	Type      string `json:"type"`
	UserID    string `json:"user_id"`
	Username  string `json:"username,omitempty"`
	LastSeen  string `json:"last_seen,omitempty"`
	IsOnline  bool   `json:"is_online"`
	Timestamp string `json:"timestamp,omitempty"`
}

type Hub struct {
	Io             *socketio.Io
	redisClient    *redis.Client
	mu             sync.RWMutex
	userSockets    map[string]map[string]bool // userID -> set of socket IDs
	statusMu       sync.Mutex
	statusVersions map[string]int64 // userID -> monotonic version counter
}

func NewHub(redisClient *redis.Client) *Hub {
	h := &Hub{
		Io:             socketio.New(),
		redisClient:    redisClient,
		userSockets:    make(map[string]map[string]bool),
		statusVersions: make(map[string]int64),
	}
	h.setupSocketIO()
	return h
}

// Run starts the Redis PubSub listener for cross-instance broadcasting
func (h *Hub) Run() {
	go h.subscribeToRedis()
}

// PublishMessage publishes a message to Redis for cross-instance broadcast
func (h *Hub) PublishMessage(message []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	h.redisClient.Publish(ctx, channelMessage, message)
}

func (h *Hub) setupSocketIO() {
	h.Io.OnConnection(func(socket *socketio.Socket) {
		// Per-connection state captured by closures.
		// stateMu guards userID/username/authed/socketRooms since they are
		// accessed from both the library's read() goroutine (for regular events)
		// and the WebSocket goroutine (for disconnect).
		var (
			userID      string
			username    string
			authed      bool
			socketRooms = make(map[string]bool) // rooms joined by this socket
			stateMu     sync.Mutex
		)

		// --- authenticate ---
		socket.On("authenticate", func(event *socketio.EventPayload) {
			stateMu.Lock()
			defer stateMu.Unlock()

			if authed {
				return
			}

			if len(event.Data) == 0 {
				socket.Emit("error", map[string]interface{}{
					"error": "user_id is required",
					"code":  "AUTH_REQUIRED",
				})
				socket.Disconnect()
				return
			}

			data, ok := event.Data[0].(map[string]interface{})
			if !ok {
				socket.Emit("error", map[string]interface{}{
					"error": "invalid auth payload",
					"code":  "INVALID_PAYLOAD",
				})
				socket.Disconnect()
				return
			}

			uid, _ := data["user_id"].(string)
			if uid == "" {
				socket.Emit("error", map[string]interface{}{
					"error": "user_id is required",
					"code":  "MISSING_USER_ID",
				})
				socket.Disconnect()
				return
			}

			uname, _ := data["username"].(string)
			if uname == "" {
				uname = "user_" + uid
			}

			userID = uid
			username = uname
			authed = true

			h.mu.Lock()
			if h.userSockets[userID] == nil {
				h.userSockets[userID] = make(map[string]bool)
			}
			h.userSockets[userID][socket.Id] = true
			h.mu.Unlock()

			h.statusMu.Lock()
			h.statusVersions[userID]++
			statusVersion := h.statusVersions[userID]
			h.statusMu.Unlock()
			go h.saveAndPublishStatus(userID, username, true, statusVersion)

			socket.Emit("authenticated", map[string]interface{}{
				"user_id":  userID,
				"username": username,
			})

			log.Printf("Socket authenticated: %s (%s)", username, userID)
		})

		// --- join ---
		socket.On("join", func(event *socketio.EventPayload) {
			stateMu.Lock()
			uid, uname, isAuthed := userID, username, authed
			stateMu.Unlock()

			if !isAuthed {
				socket.Emit("error", map[string]interface{}{"error": "not authenticated", "code": "NOT_AUTHENTICATED"})
				return
			}
			if len(event.Data) == 0 {
				socket.Emit("error", map[string]interface{}{"error": "room_id required", "code": "INVALID_REQUEST"})
				return
			}

			data, ok := event.Data[0].(map[string]interface{})
			if !ok {
				return
			}
			roomID, _ := data["room_id"].(string)
			if roomID == "" {
				socket.Emit("error", map[string]interface{}{"error": "room_id required", "code": "MISSING_ROOM_ID"})
				return
			}

			socket.Join(roomID)

			// Track room locally so disconnect handler can clean up
			stateMu.Lock()
			socketRooms[roomID] = true
			stateMu.Unlock()

			ctx := context.Background()
			h.redisClient.SAdd(ctx, "room:members:"+roomID, uid)

			joinedEvent := map[string]interface{}{
				"type":     "user_joined",
				"room_id":  roomID,
				"user_id":  uid,
				"username": uname,
			}
			if msgBytes, err := json.Marshal(joinedEvent); err == nil {
				h.PublishMessage(msgBytes)
			}

			members, err := h.redisClient.SMembers(ctx, "room:members:"+roomID).Result()
			if err == nil {
				socket.Emit("room_users", map[string]interface{}{
					"room_id": roomID,
					"users":   members,
				})
			}
		})

		// --- leave ---
		socket.On("leave", func(event *socketio.EventPayload) {
			stateMu.Lock()
			uid, isAuthed := userID, authed
			stateMu.Unlock()

			if !isAuthed || len(event.Data) == 0 {
				return
			}

			data, ok := event.Data[0].(map[string]interface{})
			if !ok {
				return
			}
			roomID, _ := data["room_id"].(string)
			if roomID == "" {
				return
			}

			socket.Leave(roomID)

			// Remove from local room tracking
			stateMu.Lock()
			delete(socketRooms, roomID)
			stateMu.Unlock()

			ctx := context.Background()
			h.redisClient.SRem(ctx, "room:members:"+roomID, uid)

			leftEvent := map[string]interface{}{
				"type":    "user_left",
				"room_id": roomID,
				"user_id": uid,
			}
			if msgBytes, err := json.Marshal(leftEvent); err == nil {
				h.PublishMessage(msgBytes)
			}
		})

		// --- message ---
		socket.On("message", func(event *socketio.EventPayload) {
			stateMu.Lock()
			uid, uname, isAuthed := userID, username, authed
			stateMu.Unlock()

			if !isAuthed {
				socket.Emit("error", map[string]interface{}{"error": "not authenticated", "code": "NOT_AUTHENTICATED"})
				return
			}
			if len(event.Data) == 0 {
				socket.Emit("error", map[string]interface{}{"error": "invalid payload", "code": "INVALID_PAYLOAD"})
				return
			}

			data, ok := event.Data[0].(map[string]interface{})
			if !ok {
				socket.Emit("error", map[string]interface{}{"error": "invalid payload format", "code": "INVALID_PAYLOAD"})
				return
			}

			roomID, _ := data["room_id"].(string)
			if roomID == "" {
				socket.Emit("error", map[string]interface{}{"error": "room_id is required", "code": "MISSING_ROOM_ID"})
				return
			}

			// Verify socket has joined the room
			inRoom := false
			for _, r := range socket.Rooms() {
				if r == roomID {
					inRoom = true
					break
				}
			}
			if !inRoom {
				socket.Emit("error", map[string]interface{}{
					"error": "you must join the room before sending messages",
					"code":  "NOT_ROOM_MEMBER",
				})
				return
			}

			content, _ := data["content"].(string)
			trimmed := strings.TrimSpace(content)
			if len(trimmed) < minMessageLength || len(trimmed) > maxMessageLength {
				socket.Emit("error", map[string]interface{}{"error": "invalid message length", "code": "INVALID_CONTENT"})
				return
			}
			safeContent := html.EscapeString(trimmed)

			msgType, _ := data["type"].(string)
			if msgType == "" {
				msgType = "text"
			}

			messageID := utils.GenerateID()
			timestamp := time.Now()

			broadcastMsg := map[string]interface{}{
				"type":            "message",
				"id":              messageID,
				"room_id":         roomID,
				"user_id":         uid,
				"username":        uname,
				"content":         safeContent,
				"message_type":    msgType,
				"title":           data["title"],
				"payload":         data["payload"],
				"attachment_urls": data["attachment_urls"],
				"created_at":      timestamp,
			}

			msgBytes, err := json.Marshal(broadcastMsg)
			if err != nil {
				return
			}

			h.PublishMessage(msgBytes)

			socket.Emit("message_ack", map[string]interface{}{
				"message_id": messageID,
				"timestamp":  timestamp,
			})
		})

		// --- subscribe_status ---
		socket.On("subscribe_status", func(event *socketio.EventPayload) {
			stateMu.Lock()
			isAuthed := authed
			stateMu.Unlock()

			if !isAuthed || len(event.Data) == 0 {
				return
			}

			data, ok := event.Data[0].(map[string]interface{})
			if !ok {
				return
			}
			targetUserID, _ := data["target_user_id"].(string)
			if targetUserID == "" {
				return
			}

			// Join virtual status room
			socket.Join("status:" + targetUserID)

			// Send current status snapshot from Redis
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			vals, err := h.redisClient.HGetAll(ctx, "user:status:"+targetUserID).Result()
			if err != nil {
				return
			}

			isOnline := vals["online"] == "true"
			eventType := "user_offline"
			if isOnline {
				eventType = "user_online"
			}

			socket.Emit(eventType, StatusEvent{
				Type:      eventType,
				UserID:    targetUserID,
				Username:  vals["username"],
				LastSeen:  vals["last_seen"],
				IsOnline:  isOnline,
				Timestamp: time.Now().Format(time.RFC3339),
			})
		})

		// --- unsubscribe_status ---
		socket.On("unsubscribe_status", func(event *socketio.EventPayload) {
			if len(event.Data) == 0 {
				return
			}
			data, ok := event.Data[0].(map[string]interface{})
			if !ok {
				return
			}
			targetUserID, _ := data["target_user_id"].(string)
			if targetUserID != "" {
				socket.Leave("status:" + targetUserID)
			}
		})

		// --- disconnect ---
		socket.On("disconnect", func(event *socketio.EventPayload) {
			stateMu.Lock()
			uid, uname, isAuthed := userID, username, authed
			// Snapshot rooms before releasing lock; library already cleared
			// socket.io rooms, so we rely on our own tracking map.
			rooms := make(map[string]bool, len(socketRooms))
			for r := range socketRooms {
				rooms[r] = true
			}
			stateMu.Unlock()

			if !isAuthed {
				return
			}

			// Clean up Redis room membership for all rooms this socket was in
			ctx := context.Background()
			for roomID := range rooms {
				h.redisClient.SRem(ctx, "room:members:"+roomID, uid)

				leftEvent := map[string]interface{}{
					"type":    "user_left",
					"room_id": roomID,
					"user_id": uid,
				}
				if msgBytes, err := json.Marshal(leftEvent); err == nil {
					h.PublishMessage(msgBytes)
				}
			}

			// Remove this socket from the user's active socket set.
			// Only set user offline when this was their last socket.
			h.mu.Lock()
			lastSocket := false
			if sockets, ok := h.userSockets[uid]; ok {
				delete(sockets, socket.Id)
				if len(sockets) == 0 {
					delete(h.userSockets, uid)
					lastSocket = true
				}
			}
			h.mu.Unlock()

			if lastSocket {
				h.statusMu.Lock()
				h.statusVersions[uid]++
				statusVersion := h.statusVersions[uid]
				h.statusMu.Unlock()
				go h.saveAndPublishStatus(uid, uname, false, statusVersion)
			}
			log.Printf("Socket disconnected: %s (%s) lastSocket=%v", uname, uid, lastSocket)
		})
	})
}

func (h *Hub) subscribeToRedis() {
	ctx := context.Background()
	pubsub := h.redisClient.Subscribe(ctx, channelMessage, channelStatus)
	defer pubsub.Close()

	for msg := range pubsub.Channel() {
		switch msg.Channel {
		case channelMessage:
			h.broadcastMessage([]byte(msg.Payload))
		case channelStatus:
			var event StatusEvent
			if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
				log.Printf("error unmarshaling status event: %v", err)
				continue
			}
			h.dispatchStatusEvent(event)
		}
	}
}

func (h *Hub) broadcastMessage(message []byte) {
	var msg struct {
		RoomID string `json:"room_id"`
		Type   string `json:"type"`
	}
	if err := json.Unmarshal(message, &msg); err != nil || msg.RoomID == "" {
		return
	}

	var data interface{}
	if err := json.Unmarshal(message, &data); err != nil {
		return
	}

	eventName := msg.Type
	if eventName == "" {
		eventName = "message"
	}

	h.Io.To(msg.RoomID).Emit(eventName, data)
}

func (h *Hub) dispatchStatusEvent(event StatusEvent) {
	h.Io.To("status:"+event.UserID).Emit(event.Type, event)
}

func (h *Hub) saveAndPublishStatus(userID, username string, isOnline bool, version int64) {
	// Skip if a newer status update has been enqueued (race condition guard).
	h.statusMu.Lock()
	if h.statusVersions[userID] != version {
		h.statusMu.Unlock()
		return
	}
	h.statusMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now().Format(time.RFC3339)
	onlineStr := "false"
	if isOnline {
		onlineStr = "true"
	}

	statusKey := "user:status:" + userID
	err := h.redisClient.HSet(ctx, statusKey,
		"online", onlineStr,
		"last_seen", now,
		"username", username,
	).Err()
	if err != nil {
		log.Printf("error saving user status: %v", err)
	}

	// TTL: online status expires in 1 h (refreshed on reconnect); offline kept 7 days for last_seen.
	ttl := 7 * 24 * time.Hour
	if isOnline {
		ttl = time.Hour
	}
	h.redisClient.Expire(ctx, statusKey, ttl)

	eventType := "user_offline"
	if isOnline {
		eventType = "user_online"
	}

	event := StatusEvent{
		Type:      eventType,
		UserID:    userID,
		Username:  username,
		LastSeen:  now,
		IsOnline:  isOnline,
		Timestamp: now,
	}

	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	if err := h.redisClient.Publish(ctx, channelStatus, data).Err(); err != nil {
		log.Printf("error publishing status: %v", err)
	}
}
