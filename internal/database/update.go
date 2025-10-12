package database

import "database/sql"

func InsertIntoWhitelist(db *sql.DB, username string, id int64) error {
	_, err := db.Exec(`INSERT OR IGNORE INTO whitelist (user_id, username) VALUES (?, ?)`, id, username)
	if err != nil {
		return err
	}
	return nil
}
