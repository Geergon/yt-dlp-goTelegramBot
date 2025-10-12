package database

import (
	"database/sql"
	"fmt"
)

func IsUserInWhitelist(db *sql.DB, userID int64) (bool, error) {
	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM whitelist WHERE user_id = ?)"

	err := db.QueryRow(query, userID).Scan(&exists)

	if err != nil && err != sql.ErrNoRows {
		return false, fmt.Errorf("помилка перевірки whitelist: %w", err)
	}

	return exists, nil
}
