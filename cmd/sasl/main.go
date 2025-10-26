package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"go-imap/internal/conf"
	"go-imap/internal/sasl"
)

func main() {
	// Command-line flags
	socketPath := flag.String("socket", "/var/spool/postfix/private/auth", "Path to UNIX socket")
	configPath := flag.String("config", "/etc/raven/raven.yaml", "Path to configuration file")
	flag.Parse()

	log.Println("Starting Raven SASL Authentication Service...")

	// Load configuration
	cfg, err := conf.LoadConfig()
	if err != nil {
		log.Printf("Warning: Failed to load config: %v", err)
		log.Println("Please ensure raven.yaml is configured with domain and auth_server_url")
		os.Exit(1)
	}

	// Validate configuration
	if cfg.Domain == "" {
		log.Fatal("Configuration error: domain is required")
	}
	if cfg.AuthServerURL == "" {
		log.Fatal("Configuration error: auth_server_url is required")
	}

	log.Printf("Configuration loaded:")
	log.Printf("  Socket path: %s", *socketPath)
	log.Printf("  Config path: %s", *configPath)
	log.Printf("  Domain: %s", cfg.Domain)
	log.Printf("  Auth URL: %s", cfg.AuthServerURL)

	// Create SASL server
	server := sasl.NewServer(*socketPath, cfg.AuthServerURL, cfg.Domain)

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start server in a goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- server.Start()
	}()

	// Wait for shutdown signal or error
	select {
	case err := <-errChan:
		if err != nil {
			log.Fatalf("Server error: %v", err)
		}
	case sig := <-sigChan:
		log.Printf("Received signal %v, shutting down gracefully...", sig)
		if err := server.Shutdown(); err != nil {
			log.Printf("Error during shutdown: %v", err)
		}
	}

	log.Println("Raven SASL Authentication Service stopped")
}
