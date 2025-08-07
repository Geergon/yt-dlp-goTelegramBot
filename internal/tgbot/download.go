package tgbot

import (
	"context"
	"fmt"
	"image/jpeg"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/Geergon/yt-dlp-goTelegramBot/internal/yt"
	"github.com/celestix/gotgproto/ext"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
	"github.com/spf13/viper"
)

type URLRequest struct {
	URL      string
	Platform string
	Command  string
	Fragment string
	Context  *ext.Context
	Update   *ext.Update
}

var bot *tgbotapi.BotAPI

func init() {
	err := os.MkdirAll("video", 0755)
	if err != nil {
		log.Fatalf("Помилка створення папки video: %v", err)
	}
	err = os.MkdirAll("photo", 0755)
	if err != nil {
		log.Fatalf("Помилка створення папки video: %v", err)
	}
	err = os.MkdirAll("audio", 0755)
	if err != nil {
		log.Fatalf("Помилка створення папки video: %v", err)
	}

	botToken := os.Getenv("BOT_TOKEN")
	if botToken == "" {
		log.Fatal("BOT_TOKEN не задано")
	}

	bot, err = tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Fatalf("Помилка ініціалізації бота: %v", err)
	}
	bot.Debug = false
}

func ProcessURL(req URLRequest) error {
	// Створюємо контекст із таймаутом у 10 хвилин
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	log.Printf("URLRequest: %s, %s", req.URL, req.Command)
	// Виконуємо обробку в окремій горутині, щоб перевіряти таймаут
	errChan := make(chan error, 1)
	go func() {
		log.Printf("Починаємо обробку URL %s (команда: %s)", req.URL, req.Command)
		errChan <- processURLWithContext(req)
	}()

	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		// Якщо минув таймаут, надсилаємо повідомлення про помилку
		chatID := Access(req.Context, req.Update)
		if chatID != 0 {
			_, err := req.Context.SendMessage(chatID, &tg.MessagesSendMessageRequest{
				Message: fmt.Sprintf("Обробка URL %s (команда: %s) перервана через таймаут (10 хвилин)", req.URL, req.Command),
			})
			if err != nil {
				log.Printf("Помилка надсилання повідомлення про таймаут: %v", err)
			}
		}
		log.Printf("Таймаут обробки URL %s (команда: %s) після 10 хвилин", req.URL, req.Command)
		return ctx.Err()
	}
}

func processURLWithContext(req URLRequest) error {
	chatID := Access(req.Context, req.Update)
	if chatID == 0 {
		log.Println("Відмова у доступі")
		return nil
	}

	switch req.Command {
	case "auto":
		return processAutoDownload(req, chatID)
	case "download":
		return processDownload(req, chatID)
	case "audio":
		return processAudio(req, chatID)
	case "fragment":
		return processFragment(req, chatID)
	default:
		log.Println("Невідома команда")
		return fmt.Errorf("невідома команда: %s", req.Command)
	}
}

