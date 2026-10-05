package tgbot

import (
	"crypto/md5"
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/Geergon/yt-dlp-goTelegramBot/internal/database"
	"github.com/gotd/td/tg"
)

func saveToCache(db *sql.DB, url string, c database.CachedMedia) {
	cacheDir := "/cache"

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		log.Printf("Помилка створення папки кешу: %v", err)
		return
	}

	ext := filepath.Ext(c.FilePath)
	hash := fmt.Sprintf("%x", md5.Sum([]byte(url)))
	cachedPath := filepath.Join(cacheDir, hash+ext)

	if err := copyFile(c.FilePath, cachedPath); err != nil {
		log.Printf("Помилка копіювання в кеш: %v", err)
		return
	}

	c.FilePath = cachedPath
	if err := database.SetCachedFile(db, url, c); err != nil {
		log.Printf("Помилка збереження в БД кешу: %v", err)
	}
	log.Printf("Збережено в кеш: %s -> %s", url, cachedPath)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		os.Remove(dst)
		return err
	}
	return nil
}

func sendFromCache(cacheDb *sql.DB, req URLRequest, chatID int64, sentMsgId int, cached *database.CachedMedia) (ok bool, err error) {
	user := req.Update.EffectiveUser()
	username := "@" + user.Username
	title := username + " (link)"
	entities := []tg.MessageEntityClass{
		&tg.MessageEntityTextURL{
			Offset: len(username) + 1,
			Length: 6,
			URL:    req.URL,
		},
	}

	if cached.DocID != 0 {
		log.Printf("Надсилання з кешу через document reference: %d", cached.DocID)
		_, err := req.Context.EditMessage(chatID, &tg.MessagesEditMessageRequest{
			ID:       sentMsgId,
			Message:  title,
			Entities: entities,
			Media: &tg.InputMediaDocument{
				Spoiler: req.Spoiler,
				ID: &tg.InputDocument{
					ID:            cached.DocID,
					AccessHash:    cached.AccessHash,
					FileReference: cached.FileReference,
				},
			},
		})
		if err == nil {
			deleteMedia(req.Context, req.Update, req.URL, chatID, "", "", false)
			return true, nil
		}
		log.Printf("Document reference протух, надсилаємо файл: %v", err)
	}

	if _, err := os.Stat(cached.FilePath); err != nil {
		log.Printf("Кешований файл не знайдено, видаляємо запис: %s", cached.FilePath)
		if delErr := database.DeleteCachedFile(cacheDb, req.URL); delErr != nil {
			return false, fmt.Errorf("видалення запису кешу: %w", delErr)
		}
		return false, nil
	}

	images, media, thumbName, musicPath, err := mediaCheck(req.Context, req.URL, req.Platform, false, cached.FilePath, req.Spoiler, "")
	if err != nil {
		return false, err
	}

	doc, err := sendMedia(req.Context, req.Update, req.URL, false, false, images, musicPath, media, chatID, sentMsgId)
	deleteMedia(req.Context, req.Update, req.URL, chatID, "", thumbName, err != nil)
	if err != nil {
		return true, err
	}

	if doc != nil { // оновлюємо протухлий reference
		cached.DocID, cached.AccessHash, cached.FileReference = doc.ID, doc.AccessHash, doc.FileReference
		if err := database.SetCachedFile(cacheDb, req.URL, *cached); err != nil {
			log.Printf("Не вдалося оновити кеш: %v", err)
		}
	}

	return true, nil
}
