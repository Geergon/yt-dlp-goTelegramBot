package database

import "database/sql"

func InitDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	createTable := `
        CREATE TABLE IF NOT EXISTS whitelist (
						user_id INTEGER PRIMARY KEY, 
						username TEXT UNIQUE
				);
    `
	_, err = db.Exec(createTable)
	if err != nil {
		return nil, err
	}

	return db, nil
}

func InitCacheTable(db *sql.DB) error {
	_, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS cache (
            url       TEXT PRIMARY KEY,
            filepath  TEXT NOT NULL,
            cached_at DATETIME DEFAULT CURRENT_TIMESTAMP
        )
    `)
	return err
}
