package database

import (
	"database/sql"
	"fmt"
)

func IsUserWhitelisted(db *sql.DB, userID int64) (bool, error) {
	var exists bool
	// Запит, який повертає 1 (true) або 0 (false)
	query := "SELECT EXISTS(SELECT 1 FROM whitelist WHERE user_id = ?)"

	// QueryRow.Scan ефективно отримує результат EXISTS
	err := db.QueryRow(query, userID).Scan(&exists)

	if err != nil && err != sql.ErrNoRows {
		return false, fmt.Errorf("помилка перевірки whitelist: %w", err)
	}

	return exists, nil
}
