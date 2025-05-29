package tgbot

import (
	"log"
	"os"

	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
)

func SendLogs(ctx *ext.Context, update *ext.Update) error {
	chatID := Access(ctx, update)
	if chatID == 0 {
		log.Println("Відмова у доступі")
		return nil
	}

	logFile := "bot.log"
	fileInfo, err := os.Stat(logFile)
	if err != nil {
		logMsg := "Помилка перевірки файлу логів"
		if os.IsNotExist(err) {
			logMsg = "Файл логів не існує: " + logFile
		}
		log.Println(logMsg)
		_, err := ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
			Message: "Помилка: файл логів недоступний.",
		})
		return err
	}

	if fileInfo.IsDir() {
		log.Printf("Файл %s є директорією", logFile)
		_, err := ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
			Message: "Помилка: файл логів є директорією.",
		})
		return err
	}

	if fileInfo.Size() == 0 {
		log.Println("Файл логів порожній")
		_, err := ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
			Message: "Файл логів порожній.",
		})
		return err
	}

	logFileData, err := uploader.NewUploader(ctx.Raw).FromPath(ctx, logFile)
	if err != nil {
		log.Printf("Помилка завантаження файлу логів: %v", err)
		_, err := ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
			Message: "Помилка: не вдалося завантажити файл логів.",
		})
		return err
	}

	media := &tg.InputMediaUploadedDocument{
		File:     logFileData,
		MimeType: "text/plain",
		Attributes: []tg.DocumentAttributeClass{
			&tg.DocumentAttributeFilename{
				FileName: "bot.log",
			},
		},
	}

	_, err = ctx.SendMedia(chatID, &tg.MessagesSendMediaRequest{
		Media: media,
	})
	if err != nil {
		log.Printf("Помилка надсилання файлу логів: %v", err)
		return err
	}

	return nil
}
