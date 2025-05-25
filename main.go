package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	yt "github.com/Geergon/yt-dlp-goTelegramBot/internal/yt"
	"github.com/glebarez/sqlite"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
	"github.com/joho/godotenv"
	"github.com/spf13/viper"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/celestix/gotgproto"
	"github.com/celestix/gotgproto/dispatcher/handlers"
	"github.com/celestix/gotgproto/dispatcher/handlers/filters"
	"github.com/celestix/gotgproto/ext"
	"github.com/celestix/gotgproto/sessionMaker"
)

func init() {
	logFile, err := os.OpenFile("bot.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Помилка відкриття файлу логів: %v", err)
	}
	log.SetOutput(&lumberjack.Logger{
		Filename:   "bot.log",
		MaxSize:    10, // МБ
		MaxBackups: 3,
		MaxAge:     28, // дні
		Compress:   true,
	})
	log.SetOutput(logFile)
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	appId, err := strconv.Atoi(os.Getenv("APP_ID"))
	if err != nil {
		log.Fatal("Помилка при отриманні APP_ID")
	}
	apiHash := os.Getenv("API_HASH")
	if apiHash == "" {
		log.Fatal("API_HASH не задано")
	}

	botToken := os.Getenv("BOT_TOKEN")
	if botToken == "" {
		log.Fatal("BOT_TOKEN не задано")
	}

	chatId := os.Getenv("CHAT_ID")
	if chatId == "" {
		log.Fatal("CHAT_ID не задано")
	}

	viper.SetConfigName("config")
	viper.SetConfigType("toml")
	viper.AddConfigPath("./config")
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Println("Конфіг файл не знайдений")
		} else {
			log.Printf("Помилка з конфіг файлом: %v", err)
		}
	}

	client, err := gotgproto.NewClient(
		// Get AppID from https://my.telegram.org/apps
		appId,
		// Get ApiHash from https://my.telegram.org/apps
		os.Getenv("API_HASH"),
		// ClientType, as we defined above
		gotgproto.ClientTypeBot(os.Getenv("BOT_TOKEN")),
		// Optional parameters of client
		&gotgproto.ClientOpts{
			Session: sessionMaker.SqlSession(sqlite.Open("./db/session")),
		},
	)
	if err != nil {
		log.Fatalln("Помилка при запуску бота:", err)
	}

	dispatcher := client.Dispatcher

	dispatcher.AddHandlerToGroup(handlers.NewMessage(filters.Message.Text, echo), 1)
	dispatcher.AddHandler(handlers.NewCommand("logs", sendLogs))
	dispatcher.AddHandler(handlers.NewCommand("update", updateYtdlp))
	dispatcher.AddHandler(handlers.NewCommand("download", download))
	dispatcher.AddHandler(handlers.NewCommand("audio", audio))
	dispatcher.AddHandler(handlers.NewCommand("start", func(ctx *ext.Context, u *ext.Update) error {
		chatID := u.EffectiveChat().GetID()
		_, err := ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
			Message: "Ласкаво просимо! Надішліть URL з YouTube, TikTok або Instagram для завантаження відео.\nКоманди:\n/logs - отримати логи\n/update - оновити yt-dlp",
		})
		if err != nil {
			log.Printf("Помилка надсилання повідомлення: %v", err)
			return err
		}
		return nil
	}))
	dispatcher.AddHandler(handlers.NewCommand("settings", settings))
	dispatcher.AddHandler(handlers.NewCallbackQuery(filters.CallbackQuery.Prefix("cb_settings_"), settingsCallback))

	fmt.Printf("Бот (@%s) стартував...\n", client.Self.Username)

	client.Idle()
}

func echo(ctx *ext.Context, update *ext.Update) error {
	if viper.GetBool("auto_download") {
		download(ctx, update)
	}
	return nil
}

