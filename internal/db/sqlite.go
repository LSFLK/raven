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

	return db, nil
}
