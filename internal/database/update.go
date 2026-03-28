package database

import "database/sql"

func InsertIntoWhitelist(db *sql.DB, username string, id int64) error {
	_, err := db.Exec(`INSERT OR IGNORE INTO whitelist (user_id, username) VALUES (?, ?)`, id, username)
	if err != nil {
		return err
	}
	return nil
}

type CachedMedia struct {
	FilePath      string
	DocID         int64
	AccessHash    int64
	FileReference []byte
}

func SetCachedFile(db *sql.DB, url string, c CachedMedia) error {
	_, err := db.Exec(
		`INSERT OR REPLACE INTO cache (url, filepath, doc_id, access_hash, file_reference)
         VALUES (?, ?, ?, ?, ?)`,
		url, c.FilePath, c.DocID, c.AccessHash, c.FileReference,
	)
	return err
}