func sendLogs(ctx *ext.Context, update *ext.Update) error {
	allowedChatId, err := strconv.Atoi(os.Getenv("CHAT_ID"))
	if err != nil {
		log.Fatalf("Не вдалося отримати CHAT_ID: %v", err)
	}

	chatID := update.EffectiveChat().GetID()
	if chatID != int64(allowedChatId) {
		log.Printf("Неавторизований доступ до команди /logs від користувача %s", update.EffectiveUser().Username)
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
		Media:   media,
		Message: "Ось логи",
	})
	if err != nil {
		log.Printf("Помилка надсилання файлу логів: %v", err)
		return err
	}

	return nil
}

func updateYtdlp(ctx *ext.Context, update *ext.Update) error {
	allowedChatId, err := strconv.Atoi(os.Getenv("CHAT_ID"))
	if err != nil {
		log.Fatalf("Не вдалося отримати CHAT_ID: %v", err)
	}

	chatID := update.EffectiveChat().GetID()
	if chatID != int64(allowedChatId) {
		log.Printf("Неавторизований доступ до команди /logs від користувача %s", update.EffectiveUser().Username)
		return nil
	}
	msg := yt.UpdateYtdlp()
	ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
		Message: msg,
	})

	return nil
}

func settings(ctx *ext.Context, update *ext.Update) error {
	allowedChatId, err := strconv.Atoi(os.Getenv("CHAT_ID"))
	if err != nil {
		log.Fatalf("Не вдалося отримати CHAT_ID: %v", err)
	}

	chatID := update.EffectiveChat().GetID()
	if chatID != int64(allowedChatId) {
		return nil
	}

	rows := []tg.KeyboardButtonRow{
		{
			Buttons: []tg.KeyboardButtonClass{
				&tg.KeyboardButtonCallback{
					Text: "Автозавантаження відео: " + boolToEmoji(viper.GetBool("auto_download")),
					Data: []byte("cb_settings_auto_download"),
				},
			},
		},
		{
			Buttons: []tg.KeyboardButtonClass{
				&tg.KeyboardButtonCallback{
					Text: "Видалення посилань: " + boolToEmoji(viper.GetBool("delete_url")),
					Data: []byte("cb_settings_delete_links"),
				},
			},
		},
	}

	_, err = ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
		Message: "⚙️ Налаштування бота:\nВиберіть опцію для увімкнення/вимкнення.",
		ReplyMarkup: &tg.ReplyInlineMarkup{
			Rows: rows,
		},
	})
	if err != nil {
		log.Printf("Помилка надсилання повідомлення /settings: %v", err)
		return err
	}

	return nil
}

func boolToEmoji(b bool) string {
	if b {
		return "✅"
	}
	return "❌"
}

func settingsCallback(ctx *ext.Context, u *ext.Update) error {
	allowedChatId, err := strconv.Atoi(os.Getenv("CHAT_ID"))
	if err != nil {
		log.Fatalf("Не вдалося отримати CHAT_ID: %v", err)
	}

	chatID := u.EffectiveChat().GetID()
	if chatID != int64(allowedChatId) {
		return nil
	}

	callback := u.CallbackQuery
	data := callback.Data
	messageID := callback.MsgID

	autoDownload := viper.GetBool("auto_download")
	deleteUrl := viper.GetBool("delete_url")
	switch string(data) {
	case "cb_settings_auto_download":
		viper.Set("auto_download", !autoDownload)
	case "cb_settings_delete_links":
		viper.Set("delete_url", !deleteUrl)
	default:
		log.Printf("Невідомий callback: %s", data)
		return nil
	}
	viper.WriteConfig()

	// Оновлюємо кнопки
	rows := []tg.KeyboardButtonRow{
		{
			Buttons: []tg.KeyboardButtonClass{
				&tg.KeyboardButtonCallback{
					Text: "Автозавантаження відео: " + boolToEmoji(viper.GetBool("auto_download")),
					Data: []byte("cb_settings_auto_download"),
				},
			},
		},
		{
			Buttons: []tg.KeyboardButtonClass{
				&tg.KeyboardButtonCallback{
					Text: "Видалення посилань: " + boolToEmoji(viper.GetBool("delete_url")),
					Data: []byte("cb_settings_delete_links"),
				},
			},
		},
	}

	_, err = ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
		ID:      messageID,
		Message: "⚙️ Налаштування бота:",
		ReplyMarkup: &tg.ReplyInlineMarkup{
			Rows: rows,
		},
	})
	if err != nil {
		log.Printf("Помилка редагування повідомлення: %v", err)
		return err
	}

	_, err = ctx.AnswerCallback(&tg.MessagesSetBotCallbackAnswerRequest{
		QueryID: callback.QueryID,
	})
	if err != nil {
		log.Printf("Помилка відповіді на callback: %v", err)
		return err
	}

	return nil
}