func processAutoDownload(req URLRequest, chatID int64) error {
	viperMutex.RLock()
	autoDownload := viper.GetBool("auto_download")
	longVideoDownload := viper.GetBool("long_video_download")
	duration := viper.GetString("duration")
	viperMutex.RUnlock()

	if !autoDownload {
		return nil
	}
	msg := req.Update.EffectiveMessage
	text := msg.Text
	if strings.Contains(text, "/download") || strings.Contains(text, "/audio") || strings.Contains(text, "/fragment") {
		return nil
	}

	durationInt, err := strconv.ParseInt(duration, 10, 64)
	if err != nil {
		log.Printf("Помилка парсингу duration: %v", err)
		return err
	}

	info, err := yt.GetVideoInfo(req.URL, req.Platform)
	if err == nil && info.Duration >= int(durationInt) && !longVideoDownload {
		log.Printf("Відео занадто довге: %d секунд", info.Duration)
		return nil
	}
	if err == nil && info.IsLive || err == nil && info.WasLive {
		log.Println("Відео це стрім")
		return nil
	}

	sentMsg, err := req.Context.SendMessage(chatID, &tg.MessagesSendMessageRequest{
		Message: "Завантаження медіа: \n[◼◼◼◼◻◻◻◻]",
	})
	if err != nil {
		log.Printf("Помилка надсилання початкового повідомлення: %v", err)
		return err
	}
	sentMsgId := sentMsg.GetID()

	isPhoto, mediaFileName, downloadErr := downloadMedia(req.Context, chatID, req.URL, req.Platform, sentMsgId, longVideoDownload)
	if downloadErr != nil {
		log.Printf("Помилка при завантаженні: %v", downloadErr)
		_, editErr := req.Context.EditMessage(chatID, &tg.MessagesEditMessageRequest{
			ID:      sentMsgId,
			Message: fmt.Sprintf("Помилка завантаження: %v", downloadErr),
		})
		if editErr != nil {
			log.Printf("Помилка редагування повідомлення: %v", editErr)
		}

		// err := req.Context.DeleteMessages(chatID, []int{msg.ID})
		// if err != nil {
		// 	log.Printf("Помилка видалення повідомлення (ID: %d, ChatID: %d): %v", msg.ID, chatID, err)
		// } else {
		// 	log.Printf("Повідомлення (ID: %d, ChatID: %d) з URL %s видалено", msg.ID, chatID, req.URL)
		// }

		deleteMedia(req.Context, req.Update, req.URL, chatID, isPhoto, "", "")
		deleteMsgTimer(req.Context, chatID, sentMsgId)
		return downloadErr
	}

	_, err = req.Context.EditMessage(chatID, &tg.MessagesEditMessageRequest{
		ID:      sentMsgId,
		Message: "Перевірка і формування медіа перед відправкою: \n[◼◼◼◼◼◼◻◻]",
	})
	if err != nil {
		log.Printf("Помилка редагування повідомлення: %v", err)
		return err
	}

	images, media, thumbName, errCheck := mediaCheck(req.Context, chatID, sentMsgId, req.URL, req.Platform, isPhoto, mediaFileName)
	if errCheck != nil {
		log.Printf("Помилка при обробці медіа: %v", errCheck)
		_, editErr := req.Context.EditMessage(chatID, &tg.MessagesEditMessageRequest{
			ID:      sentMsgId,
			Message: fmt.Sprintf("Помилка обробки медіа: %v", errCheck),
		})
		if editErr != nil {
			log.Printf("Помилка редагування повідомлення: %v", editErr)
		}

		// err := req.Context.DeleteMessages(chatID, []int{msg.ID})
		// if err != nil {
		// 	log.Printf("Помилка видалення повідомлення (ID: %d, ChatID: %d): %v", msg.ID, chatID, err)
		// } else {
		// 	log.Printf("Повідомлення (ID: %d, ChatID: %d) з URL %s видалено", msg.ID, chatID, req.URL)
		// }

		deleteMedia(req.Context, req.Update, req.URL, chatID, isPhoto, mediaFileName, thumbName)
		deleteMsgTimer(req.Context, chatID, sentMsgId)
		return errCheck
	}

	_, err = req.Context.EditMessage(chatID, &tg.MessagesEditMessageRequest{
		ID:      sentMsgId,
		Message: "Надсилання: \n[◼◼◼◼◼◼◼◻]",
	})
	if err != nil {
		log.Printf("Помилка редагування повідомлення: %v", err)
		return err
	}

	err = sendMedia(req.Context, req.Update, req.URL, isPhoto, false, images, media, chatID, sentMsgId)
	if err != nil {
		log.Printf("Помилка при надсиланні повідомлення: %v", err)
		deleteMedia(req.Context, req.Update, req.URL, chatID, isPhoto, mediaFileName, thumbName)
		_, editErr := req.Context.EditMessage(chatID, &tg.MessagesEditMessageRequest{
			ID:      sentMsgId,
			Message: fmt.Sprintf("Помилка надсилання: %v", err),
		})
		if editErr != nil {
			log.Printf("Помилка редагування повідомлення: %v", editErr)
		}
		deleteMsgTimer(req.Context, chatID, sentMsgId)
		return err
	}

	deleteMedia(req.Context, req.Update, req.URL, chatID, isPhoto, mediaFileName, thumbName)
	return nil
}

// func processAutoDownload(req URLRequest, chatID int64, sentMsgId int) error {
// 	viperMutex.RLock()
// 	autoDownload := viper.GetBool("auto_download")
// 	longVideoDownload := viper.GetBool("long_video_download")
// 	duration := viper.GetString("duration")
// 	viperMutex.RUnlock()

// 	if autoDownload {

// 		chatID := Access(req.Context, req.Update)
// 		if chatID == 0 {
// 			log.Println("Відмова у доступі")
// 			return nil
// 		}

// 		msg := req.Update.EffectiveMessage
// 		text := msg.Text
// 		if strings.Contains(text, "/download") || strings.Contains(text, "/audio") {
// 			return nil
// 		}

// 		durationInt, err := strconv.ParseInt(duration, 10, 64)
// 		if err != nil {
// 			log.Printf("Помилка парсингу duration: %v", err)
// 			return err
// 		}

// 		info, err := yt.GetVideoInfo(req.URL, req.Platform)
// 		if err == nil && info.Duration >= int(durationInt) && !longVideoDownload {
// 			log.Printf("Відео занадто довге: %d секунд", info.Duration)
// 			return nil
// 		}

// 		sentMsg, err := req.Context.SendMessage(chatID, &tg.MessagesSendMessageRequest{
// 			Message: "Завантаження медіа: \n[◼◼◼◼◻◻◻◻]",
// 		})
// 		if err != nil {
// 			log.Printf("Помилка надсилання повідомлення про початок: %v", err)
// 			return err
// 		}
// 		sentMsgId := sentMsg.GetID()

