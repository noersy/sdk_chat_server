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
	restAddr   string
	socketAddr string
	socketSrv  *http.Server
	hub        *websocket.Hub
}

func NewServer(cfg config.ServerConfig, hub *websocket.Hub) *Server {
	app := fiber.New(fiber.Config{
		DisableStartupMessage: false,
	})

	handler := NewHandler(hub)
	handler.SetupRoutes(app)

	return &Server{
		app:        app,
		restAddr:   fmt.Sprintf("%s:%s", cfg.Host, cfg.Port),
		socketAddr: fmt.Sprintf("%s:%s", cfg.Host, cfg.SocketIOPort),
		hub:        hub,
	}
}

type responseWriterWrapper struct {
	http.ResponseWriter
	headerWritten bool
}

func (w *responseWriterWrapper) WriteHeader(statusCode int) {
	if !w.headerWritten {
		w.headerWritten = true
		w.ResponseWriter.WriteHeader(statusCode)
	}
}

func (s *Server) Start() error {
	// Start Socket.IO server
	mux := http.NewServeMux()

	// Wrap to handle CORS and prevent duplicate WriteHeader calls
	mux.Handle("/socket.io/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add CORS headers for cross-origin WebSocket connections
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Wrap response writer to prevent duplicate WriteHeader calls from Socket.IO library
		wrapped := &responseWriterWrapper{ResponseWriter: w, headerWritten: false}
		s.hub.Server.ServeHTTP(wrapped, r)
	}))

	s.socketSrv = &http.Server{
		Addr:    s.socketAddr,
		Handler: mux,
	}

	go func() {
		log.Printf("Socket.IO server started on %s", s.socketAddr)
		if err := s.socketSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Socket.IO server error: %v", err)
		}
	}()

	// Start REST API server
	return s.app.Listen(s.restAddr)
}

func (s *Server) Shutdown() error {
	if s.socketSrv != nil {
		s.socketSrv.Close()
	}
	return s.app.Shutdown()
}
