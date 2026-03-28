package database

import (
	"database/sql"
	"fmt"
)

type Whitelist struct {
	Id       int64
	Username string
}

func GetAllWhitelist(db *sql.DB) ([]Whitelist, error) {
	rows, err := db.Query("SELECT user_id, username FROM whitelist")
	if err != nil {
		return nil, fmt.Errorf("помилка запиту до БД: %w", err)
	}
	var whitelist []Whitelist

	for rows.Next() {
		var w Whitelist
		err := rows.Scan(&w.Id, &w.Username)
		if err != nil {
			return nil, err
		}
		whitelist = append(whitelist, w)
	}
	return whitelist, nil
}

func GetCachedFile(db *sql.DB, url string) (string, bool) {
	var filepath string
	err := db.QueryRow("SELECT filepath FROM cache WHERE url = ?", url).Scan(&filepath)
	if err != nil {
		return "", false
	}
	return filepath, true
}