// 		isPhoto, mediaFileName, downloadErr := downloadMedia(req.Context, chatID, req.URL, req.Platform, sentMsgId, longVideoDownload)
// 		if downloadErr != nil {
// 			log.Println("Помилка при завантаженні відео")
// 			return downloadErr
// 		}

// 		req.Context.EditMessage(chatID, &tg.MessagesEditMessageRequest{
// 			ID:      sentMsg.GetID(),
// 			Message: "Перевірка і формування медіа перед відправкою: \n[◼◼◼◼◼◼◻◻]",
// 		})
// 		images, media, thumbName, errCheck := mediaCheck(req.Context, chatID, sentMsgId, req.URL, req.Platform, isPhoto, mediaFileName)
// 		if errCheck != nil {
// 			log.Printf("Помилка при обробці медіа: %v", errCheck)
// 			return errCheck
// 		}

// 		req.Context.EditMessage(chatID, &tg.MessagesEditMessageRequest{
// 			ID:      sentMsg.GetID(),
// 			Message: "Надсилання: \n[◼◼◼◼◼◼◼◻]",
// 		})
// 		err = sendMedia(req.Context, req.Update, req.URL, isPhoto, false, images, media, chatID, sentMsgId)
// 		if err != nil {
// 			log.Printf("Помилка при надсиланні повідомлення: %v", err)
// 			deleteMedia(req.Context, req.Update, req.URL, chatID, isPhoto, mediaFileName, thumbName)
// 			return err
// 		}

// 		deleteMedia(req.Context, req.Update, req.URL, chatID, isPhoto, mediaFileName, thumbName)
// 		return nil
// 	}
// 	return fmt.Errorf("Автозавантаення вимкнено")
// }

// func Echo(ctx *ext.Context, update *ext.Update) error {
// 	viperMutex.RLock()
// 	autoDownload := viper.GetBool("auto_download")
// 	longVideoDownload := viper.GetBool("long_video_download")
// 	duration := viper.GetString("duration")
// 	viperMutex.RUnlock()
// 	if autoDownload {
// 		chatID := Access(ctx, update)
// 		if chatID == 0 {
// 			log.Println("Відмова у доступі")
// 			return nil
// 		}

// 		msg := update.EffectiveMessage
// 		text := msg.Text
// 		if strings.Contains(text, "/download") || strings.Contains(text, "/audio") {
// 			return nil
// 		}

// 		url, isValid, platform := Url(update)
// 		if !isValid {
// 			return nil
// 		}
// 		duration, err := strconv.ParseInt(duration, 10, 64)

// 		info, err := yt.GetVideoInfo(url, platform)
// 		if err == nil {
// 			if info.Duration >= int(duration) {
// 				return nil
// 			}
// 		}

// 		sentMsg, err := ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
// 			Message: "Завантаження медіа: \n[◼◼◼◼◻◻◻◻]",
// 		})
// 		if err != nil {
// 			log.Printf("Помилка надсилання повідомлення про початок: %v", err)
// 			return err
// 		}
// 		sentMsgId := sentMsg.GetID()

// 		isPhoto, mediaFileName, downloadErr := downloadMedia(ctx, chatID, url, platform, sentMsgId, longVideoDownload)
// 		if downloadErr != nil {
// 			log.Println("Помилка при завантаженні відео")
// 			return downloadErr
// 		}

// 		ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
// 			ID:      sentMsg.GetID(),
// 			Message: "Перевірка і формування медіа перед відправкою: \n[◼◼◼◼◼◼◻◻]",
// 		})
// 		images, media, thumbName, errCheck := mediaCheck(ctx, chatID, sentMsgId, url, platform, isPhoto, mediaFileName)
// 		if errCheck != nil {
// 			log.Printf("Помилка при обробці медіа: %v", errCheck)
// 			return errCheck
// 		}

// 		ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
// 			ID:      sentMsg.GetID(),
// 			Message: "Надсилання: \n[◼◼◼◼◼◼◼◻]",
// 		})
// 		err = sendMedia(ctx, update, url, isPhoto, false, images, media, chatID, sentMsgId)
// 		if err != nil {
// 			log.Printf("Помилка при надсиланні повідомлення: %v", err)
// 			return err
// 		}

// 		deleteMedia(ctx, update, url, chatID, isPhoto, mediaFileName, thumbName)
// 		return nil

// 	}
// 	return nil
// }

