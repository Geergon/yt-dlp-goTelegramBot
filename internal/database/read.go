package database

import (
	"database/sql"
	"fmt"
)

func GetAllWhitelist(db *sql.DB) ([]int64, []string, error) {
	rows, err := db.Query("SELECT user_id, username FROM whitelist")
	if err != nil {
		return nil, nil, fmt.Errorf("помилка запиту до БД: %w", err)
	}
	var allId []int64
	var usernames []string

	for rows.Next() {
		var id int64
		var username string
		err := rows.Scan(&id, &username)
		if err != nil {
			return nil, nil, err
		}
		allId = append(allId, id)
		usernames = append(usernames, username)
	}
	return allId, usernames, nil
}
