package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"raven/internal/blobstorage"
	"raven/internal/db"
	"raven/internal/delivery/config"
	"raven/internal/delivery/lmtp"
)

func main() {
	// Command-line flags
	configPath := flag.String("config", "/etc/raven/delivery.yaml", "Path to configuration file")
	unixSocket := flag.String("socket", "/var/run/raven/lmtp.sock", "Path to UNIX socket")
	tcpAddr := flag.String("tcp", "", "TCP address to bind (e.g., 127.0.0.1:24 or :24)")
	dbPath := flag.String("db", "/app/data/databases", "Path to database directory")
	flag.Parse()

	log.Println("Starting Raven Delivery Service (LMTP)...")

	// Load configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Printf("Warning: Failed to load config from %s: %v", *configPath, err)
		log.Println("Using default configuration")
		cfg = config.DefaultConfig()
	}

	// Override config with command-line flags if provided
	if *unixSocket != "" {
		cfg.LMTP.UnixSocket = *unixSocket
	}
	if *tcpAddr != "" {
		cfg.LMTP.TCPAddress = *tcpAddr
	}
	if *dbPath != "" {
		cfg.Database.Path = *dbPath
	}

	// Initialize database manager
	dbManager, err := db.NewDBManager(cfg.Database.Path)
	if err != nil {
		log.Fatalf("Failed to initialize database manager: %v", err)
	}
	defer func() {
		if err := dbManager.Close(); err != nil {
			log.Printf("Error closing database manager: %v", err)
		}
	}()

	log.Printf("Database manager initialized: %s", cfg.Database.Path)

	// Initialize S3 blob storage if enabled
	var s3Storage *blobstorage.S3BlobStorage
	if cfg.BlobStorage.Enabled {
		log.Println("Initializing S3 blob storage...")
		s3Storage, err = blobstorage.NewS3BlobStorage(cfg.BlobStorage)
		if err != nil {
			log.Fatalf("Failed to initialize S3 blob storage: %v", err)
		}
		log.Printf("S3 blob storage initialized: %s (bucket: %s)", cfg.BlobStorage.Endpoint, cfg.BlobStorage.Bucket)
	} else {
		log.Println("S3 blob storage is disabled, using local SQLite storage")
	}

	// Create LMTP server with S3 storage
	server := lmtp.NewServerWithS3(dbManager, cfg, s3Storage)

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

	log.Println("Raven Delivery Service stopped")
}
