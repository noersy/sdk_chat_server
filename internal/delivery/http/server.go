package http

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/noersy/websocket-chat/config"
	"github.com/noersy/websocket-chat/internal/delivery/websocket"
)

type Server struct {
	app        *fiber.App
	addr       string
	hub        *websocket.Hub
	httpServer *http.Server
	restAddr   string
}

func NewServer(cfg config.ServerConfig, hub *websocket.Hub) *Server {
	app := fiber.New(fiber.Config{
		DisableStartupMessage: false,
	})

	handler := NewHandler(hub)
	handler.SetupRoutes(app)

	return &Server{
		app:      app,
		addr:     fmt.Sprintf("%s:%s", cfg.SocketIOHost, cfg.SocketIOPort),
		restAddr: fmt.Sprintf("%s:%s", cfg.Host, cfg.Port),
		hub:      hub,
	}
}

func (s *Server) Start() error {
	// Start Socket.IO on native HTTP server
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/socket.io/", s.hub.Server)

		s.httpServer = &http.Server{
			Addr:    s.addr,
			Handler: mux,
		}

		log.Printf("Socket.IO server starting on %s", s.addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Socket.IO HTTP server error: %v", err)
		}
	}()

	// Start Fiber REST API on separate port
	return s.app.Listen(s.restAddr)
}

func (s *Server) Shutdown() error {
	if s.httpServer != nil {
		if err := s.httpServer.Close(); err != nil {
			log.Printf("Error closing HTTP server: %v", err)
		}
	}
	return s.app.Shutdown()
}
