package main

import (
	"database/sql"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"io"
)

type Database interface {
	io.Closer

	CreateTableStatus() error

	InsertStatus(tweetID int64, tootID string) error
	GetLastestTweetID() (latestID int64, err error)
	GetStatusByTweetID(tweetID int64) (tootID string, err error)
	GetStatusByTootID(tootID string) (tweetID int64, err error)

	CountStatus() (count int)
}

type TwtlDatabase struct {
	*sql.DB
}

func (db *TwtlDatabase) CreateTableStatus() error {
	sqlStmt := `
		CREATE TABLE IF NOT EXISTS status(
			id 				INTEGER PRIMARY KEY AUTOINCREMENT,
			tweet_id		INTEGER NOT NULL UNIQUE,
			toot_id			VARCHAR(255) NOT NULL,
			inserted_at		DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`
	_, err := db.Exec(sqlStmt)
	return err
}

func (db *TwtlDatabase) InsertStatus(tweetID int64, tootID string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.Exec("INSERT INTO status(tweet_id, toot_id) VALUES(?, ?);", tweetID, tootID)
	if err != nil {
		return err
	}

	_, err = res.LastInsertId()
	if err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (db *TwtlDatabase) GetLastestTweetID() (latestID int64, err error) {
	err = db.QueryRow("SELECT tweet_id FROM status ORDER BY tweet_id DESC;").Scan(&latestID)
	if err != nil {
		return 0, err
	}
	return latestID, nil
}

func (db *TwtlDatabase) GetStatusByTweetID(tweetID int64) (tootID string, err error) {
	err = db.QueryRow("SELECT toot_id FROM status WHERE tweet_id = ?;", tweetID).Scan(&tootID)
	if err != nil {
		return "", err
	}
	return tootID, nil
}

func (db *TwtlDatabase) GetStatusByTootID(tootID string) (tweetID int64, err error) {
	err = db.QueryRow("SELECT tweet_id FROM status WHERE toot_id = ?;", tootID).Scan(&tweetID)
	if err != nil {
		return 0, err
	}
	return tweetID, nil
}

func (db *TwtlDatabase) CountStatus() (count int) {
	err := db.QueryRow("SELECT COUNT(*) FROM status").Scan(&count)
	if err != nil {
		return 0
	}
	return count
}

func NewDatabase(target string) (*TwtlDatabase, error) {
	db, err := sql.Open("sqlite3", fmt.Sprintf("%s", target))
	if err != nil {
		return nil, err
	}
	if err = db.Ping(); err != nil {
		return nil, err
	}
	return &TwtlDatabase{db}, nil
}