func processDownload(req URLRequest, chatID int64) error {
	sentMsg, err := req.Context.SendMessage(chatID, &tg.MessagesSendMessageRequest{
		Message: "Завантаження медіа: \n[◼◼◼◼◻◻◻◻]",
	})
	if err != nil {
		log.Printf("Помилка надсилання початкового повідомлення: %v", err)
		return err
	}
	sentMsgId := sentMsg.GetID()

	isPhoto, mediaFileName, downloadErr := downloadMedia(req.Context, chatID, req.URL, req.Platform, sentMsgId, true)
	if downloadErr != nil {
		log.Printf("Помилка при завантаженні відео: %v", downloadErr)
		_, editErr := req.Context.EditMessage(chatID, &tg.MessagesEditMessageRequest{
			ID:      sentMsgId,
			Message: fmt.Sprintf("Помилка завантаження: %v", downloadErr),
		})
		if editErr != nil {
			log.Printf("Помилка редагування повідомлення: %v", editErr)
		}
		deleteMsgTimer(req.Context, chatID, sentMsgId)
		return downloadErr
	}

	_, err = req.Context.EditMessage(chatID, &tg.MessagesEditMessageRequest{
		ID:      sentMsgId,
		Message: "Перевірка і формування медіа перед відправкою: \n[◼◼◼◼◼◼◻◻]",
	})
	if err != nil {
		log.Printf("Помилка редагування повідомлення: %v", err)
		return err
	}

	images, media, thumbName, errCheck := mediaCheck(req.Context, chatID, sentMsgId, req.URL, req.Platform, isPhoto, mediaFileName)
	if errCheck != nil {
		log.Printf("Помилка при обробці медіа: %v", errCheck)
		_, editErr := req.Context.EditMessage(chatID, &tg.MessagesEditMessageRequest{
			ID:      sentMsgId,
			Message: fmt.Sprintf("Помилка обробки медіа: %v", errCheck),
		})
		if editErr != nil {
			log.Printf("Помилка редагування повідомлення: %v", editErr)
		}
		deleteMsgTimer(req.Context, chatID, sentMsgId)
		return errCheck
	}

	_, err = req.Context.EditMessage(chatID, &tg.MessagesEditMessageRequest{
		ID:      sentMsgId,
		Message: "Надсилання: \n[◼◼◼◼◼◼◼◻]",
	})
	if err != nil {
		log.Printf("Помилка редагування повідомлення: %v", err)
		return err
	}

	err = sendMedia(req.Context, req.Update, req.URL, isPhoto, false, images, media, chatID, sentMsgId)
	if err != nil {
		log.Printf("Помилка при надсиланні повідомлення: %v", err)
		deleteMedia(req.Context, req.Update, req.URL, chatID, isPhoto, mediaFileName, thumbName)
		_, editErr := req.Context.EditMessage(chatID, &tg.MessagesEditMessageRequest{
			ID:      sentMsgId,
			Message: fmt.Sprintf("Помилка надсилання: %v", err),
		})
		if editErr != nil {
			log.Printf("Помилка редагування повідомлення: %v", editErr)
		}
		deleteMsgTimer(req.Context, chatID, sentMsgId)
		return err
	}

	deleteMedia(req.Context, req.Update, req.URL, chatID, isPhoto, mediaFileName, thumbName)
	return nil
}

