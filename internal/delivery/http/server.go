package http

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/noersy/websocket-chat/config"
	"github.com/noersy/websocket-chat/internal/delivery/websocket"
)

type Server struct {
	app  *fiber.App
	addr string
	hub  *websocket.Hub
}

func NewServer(cfg config.ServerConfig, hub *websocket.Hub) *Server {
	app := fiber.New(fiber.Config{
		DisableStartupMessage: false,
	})

	handler := NewHandler(hub)
	handler.SetupRoutes(app)

	return &Server{
		app:  app,
		addr: fmt.Sprintf("%s:%s", cfg.Host, cfg.Port),
		hub:  hub,
	}
}

func (s *Server) Start() error {
	return s.app.Listen(s.addr)
}

func (s *Server) Shutdown() error {
	return s.app.Shutdown()
}
