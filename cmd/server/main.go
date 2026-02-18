package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/noersy/websocket-chat/config"
	"github.com/noersy/websocket-chat/internal/delivery/http"
	"github.com/noersy/websocket-chat/internal/delivery/websocket"
	"github.com/noersy/websocket-chat/internal/infrastructure/redis"
)

func main() {
	// Load configuration
	cfg := config.Load()

	// Initialize Redis
	redisClient, err := redis.NewClient(cfg.Redis)
	if err != nil {
		log.Fatalf("Failed to initialize redis: %v", err)
	}
	defer redisClient.Close()

	// Initialize WebSocket hub with Redis
	hub := websocket.NewHub(redisClient)
	hub.Run() // Starts the socket.io server and redis subscriber

	// Initialize HTTP server
	server := http.NewServer(cfg.Server, hub)

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("Server started on %s:%s", cfg.Server.Host, cfg.Server.Port)
		if err := server.Start(); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	<-quit
	log.Println("Shutting down server...")
	if err := server.Shutdown(); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}
}