func processFragment(req URLRequest, chatID int64) error {
	sentMsg, err := req.Context.SendMessage(chatID, &tg.MessagesSendMessageRequest{
		Message: "Завантаження відео і вирізання фрагменту: \n[◼◼◼◼◻◻◻◻]",
	})
	if err != nil {
		log.Printf("Помилка надсилання початкового повідомлення: %v", err)
		return err
	}
	sentMsgId := sentMsg.GetID()

	if req.Fragment == "" {
		_, err := req.Context.EditMessage(chatID, &tg.MessagesEditMessageRequest{
			ID:      sentMsgId,
			Message: "Помилка: не вказано фрагмент. Використання: /fragment <YouTube_URL> <00:00-00:00>",
		})
		deleteMsgTimer(req.Context, chatID, sentMsgId)
		return err
	}

	viperMutex.RLock()
	filter := viper.GetString("yt-dlp_filter")
	viperMutex.RUnlock()

	timeUnix := time.Now().UnixMilli()
	outputFile := fmt.Sprintf("./video/outputFrag%d.mp4", timeUnix)
	cmd := exec.Command(
		"yt-dlp",
		"--download-sections", fmt.Sprintf("*%s", req.Fragment),
		"-f", filter,
		"-o", outputFile,
		req.URL,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("yt-dlp error: %v\nOutput: %s", err, string(output))
		_, editErr := req.Context.EditMessage(chatID, &tg.MessagesEditMessageRequest{
			ID:      sentMsgId,
			Message: fmt.Sprintf("Помилка завантаження фрагменту: %v", err),
		})
		if editErr != nil {
			log.Printf("Помилка редагування повідомлення: %v", editErr)
		}
		deleteMsgTimer(req.Context, chatID, sentMsgId)
		return err
	}

	log.Printf("Завантаження фрагменту %s завершено успішно", req.URL)

	_, err = req.Context.EditMessage(chatID, &tg.MessagesEditMessageRequest{
		ID:      sentMsgId,
		Message: "Перевірка і формування медіа перед відправкою: \n[◼◼◼◼◼◼◻◻]",
	})
	if err != nil {
		log.Printf("Помилка редагування повідомлення: %v", err)
		return err
	}

	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		_, editErr := req.Context.EditMessage(chatID, &tg.MessagesEditMessageRequest{
			ID:      sentMsgId,
			Message: "Не вдалося завантажити фрагмент",
		})
		if editErr != nil {
			log.Printf("Помилка редагування повідомлення: %v", editErr)
		}
		deleteMsgTimer(req.Context, chatID, sentMsgId)
		return err
	}

	videoFile, err := uploader.NewUploader(req.Context.Raw).FromPath(req.Context, outputFile)
	if err != nil {
		log.Printf("Помилка завантаження відео в Telegram: %v", err)
		_, editErr := req.Context.EditMessage(chatID, &tg.MessagesEditMessageRequest{
			ID:      sentMsgId,
			Message: fmt.Sprintf("Помилка завантаження фрагменту в Telegram: %v", err),
		})
		if editErr != nil {
			log.Printf("Помилка редагування повідомлення: %v", editErr)
		}
		deleteMsgTimer(req.Context, chatID, sentMsgId)
		return err
	}

	var media tg.InputMediaClass
	var thumbName string
	media = &tg.InputMediaUploadedDocument{
		File:     videoFile,
		MimeType: "video/mp4",
		Attributes: []tg.DocumentAttributeClass{
			&tg.DocumentAttributeVideo{
				SupportsStreaming: true,
			},
			&tg.DocumentAttributeFilename{
				FileName: path.Base(outputFile),
			},
		},
	}

	if thumbName = yt.GetThumb(req.URL, "YouTube"); thumbName != "" {
		if thumbFileStat, err := os.Stat(thumbName); err == nil && !thumbFileStat.IsDir() {
			if thumbFile, err := uploader.NewUploader(req.Context.Raw).FromPath(req.Context, thumbName); err == nil {
				media.(*tg.InputMediaUploadedDocument).Thumb = thumbFile
			} else {
				log.Printf("Помилка завантаження прев’ю %s: %v", thumbName, err)
			}
		} else {
			log.Printf("Прев’ю недоступне або є помилкою: %s", thumbName)
		}
	}

	_, err = req.Context.EditMessage(chatID, &tg.MessagesEditMessageRequest{
		ID:      sentMsgId,
		Message: "Надсилання: \n[◼◼◼◼◼◼◼◻]",
	})
	if err != nil {
		log.Printf("Помилка редагування повідомлення: %v", err)
		return err
	}

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

	_, err = req.Context.EditMessage(chatID, &tg.MessagesEditMessageRequest{
		ID:       sentMsgId,
		Message:  title,
		Media:    media,
		Entities: entities,
	})
	if err != nil {
		log.Printf("Помилка відправлення фрагменту: %v", err)
		deleteMedia(req.Context, req.Update, req.URL, chatID, false, outputFile, thumbName)
		_, editErr := req.Context.EditMessage(chatID, &tg.MessagesEditMessageRequest{
			ID:      sentMsgId,
			Message: fmt.Sprintf("Помилка надсилання фрагменту: %v", err),
		})
		if editErr != nil {
			log.Printf("Помилка редагування повідомлення: %v", editErr)
		}
		deleteMsgTimer(req.Context, chatID, sentMsgId)
		return err
	}

	if err := os.Remove(outputFile); err != nil {
		log.Printf("Помилка видалення файлу %s: %v", outputFile, err)
	}

	if thumbName != "" {
		if err := os.Remove(thumbName); err != nil {
			log.Printf("Не вдалося видалити прев’ю: %v", err)
		}
	}

	return nil
}

