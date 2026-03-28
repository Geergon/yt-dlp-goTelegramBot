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

func GetCachedFile(db *sql.DB, url string) (*CachedMedia, bool) {
	c := &CachedMedia{}
	err := db.QueryRow(
		"SELECT filepath, doc_id, access_hash, file_reference FROM cache WHERE url = ?", url,
	).Scan(&c.FilePath, &c.DocID, &c.AccessHash, &c.FileReference)
	if err != nil {
		return nil, false
	}
	return c, true
}
