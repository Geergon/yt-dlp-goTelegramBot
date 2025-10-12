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
