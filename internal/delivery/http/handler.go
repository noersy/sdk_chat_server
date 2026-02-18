package http

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	ws "github.com/noersy/websocket-chat/internal/delivery/websocket"
)

type Handler struct {
	hub *ws.Hub
}

func NewHandler(hub *ws.Hub) *Handler {
	return &Handler{hub: hub}
}

func (h *Handler) SetupRoutes(app *fiber.App) {
	// Global middleware
	app.Use(cors.New(cors.Config{
		AllowOriginsFunc: func(origin string) bool {
			return true
		},
		AllowMethods:     "GET,POST,OPTIONS",
		AllowHeaders:     "Accept,Content-Type,Authorization,X-Requested-With,Origin",
		AllowCredentials: true,
	}))
	app.Use(logger.New())

	// Health check
	app.Get("/health", h.healthCheck)

	// REST → WebSocket bridge
	app.Post("/messages", h.handleBroadcast)
}

func (h *Handler) healthCheck(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"status": "ok"})
}

type BroadcastRequest struct {
	RoomID         string                 `json:"room_id"`
	Content        string                 `json:"content"`
	Type           string                 `json:"type"`
	Title          string                 `json:"title"`
	UserID         string                 `json:"user_id"`
	Username       string                 `json:"username"`
	Payload        map[string]interface{} `json:"payload,omitempty"`
	AttachmentURLs []string               `json:"attachment_urls,omitempty"`
}

func (h *Handler) handleBroadcast(c *fiber.Ctx) error {
	var req BroadcastRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if req.RoomID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "room_id is required"})
	}

	if req.Type == "" {
		req.Type = "text"
	}
	if req.UserID == "" {
		req.UserID = "system"
	}
	if req.Username == "" {
		req.Username = "System"
	}

	msg := map[string]interface{}{
		"type":            "message",
		"room_id":         req.RoomID,
		"id":              fmt.Sprintf("%d", time.Now().UnixNano()),
		"user_id":         req.UserID,
		"username":        req.Username,
		"content":         req.Content,
		"title":           req.Title,
		"created_at":      time.Now().Format(time.RFC3339),
		"payload":         req.Payload,
		"attachment_urls": req.AttachmentURLs,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Internal server error"})
	}

	h.hub.PublishMessage(data)

	return c.JSON(fiber.Map{"status": "sent"})
}
