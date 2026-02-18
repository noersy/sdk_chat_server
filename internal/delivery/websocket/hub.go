package websocket

import (
	"context"
	"encoding/json"
	"html"
	"log"
	"strings"
	"sync"
	"time"

	socketio "github.com/doquangtan/gofiber-socket.io/v2"
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

type ConnectionState struct {
	UserID   string
	Username string
	Authed   bool
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
	io := socketio.New()

	h := &Hub{
		Io:             io,
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

// Close shuts down the server
func (h *Hub) Close() {
	if h.Io != nil {
		h.Io.Close()
	}
}

// PublishMessage publishes a message to Redis for cross-instance broadcast
func (h *Hub) PublishMessage(message []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	h.redisClient.Publish(ctx, channelMessage, message)
}

func (h *Hub) setupSocketIO() {
	h.Io.On("connection", func(s socketio.Socket) {
		log.Printf("Socket connected: %s", s.ID())
		s.SetContext(&ConnectionState{})
	})

	h.Io.On("authenticate", func(s socketio.Socket, msg interface{}) {
		state := s.Context().(*ConnectionState)
		if state.Authed {
			return
		}

		data, ok := msg.(map[string]interface{})
		if !ok {
			s.Emit("error", map[string]interface{}{
				"error": "invalid auth payload",
				"code":  "INVALID_PAYLOAD",
			})
			s.Disconnect()
			return
		}

		uid, _ := data["user_id"].(string)
		if uid == "" {
			s.Emit("error", map[string]interface{}{
				"error": "user_id is required",
				"code":  "MISSING_USER_ID",
			})
			s.Disconnect()
			return
		}

		uname, _ := data["username"].(string)
		if uname == "" {
			uname = "user_" + uid
		}

		// Update state
		state.UserID = uid
		state.Username = uname
		state.Authed = true
		s.SetContext(state)

		h.mu.Lock()
		if h.userSockets[uid] == nil {
			h.userSockets[uid] = make(map[string]bool)
		}
		h.userSockets[uid][s.ID()] = true
		h.mu.Unlock()

		h.statusMu.Lock()
		h.statusVersions[uid]++
		statusVersion := h.statusVersions[uid]
		h.statusMu.Unlock()
		go h.saveAndPublishStatus(uid, uname, true, statusVersion)

		s.Emit("authenticated", map[string]interface{}{
			"user_id":  uid,
			"username": uname,
		})

		log.Printf("Socket authenticated: %s (%s)", uname, uid)
	})

	h.Io.On("join", func(s socketio.Socket, msg interface{}) {
		state := s.Context().(*ConnectionState)
		if !state.Authed {
			s.Emit("error", map[string]interface{}{"error": "not authenticated", "code": "NOT_AUTHENTICATED"})
			return
		}

		data, ok := msg.(map[string]interface{})
		if !ok {
			return
		}
		roomID, _ := data["room_id"].(string)
		if roomID == "" {
			s.Emit("error", map[string]interface{}{"error": "room_id required", "code": "MISSING_ROOM_ID"})
			return
		}

		s.Join(roomID)

		ctx := context.Background()
		h.redisClient.SAdd(ctx, "room:members:"+roomID, state.UserID)

		joinedEvent := map[string]interface{}{
			"type":     "user_joined",
			"room_id":  roomID,
			"user_id":  state.UserID,
			"username": state.Username,
		}
		if msgBytes, err := json.Marshal(joinedEvent); err == nil {
			h.PublishMessage(msgBytes)
		}

		members, err := h.redisClient.SMembers(ctx, "room:members:"+roomID).Result()
		if err == nil {
			s.Emit("room_users", map[string]interface{}{
				"room_id": roomID,
				"users":   members,
			})
		}
	})

	h.Io.On("leave", func(s socketio.Socket, msg interface{}) {
		state := s.Context().(*ConnectionState)
		if !state.Authed {
			return
		}

		data, ok := msg.(map[string]interface{})
		if !ok {
			return
		}
		roomID, _ := data["room_id"].(string)
		if roomID == "" {
			return
		}

		s.Leave(roomID)

		ctx := context.Background()
		h.redisClient.SRem(ctx, "room:members:"+roomID, state.UserID)

		leftEvent := map[string]interface{}{
			"type":    "user_left",
			"room_id": roomID,
			"user_id": state.UserID,
		}
		if msgBytes, err := json.Marshal(leftEvent); err == nil {
			h.PublishMessage(msgBytes)
		}
	})

	h.Io.On("message", func(s socketio.Socket, msg interface{}) {
		state := s.Context().(*ConnectionState)
		if !state.Authed {
			s.Emit("error", map[string]interface{}{"error": "not authenticated", "code": "NOT_AUTHENTICATED"})
			return
		}

		data, ok := msg.(map[string]interface{})
		if !ok {
			s.Emit("error", map[string]interface{}{"error": "invalid payload format", "code": "INVALID_PAYLOAD"})
			return
		}

		roomID, _ := data["room_id"].(string)
		if roomID == "" {
			s.Emit("error", map[string]interface{}{"error": "room_id is required", "code": "MISSING_ROOM_ID"})
			return
		}

		// Verify socket has joined the room
		rooms := s.Rooms()
		inRoom := false
		for _, r := range rooms {
			if r == roomID {
				inRoom = true
				break
			}
		}
		if !inRoom {
			s.Emit("error", map[string]interface{}{
				"error": "you must join the room before sending messages",
				"code":  "NOT_ROOM_MEMBER",
			})
			return
		}

		content, _ := data["content"].(string)
		trimmed := strings.TrimSpace(content)
		if len(trimmed) < minMessageLength || len(trimmed) > maxMessageLength {
			s.Emit("error", map[string]interface{}{"error": "invalid message length", "code": "INVALID_CONTENT"})
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
			"user_id":         state.UserID,
			"username":        state.Username,
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

		s.Emit("message_ack", map[string]interface{}{
			"message_id": messageID,
			"timestamp":  timestamp,
		})
	})

	h.Io.On("subscribe_status", func(s socketio.Socket, msg interface{}) {
		state := s.Context().(*ConnectionState)
		if !state.Authed {
			return
		}

		data, ok := msg.(map[string]interface{})
		if !ok {
			return
		}
		targetUserID, _ := data["target_user_id"].(string)
		if targetUserID == "" {
			return
		}

		// Join virtual status room
		s.Join("status:" + targetUserID)

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

		s.Emit(eventType, StatusEvent{
			Type:      eventType,
			UserID:    targetUserID,
			Username:  vals["username"],
			LastSeen:  vals["last_seen"],
			IsOnline:  isOnline,
			Timestamp: time.Now().Format(time.RFC3339),
		})
	})

	h.Io.On("unsubscribe_status", func(s socketio.Socket, msg interface{}) {
		data, ok := msg.(map[string]interface{})
		if !ok {
			return
		}
		targetUserID, _ := data["target_user_id"].(string)
		if targetUserID != "" {
			s.Leave("status:" + targetUserID)
		}
	})

	h.Io.On("disconnect", func(s socketio.Socket) {
		state, ok := s.Context().(*ConnectionState)
		if !ok || !state.Authed {
			return
		}

		// Clean up Redis room membership for all rooms this socket was in
		ctx := context.Background()
		for _, roomID := range s.Rooms() {
			if strings.HasPrefix(roomID, "status:") {
				continue
			}
			h.redisClient.SRem(ctx, "room:members:"+roomID, state.UserID)

			leftEvent := map[string]interface{}{
				"type":    "user_left",
				"room_id": roomID,
				"user_id": state.UserID,
			}
			if msgBytes, err := json.Marshal(leftEvent); err == nil {
				h.PublishMessage(msgBytes)
			}
		}

		// Remove this socket from the user's active socket set.
		h.mu.Lock()
		lastSocket := false
		if sockets, ok := h.userSockets[state.UserID]; ok {
			delete(sockets, s.ID())
			if len(sockets) == 0 {
				delete(h.userSockets, state.UserID)
				lastSocket = true
			}
		}
		h.mu.Unlock()

		if lastSocket {
			h.statusMu.Lock()
			h.statusVersions[state.UserID]++
			statusVersion := h.statusVersions[state.UserID]
			h.statusMu.Unlock()
			go h.saveAndPublishStatus(state.UserID, state.Username, false, statusVersion)
		}
		log.Printf("Socket disconnected: %s (%s) lastSocket=%v", state.Username, state.UserID, lastSocket)
	})
}

func (h *Hub) subscribeToRedis() {
	if h.redisClient == nil {
		return
	}
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

	// Broadcast to room using the Fiber Socket.IO library
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
