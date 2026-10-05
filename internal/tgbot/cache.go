package tgbot

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/Geergon/yt-dlp-goTelegramBot/internal/database"
	"github.com/celestix/gotgproto/types"
	"github.com/gotd/td/tg"
)

func sendFromCache(cacheDb *sql.DB, req URLRequest, chatID int64, sentMsg *types.Message, cached *database.CachedMedia) (ok bool, err error) {
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

	sentMsgId := sentMsg.GetID()

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
	// Fallback на файловий кеш...
	if _, err := os.Stat(cached.FilePath); err == nil {
		log.Printf("Знайдено в кеші: %s", cached.FilePath)
		images, media, thumbName, musicPath, err := mediaCheck(req.Context, chatID, sentMsgId, req.URL, req.Platform, false, cached.FilePath, req.Spoiler, "")
		if err == nil {
			_, err := sendMedia(req.Context, req.Update, req.URL, false, false, images, musicPath, media, chatID, sentMsgId)
			deleteMedia(req.Context, req.Update, req.URL, chatID, "", thumbName, false)
			return true, err
		}
	} else {
		log.Printf("Кешований файл не знайдено на диску, видаляємо запис: %s", cached.FilePath)
		err = database.DeleteCachedFile(cacheDb, req.URL)
		if err != nil {
			return false, fmt.Errorf("помилка при видаленні файлового кешу")
		}
	}
	return false, nil
}