func processAudio(req URLRequest, chatID int64) error {
	sentMsg, err := req.Context.SendMessage(chatID, &tg.MessagesSendMessageRequest{
		Message: "Завантаження аудіо: \n[◼◼◼◼◻◻◻◻]",
	})
	if err != nil {
		log.Printf("Помилка надсилання початкового повідомлення: %v", err)
		return err
	}
	sentMsgId := sentMsg.GetID()

	const maxAttempts = 3
	const retryDelay = 5 * time.Second

	var audioName string
	var downloadErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		audio, err := yt.DownloadAudio(req.URL, req.Platform)
		if err != nil {
			log.Printf("Спроба %d завантаження аудіо (%s) не вдалося: %v", attempt, req.Platform, err)
			downloadErr = err
			if attempt < maxAttempts {
				log.Printf("Чекаємо %v перед наступною спробою...", retryDelay)
				time.Sleep(retryDelay)
			}
			continue
		}

		if len(audio) == 0 {
			log.Printf("Не знайдено аудіофайлів після завантаження для URL: %s (спроба %d)", req.URL, attempt)
			downloadErr = fmt.Errorf("не знайдено аудіофайлів після завантаження")
			if attempt < maxAttempts {
				log.Printf("Чекаємо %v перед наступною спробою...", retryDelay)
				time.Sleep(retryDelay)
			}
			continue
		}

		audioName = audio[0]
		log.Printf("Аудіо успішно завантажено на спробі %d: %s", attempt, audioName)
		downloadErr = nil
		break
	}

	if downloadErr != nil || audioName == "" {
		log.Printf("Не вдалося завантажити аудіо після %d спроб для URL: %s, остання помилка: %v", maxAttempts, req.URL, downloadErr)
		errMsg := fmt.Sprintf("Не вдалося завантажити аудіо після %d спроб: %v", maxAttempts, downloadErr)
		_, editErr := req.Context.EditMessage(chatID, &tg.MessagesEditMessageRequest{
			ID:      sentMsgId,
			Message: errMsg,
		})
		if editErr != nil {
			log.Printf("Помилка редагування повідомлення: %v", editErr)
		}
		deleteMsgTimer(req.Context, chatID, sentMsgId)
		return fmt.Errorf("download error: %w", downloadErr)
	}

	_, err = req.Context.EditMessage(chatID, &tg.MessagesEditMessageRequest{
		ID:      sentMsgId,
		Message: "Перевірка і формування аудіо перед відправкою: \n[◼◼◼◼◼◼◻◻]",
	})
	if err != nil {
		log.Printf("Помилка редагування повідомлення: %v", err)
		return err
	}

	fileData, err := uploader.NewUploader(req.Context.Raw).FromPath(req.Context, audioName)
	if err != nil {
		log.Printf("Помилка завантаження аудіо в Telegram: %v", err)
		logErr := fmt.Sprintf("Помилка завантаження аудіо в Telegram: %v", err)
		_, editErr := req.Context.EditMessage(chatID, &tg.MessagesEditMessageRequest{
			ID:      sentMsgId,
			Message: logErr,
		})
		if editErr != nil {
			log.Printf("Помилка редагування повідомлення: %v", editErr)
		}
		deleteMsgTimer(req.Context, chatID, sentMsgId)
		return err
	}

	media := &tg.InputMediaUploadedDocument{
		File:     fileData,
		MimeType: "audio/mpeg",
		Attributes: []tg.DocumentAttributeClass{
			&tg.DocumentAttributeAudio{
				Title: path.Base(audioName),
			},
			&tg.DocumentAttributeFilename{
				FileName: audioName,
			},
		},
	}

	thumbName := yt.GetThumb(req.URL, req.Platform)
	if thumbName != "" {
		if thumbFileStat, err := os.Stat(thumbName); err == nil && !thumbFileStat.IsDir() {
			if thumbFile, err := uploader.NewUploader(req.Context.Raw).FromPath(req.Context, thumbName); err == nil {
				media.Thumb = thumbFile
			} else {
				log.Printf("Помилка завантаження прев’ю %s: %v", thumbName, err)
			}
		} else {
			log.Printf("Прев’ю недоступне або є помилкою: %s", thumbName)
		}
	}

	_, err = req.Context.EditMessage(chatID, &tg.MessagesEditMessageRequest{
		ID:      sentMsgId,
		Message: "Надсилання: \n[◼◼◼◼◼◼◼◻]",
	})
	if err != nil {
		log.Printf("Помилка редагування повідомлення: %v", err)
		return err
	}

	err = sendMedia(req.Context, req.Update, req.URL, false, true, nil, media, chatID, sentMsgId)
	if err != nil {
		log.Printf("Помилка при надсиланні аудіо: %v", err)
		deleteMedia(req.Context, req.Update, req.URL, chatID, false, audioName, thumbName)
		_, editErr := req.Context.EditMessage(chatID, &tg.MessagesEditMessageRequest{
			ID:      sentMsgId,
			Message: fmt.Sprintf("Помилка надсилання аудіо: %v", err),
		})
		if editErr != nil {
			log.Printf("Помилка редагування повідомлення: %v", editErr)
		}
		deleteMsgTimer(req.Context, chatID, sentMsgId)
		return err
	}

	deleteMedia(req.Context, req.Update, req.URL, chatID, false, audioName, thumbName)
	return nil
}

func Url(update *ext.Update) (string, bool, string) {
	msg := update.EffectiveMessage
	text := msg.Text

	if strings.Contains(text, "/fragment") {
		return "", false, ""
	}
	var url, platform string
	var isValid bool

	if urlYT, isYT := yt.GetYoutubeURL(text); isYT {
		url, isValid, platform = urlYT, true, "YouTube"
	} else if urlTT, isTT := yt.GetTikTokURL(text); isTT {
		url, isValid, platform = urlTT, true, "TikTok"
	} else if urlInsta, isInsta := yt.GetInstaURL(text); isInsta {
		url, isValid, platform = urlInsta, true, "Instagram"
	}

	if !isValid || len(url) == 0 || !yt.IsUrl(url) {
		return "", false, ""
	}
	return url, isValid, platform
}

