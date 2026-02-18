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

		// Wrap Socket.IO handler to ensure proper polling transport
		mux.HandleFunc("/socket.io/", func(w http.ResponseWriter, r *http.Request) {
			// Add CORS headers first
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "OPTIONS, GET, POST")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

			// Handle OPTIONS requests for CORS
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			// Log polling requests
			transport := r.URL.Query().Get("transport")
			if transport == "polling" {
				log.Printf("Polling request: %s %s", r.Method, r.URL.String())
				// Ensure text-based response for polling
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			}

			s.hub.Server.ServeHTTP(w, r)
		})

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
