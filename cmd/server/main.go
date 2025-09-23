package main

import (
	"log"
	"net"

	"go-imap/internal/db"
	"go-imap/internal/server"
)

const SERVER_IP = "0.0.0.0:143"

func main() {
	log.Println("Starting SQLite IMAP server (no-auth mode)...")

	// Init DB
	database, err := db.InitDB("mails.db")
	if err != nil {
		log.Fatal("Failed to initialize database:", err)
	}

	imapServer := server.NewIMAPServer(database)

	ln, err := net.Listen("tcp", SERVER_IP)
	if err != nil {
		log.Fatal("Failed to start TCP listener:", err)
	}
	defer ln.Close()

	log.Printf("SQLite IMAP server running on %s", SERVER_IP)
	log.Println("Configure your email client with:")
	log.Println("  Server: localhost (or container IP)")
	log.Println("  Port: 143")
	log.Println("  Security: None")
	log.Println("  Username: anything")
	log.Println("  Password: anything")

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println("Accept error:", err)
			continue
		}

		log.Printf("New connection from: %s", conn.RemoteAddr())
		go imapServer.HandleConnection(conn)
	}
}