func download(ctx *ext.Context, update *ext.Update) error {
	allowedChatId, err := strconv.Atoi(os.Getenv("CHAT_ID"))
	if err != nil {
		log.Fatalln("Не вдалося отримати chatID")
	}
	chat := update.EffectiveChat()
	chatID := update.EffectiveChat().GetID()
	user := update.EffectiveUser()

	if chat == nil || chatID != int64(allowedChatId) {
		// Неавторизований доступ
		fmt.Printf("Неавторизований доступ: %s \n", user.Username)
		return nil
	}

	msg := update.EffectiveMessage
	text := msg.Text
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
		return nil
	}

	sentMsg, err := ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
		Message: "Отримання інформації про відео: \n[◼◼◻◻◻◻◻◻]",
	})
	if err != nil {
		log.Printf("Помилка надсилання повідомлення про початок: %v", err)
		return err
	}

	info, err := yt.GetVideoInfo(url)
	if err != nil {
		log.Printf("Помилка отримання інформації про відео (%s): %v", platform, err)
		_, editErr := ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
			ID:      sentMsg.GetID(),
			Message: "Помилка: не вдалося отримати інформацію про відео.",
		})
		if editErr != nil {
			log.Printf("Помилка редагування повідомлення: %v", editErr)
		}
		return err
	}

	ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
		ID:      sentMsg.GetID(),
		Message: "Завантаження відео: \n[◼◼◼◼◻◻◻◻]",
	})
	var downloadFunc func(context.Context, string, *yt.VideoInfo) error
	switch platform {
	case "YouTube":
		downloadFunc = yt.DownloadYTVideo
	case "TikTok":
		downloadFunc = yt.DownloadTTVideo
	case "Instagram":
		downloadFunc = yt.DownloadInstaVideo
	}

	const maxAttempts = 3
	const retryDelay = 10 * time.Second
	const timeout = 15 * time.Minute

	var downloadErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		downloadCtx, cancel := context.WithTimeout(context.Background(), timeout)
		downloadCtx = context.WithValue(downloadCtx, "attempt", attempt)
		defer cancel()

		downloadErr = downloadFunc(downloadCtx, url, info)
		if downloadErr == nil {
			break
		}
		log.Printf("Спроба %d завантаження (%s) не вдалася: %v", attempt, platform, downloadErr)

		videoName := yt.GetVideoName(url, info)
		partFile := videoName + ".part"
		if err := os.Remove(partFile); err != nil && !os.IsNotExist(err) {
			log.Printf("Не вдалося видалити частковий файл %s: %v", partFile, err)
		}

		if attempt < maxAttempts {
			time.Sleep(retryDelay)
		}
	}

	if downloadErr != nil {
		log.Printf("Не вдалося завантажити відео після %d спроб (%s): %v", maxAttempts, platform, downloadErr)
		errMsg := fmt.Sprintf("Не вдалося завантажити відео після %d спроб (%s): %v", maxAttempts, platform, downloadErr)
		_, editErr := ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
			ID:      sentMsg.GetID(),
			Message: errMsg,
		})
		if editErr != nil {
			log.Printf("Помилка редагування повідомлення: %v", editErr)
		}
		return downloadErr
	}

	ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
		ID:      sentMsg.GetID(),
		Message: "Перевірка і формування медіа перед відправкою: \n[◼◼◼◼◼◼◻◻]",
	})

	videoName := yt.GetVideoName(url, info)
	file, err := os.Stat(videoName)
	if err != nil {
		logMsg := "Помилка перевірки файлу відео"
		if os.IsNotExist(err) {
			logMsg = "Файл відео не існує: " + videoName
		}
		log.Println(logMsg)
		_, editErr := ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
			ID:      sentMsg.GetID(),
			Message: "Помилка: не вдалося завантажити відео: " + logMsg,
		})
		if editErr != nil {
			log.Printf("Помилка редагування повідомлення: %v", editErr)
		}
		return err
	}
	if file.IsDir() {
		log.Printf("Файл %s є директорією", videoName)
		_, editErr := ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
			ID:      sentMsg.GetID(),
			Message: "Помилка: завантажений файл є директорією.",
		})
		if editErr != nil {
			log.Printf("Помилка редагування повідомлення: %v", editErr)
		}
		return nil
	}

	videoFile, err := uploader.NewUploader(ctx.Raw).FromPath(ctx, videoName)
	if err != nil {
		log.Printf("Помилка завантаження відео в Telegram: %v", err)
		logErr := fmt.Sprintf("Помилка завантаження відео в Telegram: \n%v", err)
		_, editErr := ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
			ID:      sentMsg.GetID(),
			Message: logErr,
		})
		if editErr != nil {
			log.Printf("Помилка редагування повідомлення: %v", editErr)
		}
		return err
	}

	thumbName := yt.GetThumb(url, info)
	var thumbFile tg.InputFileClass
	if thumbName != "" {
		thumbFileStat, err := os.Stat(thumbName)
		if err == nil && !thumbFileStat.IsDir() {
			thumbFile, err = uploader.NewUploader(ctx.Raw).FromPath(ctx, thumbName)
			if err != nil {
				log.Printf("Помилка завантаження прев’ю: %v", err)
				thumbFile = nil
			}
		} else {
			log.Printf("Прев’ю недоступне або є директорією: %s", thumbName)
			thumbFile = nil
		}
	}

	media := &tg.InputMediaUploadedDocument{
		File:     videoFile,
		MimeType: "video/mp4",
		Thumb:    thumbFile,
		Attributes: []tg.DocumentAttributeClass{
			&tg.DocumentAttributeVideo{
				SupportsStreaming: true,
			},
			&tg.DocumentAttributeFilename{
				FileName: videoName,
			},
		},
	}

	ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
		ID:      sentMsg.GetID(),
		Message: "Надсилання відео: \n[◼◼◼◼◼◼◼◻]",
	})

	_, err = ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
		ID:      sentMsg.GetID(),
		Message: info.Title,
		Media:   media,
	})
	if err != nil {
		log.Printf("Помилка редагування повідомлення з відео: %v", err)
		return err
	}

	if viper.GetBool("delete_url") {
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

	if err := os.Remove(videoName); err != nil {
		log.Printf("Не вдалося видалити відео: %v", err)
	}
	if thumbName != "" {
		if err := os.Remove(thumbName); err != nil {
			log.Printf("Не вдалося видалити прев’ю: %v", err)
		}
	}
	return nil
}

func audio(ctx *ext.Context, update *ext.Update) error {
	allowedChatId, err := strconv.Atoi(os.Getenv("CHAT_ID"))
	if err != nil {
		log.Fatalln("Не вдалося отримати chatID")
	}
	chat := update.EffectiveChat()
	chatID := update.EffectiveChat().GetID()
	user := update.EffectiveUser()

	if chat == nil || chatID != int64(allowedChatId) {
		// Неавторизований доступ
		fmt.Printf("Неавторизований доступ: %s \n", user.Username)
		return nil
	}

	msg := update.EffectiveMessage
	text := msg.Text
	var isValid bool

	url, _ := yt.GetYoutubeURL(text)

	if !isValid || len(url) == 0 || !yt.IsUrl(url) {
		return nil
	}
	yt.DownloadAudio(url)
	return nil
}
