package database

import (
	"database/sql"
	"log"
)

func DeleteUser(db *sql.DB, username string) error {
	_, err := db.Exec("DELETE FROM whitelist WHERE username = ?", username)
	if err != nil {
		log.Printf("Не вдалося видалити користувача з вайтлисту: %v", err)
		return err
	}
	return nil
}

func DeleteCachedFile(db *sql.DB, url string) error {
	_, err := db.Exec("DELETE FROM cache WHERE url = ?", url)
	return err
}
