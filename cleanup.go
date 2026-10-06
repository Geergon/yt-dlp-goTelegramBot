package main

import (
	"database/sql"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/Geergon/yt-dlp-goTelegramBot/internal/config"
	"github.com/Geergon/yt-dlp-goTelegramBot/internal/database"
)

func startCleanupRoutine() {
	const cleanupInterval = 30 * time.Minute
	const fileAgeThreshold = 10 * time.Minute

	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		if len(semaphore) == 0 && len(urlQueue) == 0 {
			log.Println("Воркери вільні, запускаємо очищення папок")
			if err := cleanOldFiles(fileAgeThreshold); err != nil {
				log.Printf("Помилка очищення папок: %v", err)
			}
		} else {
			log.Println("Воркери зайняті або є завдання в черзі, пропускаємо очищення")
		}
	}
}

func startCacheCleanup(db *sql.DB) {
	ticker := time.NewTicker(24 * time.Hour)
	go func() {
		cleanCache(db) // одразу при старті
		for range ticker.C {
			cleanCache(db)
		}
	}()
}

func cleanCache(db *sql.DB) {
	maxAgeDays := config.GetInt("cache_max_age_days")
	maxSizeGB := config.GetFloat64("cache_max_size_gb")

	cacheDir := "./cache"

	if maxAgeDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -maxAgeDays)
		rows, err := db.Query("SELECT url, filepath FROM cache WHERE cached_at < ?", cutoff)
		if err != nil {
			log.Printf("Помилка запиту старих записів кешу: %v", err)
		} else {
			defer rows.Close()
			for rows.Next() {
				var url, filepath string
				if err := rows.Scan(&url, &filepath); err != nil {
					continue
				}
				if err := os.Remove(filepath); err != nil && !os.IsNotExist(err) {
					log.Printf("Помилка видалення файлу кешу %s: %v", filepath, err)
					continue
				}
				database.DeleteCachedFile(db, url)
				log.Printf("Видалено старий кеш: %s", filepath)
			}
		}
	}

	if maxSizeGB > 0 {
		maxBytes := int64(maxSizeGB * 1024 * 1024 * 1024)
		for {
			size, err := dirSize(cacheDir)
			if err != nil || size <= maxBytes {
				break
			}
			var url, filepath string
			err = db.QueryRow("SELECT url, filepath FROM cache ORDER BY cached_at ASC LIMIT 1").Scan(&url, &filepath)
			if err != nil {
				break
			}
			if err := os.Remove(filepath); err != nil && !os.IsNotExist(err) {
				log.Printf("Помилка видалення файлу кешу %s: %v", filepath, err)
				break
			}
			database.DeleteCachedFile(db, url)
			log.Printf("Кеш переповнений, видалено: %s", filepath)
		}
	}
}

func dirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

func cleanOldFiles(threshold time.Duration) error {
	// Remove temp audio dir
	tempDir := os.TempDir()
	files, err := os.ReadDir(tempDir)
	if err != nil {
		return err
	}

	lifetime := 10 * time.Minute
	for _, prefix := range []string{"audio-download-", "media-download-", "thumbnail-download-", "gallery-dl-download-"} {
		for _, file := range files {
			if file.IsDir() && len(file.Name()) >= len(prefix) && file.Name()[:len(prefix)] == prefix {

				info, err := file.Info()
				if err != nil {
					continue
				}

				if time.Since(info.ModTime()) > lifetime {
					fullPath := filepath.Join(tempDir, file.Name())
					os.RemoveAll(fullPath)
				}
			}
		}
	}

	// remove unused files
	dirs := []string{"video", "photo", "audio"}
	now := time.Now()

	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil // Пропускаємо директорії
			}

			info, err := d.Info()
			if err != nil {
				log.Printf("Помилка отримання інформації про файл %s: %v", path, err)
				return nil // Пропускаємо файл
			}

			if now.Sub(info.ModTime()) > threshold {
				if err := os.Remove(path); err != nil {
					log.Printf("Помилка видалення файлу %s: %v", path, err)
					return nil // Пропускаємо помилку, щоб продовжити
				}
				log.Printf("Видалено старий файл: %s", path)
			}
			return nil
		})
		if err != nil {
			log.Printf("Помилка обробки директорії %s: %v", dir, err)
		}
	}
	return nil
}