func downloadMedia(ctx *ext.Context, chatID int64, url string, platform string, sentMsgId int, longVideoDownload bool) (bool, string, error) {
	var downloadFunc func(string, string) (bool, error)
	timeUnix := time.Now().UnixMilli()
	mediaFileName := fmt.Sprintf("./video/output%d.mp4", timeUnix)
	switch platform {
	case "YouTube":
		downloadFunc = func(url string, output string) (bool, error) {
			_, err := yt.DownloadYTVideo(url, output, longVideoDownload)
			return false, err // Always video
		}
	case "TikTok":
		downloadFunc = func(url string, output string) (bool, error) {
			isPhotos, err := yt.DownloadTTVideo(url, output)
			return isPhotos, err
		}
	case "Instagram":
		downloadFunc = func(url string, output string) (bool, error) {
			isPhotos, err := yt.DownloadInstaVideo(url, output)
			return isPhotos, err
		}
	}
	const maxAttempts = 3
	const retryDelay = 10 * time.Second

	var downloadErr error
	var isPhoto bool
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		isPhoto, downloadErr = downloadFunc(url, mediaFileName)
		if downloadErr == nil || isPhoto {
			break
		}
		log.Printf("Спроба %d завантаження (%s) не вдалося: %v", attempt, platform, downloadErr)

		videoName := "output.mp4"
		partFile := videoName + ".part"
		if err := os.Remove(partFile); err != nil && !os.IsNotExist(err) {
			log.Printf("Не вдалося видалити частковий файл %s: %v", partFile, err)
		}

		if attempt < maxAttempts {
			time.Sleep(retryDelay)
		}
	}

	if downloadErr != nil && !isPhoto {
		log.Printf("Не вдалося завантажити медіа після %d спроб (%s): %v", maxAttempts, platform, downloadErr)
		errMsg := fmt.Sprintf("Не вдалося завантажити медіа після %d спроб (%s): %v", maxAttempts, platform, downloadErr)
		_, editErr := ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
			ID:      sentMsgId,
			Message: errMsg,
		})
		if editErr != nil {
			log.Printf("Помилка редагування повідомлення: %v", editErr)
		}
		deleteMsgTimer(ctx, chatID, sentMsgId)
		return isPhoto, mediaFileName, downloadErr
	}
	return isPhoto, mediaFileName, nil
}

func mediaCheck(ctx *ext.Context, chatID int64, sentMsgId int, url string, platform string, isPhoto bool, mediaFileName string) ([]string, tg.InputMediaClass, string, error) {
	var thumbName string
	var media tg.InputMediaClass
	var isExist bool
	var images []string

	if !isPhoto {
		file, err := os.Stat(mediaFileName)
		if err != nil {
			logMsg := "Помилка перевірки файлу відео"
			if os.IsNotExist(err) {
				logMsg = "Файл не існує: " + mediaFileName
			}
			log.Println(logMsg)
			ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
				ID:      sentMsgId,
				Message: "Помилка: не вдалося завантажити відео: " + logMsg,
			})
			deleteMsgTimer(ctx, chatID, sentMsgId)
			return nil, nil, "", err
		}

		if file.IsDir() {
			log.Printf("Файл %s є директорією", mediaFileName)
			ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
				ID:      sentMsgId,
				Message: "Помилка: завантажений файл є директорією.",
			})
			deleteMsgTimer(ctx, chatID, sentMsgId)
			return nil, nil, "", fmt.Errorf("Файл %s є директорією", mediaFileName)
		}

		fileData, err := uploader.NewUploader(ctx.Raw).FromPath(ctx, mediaFileName)
		if err != nil {
			log.Printf("Помилка завантаження відео в Telegram: %v", err)
			logErr := fmt.Sprintf("Помилка завантаження відео в Telegram: \n%v", err)
			_, editErr := ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
				ID:      sentMsgId,
				Message: logErr,
			})
			if editErr != nil {
				log.Printf("Помилка редагування повідомлення: %v", editErr)
			}
			return nil, nil, "", err
		}

		media = &tg.InputMediaUploadedDocument{
			File:     fileData,
			MimeType: "video/mp4",
			Attributes: []tg.DocumentAttributeClass{
				&tg.DocumentAttributeVideo{
					SupportsStreaming: true,
				},
				&tg.DocumentAttributeFilename{
					FileName: mediaFileName,
				},
			},
		}

		if thumbName = yt.GetThumb(url, platform); thumbName != "" {
			if thumbFileStat, err := os.Stat(thumbName); err == nil && !thumbFileStat.IsDir() {
				if thumbFile, err := uploader.NewUploader(ctx.Raw).FromPath(ctx, thumbName); err == nil {
					media.(*tg.InputMediaUploadedDocument).Thumb = thumbFile
				} else {
					log.Printf("Помилка завантаження прев’ю %s: %v", thumbName, err)
				}
			} else {
				log.Printf("Прев’ю недоступне або є директорією: %s", thumbName)
			}
		}
	} else {
		images, isExist = yt.GetPhotoPathList()
		if !isExist {
			log.Println("Помилка при завантаженні фотографій. Їх не існує")
			return nil, nil, "", fmt.Errorf("Помилка при завантаженні фотографій")
		}

		for _, filePath := range images {
			fileInfo, err := os.Stat(filePath)
			if os.IsNotExist(err) {
				log.Printf("Файл %s не існує", filePath)
				continue
			}
			if fileInfo.Size() > 10*1024*1024 {
				log.Printf("Файл %s занадто великий: %d байтів", filePath, fileInfo.Size())
				continue
			}

			file, err := os.Open(filePath)
			if err != nil {
				log.Printf("Помилка відкриття файлу %s: %v", filePath, err)
				continue
			}
			defer file.Close()
			_, err = jpeg.Decode(file)
			if err != nil {
				log.Printf("Файл %s не є коректним JPEG: %v", filePath, err)
				continue
			}
		}
	}
	return images, media, thumbName, nil
}

