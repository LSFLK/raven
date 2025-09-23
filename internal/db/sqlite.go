package db

import (
	"database/sql"
	"log"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func InitDB(file string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", file)
	if err != nil {
		return nil, err
	}

	schema := `
	CREATE TABLE IF NOT EXISTS mails (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		subject TEXT,
		sender TEXT,
		recipient TEXT,
		date_sent TEXT,
		raw_message TEXT,
		flags TEXT DEFAULT '',
		folder TEXT DEFAULT 'INBOX'
	);

	CREATE TABLE IF NOT EXISTS folders (
		name TEXT PRIMARY KEY,
		delimiter TEXT DEFAULT '/',
		attributes TEXT DEFAULT ''
	);

	INSERT OR IGNORE INTO folders (name) VALUES
		('INBOX'),
		('Sent'),
		('Drafts'),
		('Trash');
	`
	if _, err = db.Exec(schema); err != nil {
		return nil, err
	}

	// Insert test mails if DB empty
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM mails").Scan(&count)
	if err != nil {
		return nil, err
	}

	if count == 0 {
		testMails := []struct {
			subject, sender, recipient, body string
		}{
			{
				"Welcome to SQLite IMAP",
				"admin@example.com",
				"user@example.com",
				"From: admin@example.com\r\n" +
					"To: user@example.com\r\n" +
					"Subject: Welcome to SQLite IMAP\r\n" +
					"Date: " + time.Now().Format(time.RFC1123Z) + "\r\n" +
					"\r\nHello user@example.com,\r\nThis is your first test mail!\r\n",
			},
			{
				"Meeting Reminder",
				"boss@example.com",
				"user@example.com",
				"From: boss@example.com\r\n" +
					"To: user@example.com\r\n" +
					"Subject: Meeting Reminder\r\n" +
					"Date: " + time.Now().Add(-2*time.Hour).Format(time.RFC1123Z) + "\r\n" +
					"\r\nDon’t forget our meeting at 3PM today.\r\n",
			},
		}

		for _, m := range testMails {
			_, err = db.Exec(
				"INSERT INTO mails (subject, sender, recipient, date_sent, raw_message, folder) VALUES (?, ?, ?, ?, ?, ?)",
				m.subject, m.sender, m.recipient, time.Now().Format(time.RFC3339), m.body, "INBOX",
			)
			if err != nil {
				return nil, err
			}
		}
		log.Println("Inserted sample mails for user@example.com")
	}

	return db, nil
}
