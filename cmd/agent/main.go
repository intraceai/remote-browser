package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/intraceai/remote-browser/internal/api"
	"github.com/intraceai/remote-browser/internal/browser"
)

func main() {
	listenAddr := getEnv("LISTEN_ADDR", ":8082")

	log.Println("Starting browser...")
	b, err := browser.New()
	if err != nil {
		log.Fatalf("Failed to start browser: %v", err)
	}
	defer b.Close()

	server := api.NewServer(b)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down...")
		b.Close()
		os.Exit(0)
	}()

	log.Printf("Starting agent server on %s", listenAddr)
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