func sendMedia(ctx *ext.Context, update *ext.Update, url string, isPhoto bool, isAudio bool, images []string, media tg.InputMediaClass, chatID int64, sentMsgId int) error {
	user := update.EffectiveUser()
	username := "@" + user.Username
	title := username + " (link)"
	entities := []tg.MessageEntityClass{
		&tg.MessageEntityTextURL{
			Offset: len(username) + 1,
			Length: 6,
			URL:    url,
		},
	}

	if isPhoto {
		err := ctx.DeleteMessages(chatID, []int{sentMsgId})
		if err != nil {
			log.Println("Помилка видалення повідомлення")
		} else {
			log.Printf("Повідомлення (ID: %d, ChatID: %d) з URL %s видалено", sentMsgId, chatID, url)
		}

		botChatID := chatID
		idAsString := strconv.FormatInt(botChatID, 10)
		prefixedIDString := "-100" + idAsString
		newTelegramID, err := strconv.ParseInt(prefixedIDString, 10, 64)

		log.Printf("Надсилання %d зображень до чату: ID %d, використовуючи telegram-bot-api", len(images), botChatID)
		var mediaGroup []interface{}
		for i, filePath := range images {
			media := tgbotapi.NewInputMediaPhoto(tgbotapi.FilePath(filePath))
			if i == 0 {
				media.Caption = fmt.Sprintf("%s (<a href=\"%s\">link</a>)", username, url)
				media.ParseMode = "HTML"
			}
			mediaGroup = append(mediaGroup, media)
		}
		msgConfig := tgbotapi.NewMediaGroup(newTelegramID, mediaGroup)
		_, err = bot.SendMediaGroup(msgConfig)
		if err != nil {
			log.Printf("Помилка відправлення медіа-групи через telegram-bot-api: %v", err)
			return err
		}
		log.Printf("Альбом з %d зображеннями усіпшно відправлено", len(images))
	} else if isAudio {

		ctx.DeleteMessages(chatID, []int{sentMsgId})
		_, err := ctx.SendMedia(chatID, &tg.MessagesSendMediaRequest{
			Media: media,
		})
		if err != nil {
			log.Printf("Помилка редагування повідомлення з аудіо: %v", err)
			return err
		}
	} else {
		_, err := ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
			ID:       sentMsgId,
			Message:  title,
			Media:    media,
			Entities: entities,
		})
		if err != nil {
			log.Printf("Помилка редагування повідомлення з відео: %v", err)
			return err
		}
	}
	return nil
}

func deleteMedia(ctx *ext.Context, update *ext.Update, url string, chatID int64, isPhoto bool, mediaFileName string, thumbName string) {
	msg := update.EffectiveMessage
	text := msg.Text

	viperMutex.RLock()
	deleteURL := viper.GetBool("delete_url")
	viperMutex.RUnlock()
	if deleteURL {
		if strings.TrimSpace(text) == url {
			log.Printf("Спроба видалити повідомлення (ID: %d, ChatID: %d) з URL: %s", msg.ID, chatID, url)

			err := ctx.DeleteMessages(chatID, []int{msg.ID})
			if err != nil {
				log.Printf("Помилка видалення повідомлення (ID: %d, ChatID: %d): %v", msg.ID, chatID, err)
			} else {
				log.Printf("Повідомлення (ID: %d, ChatID: %d) з URL %s видалено", msg.ID, chatID, url)
			}
		}
	}

	if isPhoto {
		dir := "./photo"
		photo := os.DirFS(dir)
		jpgFiles, err := fs.Glob(photo, "*.jpg")
		if err != nil {
			log.Printf("Помилка пошуку JPG-файлів у %s: %v", dir, err)
		}
		for _, m := range jpgFiles {
			filePath := path.Join(dir, m)
			if err := os.Remove(filePath); err != nil {
				log.Printf("Не вдалося видалити %s: %v", filePath, err)
			}
		}
	}
	if !isPhoto {
		if err := os.Remove(mediaFileName); err != nil {
			log.Printf("Не вдалося видалити медіа: %v", err)
		}
		if thumbName != "" {
			if err := os.Remove(thumbName); err != nil {
				log.Printf("Не вдалося видалити прев’ю: %v", err)
			}
		}
	}
}

func deleteMsgTimer(ctx *ext.Context, chatID int64, sentMsgId int) {
	const errorMessageTimeout = 60 * time.Second

	time.AfterFunc(errorMessageTimeout, func() {
		ctx.DeleteMessages(chatID, []int{sentMsgId})
	})
}
