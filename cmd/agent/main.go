package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/intraceai/remote-browser/internal/api"
)

func main() {
	listenAddr := getEnv("LISTEN_ADDR", ":8082")

	log.Println("Starting browser agent...")
	server, err := api.NewServer()
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}
	defer server.Close()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down...")
		server.Close()
		os.Exit(0)
	}()

	log.Printf("Agent listening on %s", listenAddr)
	if err := server.Run(listenAddr); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
