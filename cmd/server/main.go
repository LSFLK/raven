package main

import (
	"log"
	"net"

	"go-imap/internal/db"
	"go-imap/internal/server"
)

const SERVER_IP = "0.0.0.0:143"
const SERVER_IP_SSL = "0.0.0.0:993"

func main() {
	log.Println("Starting SQLite IMAP server (no-auth mode)...")

	// Init DB
	database, err := db.InitDB("data/mails.db")
	if err != nil {
		log.Fatal("Failed to initialize database:", err)
	}

	imapServer := server.NewIMAPServer(database)

	// Start plain IMAP (143)
	go func() {
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
	}()

	// Start IMAPS (993)
	lnSSL, err := net.Listen("tcp", SERVER_IP_SSL)
	if err != nil {
		log.Fatal("Failed to start SSL TCP listener:", err)
	}
	defer lnSSL.Close()

	log.Printf("SQLite IMAPS server running on %s", SERVER_IP_SSL)
	log.Println("Configure your email client with:")
	log.Println("  Server: localhost (or container IP)")
	log.Println("  Port: 993")
	log.Println("  Security: SSL/TLS")
	log.Println("  Username: anything")
	log.Println("  Password: anything")

	for {
		conn, err := lnSSL.Accept()
		if err != nil {
			log.Println("Accept error:", err)
			continue
		}

		log.Printf("New SSL connection from: %s", conn.RemoteAddr())
		go imapServer.HandleSSLConnection(conn)
	}
}