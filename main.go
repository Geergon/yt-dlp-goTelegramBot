package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	yt "github.com/Geergon/yt-dlp-goTelegramBot/internal/yt"
	"github.com/fsnotify/fsnotify"
	"github.com/glebarez/sqlite"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
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

var viperMutex sync.RWMutex

func main() {
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

	viperMutex.Lock()
	viper.SetConfigName("config")
	viper.SetConfigType("toml")
	viper.AddConfigPath("./config")
	viper.SetDefault("auto_download", true)
	viper.SetDefault("delete_url", true)
	viper.SetDefault("allowed_user", []int{})
	viper.SetDefault("allowed_chat", []int{})
	viper.SetDefault("yt-dlp_filter", "bv[filesize<500M][ext=mp4]+ba[ext=m4a]/bv[height=720][filesize<400M][ext=mp4]+ba[ext=m4a]/bv[height=480][filesize<300M][ext=mp4]+ba[ext=m4a]")
	viper.SafeWriteConfig()
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Println("Конфіг файл не знайдений")
		} else {
			log.Printf("Помилка з конфіг файлом: %v", err)
		}
	}

	viperMutex.Unlock()
	viper.WatchConfig()
	viper.OnConfigChange(func(e fsnotify.Event) {
		viperMutex.Lock()
		defer viperMutex.Unlock()
		log.Printf("Конфігурація змінена: %s", e.Name)
		if err := viper.ReadInConfig(); err != nil {
			log.Printf("Помилка перечитування конфігурації: %v", err)
			return
		}
	})

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
	dispatcher.AddHandler(handlers.NewCommand("fragment", fragment))
	dispatcher.AddHandler(handlers.NewCommand("download", download))
	dispatcher.AddHandler(handlers.NewCommand("start"+client.Self.Username, func(ctx *ext.Context, u *ext.Update) error {
		chatID := u.EffectiveChat().GetID()
		_, err := ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
			Message: `Ласкаво просимо! Надішліть URL з YouTube, TikTok або Instagram для завантаження відео.\n
			Команди:\n
			/logs - отримати логи\n
			/update - оновити yt-dlp\n
			/fragments - завантажити фрагмент відео, 
			/download - ручне завантаження відео`,
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
	viperMutex.RLock()
	autoDownload := viper.GetBool("auto_download")
	viperMutex.RUnlock()
	if autoDownload {
		msg := update.EffectiveMessage
		text := msg.Text
		if strings.Contains(text, "/download") {
			return nil
		}
		download(ctx, update)
	}
	return nil
}

func sendLogs(ctx *ext.Context, update *ext.Update) error {
	chatID := access(ctx, update)
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

func updateYtdlp(ctx *ext.Context, update *ext.Update) error {
	chatID := access(ctx, update)
	if chatID == 0 {
		log.Println("Відмова у доступі")
		return nil
	}

	ytdlp := yt.UpdateYtdlp()
	gallerydl := yt.UpdateGallerydl()
	msg := fmt.Sprintf("%s\n\n%s", ytdlp, gallerydl)
	ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
		Message: msg,
	})

	return nil
}

func settings(ctx *ext.Context, update *ext.Update) error {
	chatID := access(ctx, update)
	if chatID == 0 {
		log.Println("Відмова у доступі")
		return nil
	}

	viperMutex.RLock()
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
	viperMutex.RUnlock()

	_, _ = ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
		Message: "⚙️ Налаштування бота:\nВиберіть опцію для увімкнення/вимкнення.",
		ReplyMarkup: &tg.ReplyInlineMarkup{
			Rows: rows,
		},
	})
	// if err != nil {
	// 	log.Printf("Помилка надсилання повідомлення /settings: %v", err)
	// 	return err
	// }

	return nil
}

func boolToEmoji(b bool) string {
	if b {
		return "✅"
	}
	return "❌"
}

func settingsCallback(ctx *ext.Context, u *ext.Update) error {
	chatID := access(ctx, u)
	if chatID == 0 {
		log.Println("Відмова у доступі")
		return nil
	}

	callback := u.CallbackQuery
	data := callback.Data
	messageID := callback.MsgID

	viperMutex.Lock()
	autoDownload := viper.GetBool("auto_download")
	deleteUrl := viper.GetBool("delete_url")
	switch string(data) {
	case "cb_settings_auto_download":
		viper.Set("auto_download", !autoDownload)
	case "cb_settings_delete_links":
		viper.Set("delete_url", !deleteUrl)
	default:
		log.Printf("Невідомий callback: %s", data)
		viperMutex.Unlock()
		return nil
	}
	if err := viper.WriteConfig(); err != nil {
		log.Printf("Помилка збереження конфігурації: %v", err)
		viperMutex.Unlock()
		return err
	}
	viperMutex.Unlock()

	viperMutex.RLock()
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
	viperMutex.RUnlock()

	_, _ = ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
		ID:      messageID,
		Message: "⚙️ Налаштування бота:",
		ReplyMarkup: &tg.ReplyInlineMarkup{
			Rows: rows,
		},
	})
	// if err != nil {
	// 	log.Printf("Помилка редагування повідомлення: %v", err)
	// 	return err
	// }

	_, _ = ctx.AnswerCallback(&tg.MessagesSetBotCallbackAnswerRequest{
		QueryID: callback.QueryID,
	})
	// if err != nil {
	// 	log.Printf("Помилка відповіді на callback: %v", err)
	// 	return err
	// }

	return nil
}

func download(ctx *ext.Context, update *ext.Update) error {
	chatID := access(ctx, update)
	if chatID == 0 {
		log.Println("Відмова у доступі")
		return nil
	}

	msg := update.EffectiveMessage
	text := msg.Text

	if strings.Contains(text, "/fragment") {
		return nil
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
		return nil
	}

	sentMsg, err := ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
		Message: "Обробка посилання: \n[◼◼◻◻◻◻◻◻]",
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
	}

	ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
		ID:      sentMsg.GetID(),
		Message: "Завантаження відео: \n[◼◼◼◼◻◻◻◻]",
	})
	var downloadFunc func(string) (bool, error)
	var mediaFileName string
	switch platform {
	case "YouTube":
		downloadFunc = func(url string) (bool, error) {
			err := yt.DownloadYTVideo(url)
			return false, err // Always video
		}
		mediaFileName = "output.mp4"
	case "TikTok":
		downloadFunc = yt.DownloadTTVideo // Returns isPhoto
		mediaFileName = "output.mp4"
	case "Instagram":
		downloadFunc = yt.DownloadInstaVideo
		mediaFileName = "output.mp4"
	}

	const maxAttempts = 3
	const retryDelay = 10 * time.Second

	var downloadErr error
	var isPhoto bool
	for attempt := 1; attempt <= maxAttempts; attempt++ {

		isPhoto, downloadErr = downloadFunc(url)
		if downloadErr == nil {
			break
		}
		log.Printf("Спроба %d завантаження (%s) не вдалася: %v", attempt, platform, downloadErr)

		videoName := "output.mp4"
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

	if platform == "TikTok" && isPhoto || platform == "Instagram" && isPhoto {
		mediaFileName = "output.jpg"
	}
	file, err := os.Stat(mediaFileName)
	if err != nil {
		logMsg := "Помилка перевірки файлу відео"
		if os.IsNotExist(err) {
			logMsg = "Файл відео не існує: " + mediaFileName
		}
		log.Println(logMsg)
		ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
			ID:      sentMsg.GetID(),
			Message: "Помилка: не вдалося завантажити відео: " + logMsg,
		})
		return err
	}
	if file.IsDir() {
		log.Printf("Файл %s є директорією", mediaFileName)
		ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
			ID:      sentMsg.GetID(),
			Message: "Помилка: завантажений файл є директорією.",
		})
		return nil
	}

	fileData, err := uploader.NewUploader(ctx.Raw).FromPath(ctx, mediaFileName)
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

	var thumbName string
	var media tg.InputMediaClass
	if isPhoto {
		media = &tg.InputMediaUploadedPhoto{
			File: fileData,
		}
	} else {
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

		if thumbName = yt.GetThumb(url); thumbName != "" {
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
	}

	ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
		ID:      sentMsg.GetID(),
		Message: "Надсилання відео: \n[◼◼◼◼◼◼◼◻]",
	})
	var title string
	if info.Title == "" {
		title = "Не вдалося отримати назву відео"
	}
	if isPhoto {
		title = "."
	} else {
		title = info.Title
	}
	_, err = ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
		ID:      sentMsg.GetID(),
		Message: title,
		Media:   media,
	})
	if err != nil {
		log.Printf("Помилка редагування повідомлення з відео: %v", err)
		return err
	}

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

	if err := os.Remove(mediaFileName); err != nil {
		log.Printf("Не вдалося видалити медіа: %v", err)
	}
	if !isPhoto {
		if thumbName != "" {
			if err := os.Remove(thumbName); err != nil {
				log.Printf("Не вдалося видалити прев’ю: %v", err)
			}
		}
	}
	return nil
}

func fragment(ctx *ext.Context, u *ext.Update) error {
	chatID := access(ctx, u)
	if chatID == 0 {
		log.Println("Відмова у доступі")
		return nil
	}

	args := strings.Fields(u.EffectiveMessage.Text)
	if len(args) != 3 {
		_, err := ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
			Message: "Використання: /fragment <YouTube_URL> <00:00-00:00>\nПриклад, який завантажить відео з п'ятої хвилини відео по сьому хвилину: /fragment https://www.youtube.com/watch?v=XYZ 05:00-07:00 \n Можна вказувати секунди '00:10-00:50' і години '01:01:00-01:03:00'",
		})
		return err
	}

	sentMsg, err := ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
		Message: "Обробка посилання: \n[◼◼◻◻◻◻◻◻]",
	})

	url := args[1]
	fragment := args[2]

	if strings.Contains(url, "help") {
		ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
			Message: "Використання: /fragment <YouTube_URL> <00:00-00:00>\nПриклад, який завантажить відео з п'ятої хвилини відео по сьому хвилину: /fragment https://www.youtube.com/watch?v=XYZ 05:00-07:00 \nМожна вказувати секунди '00:10-00:50' і години '01:01:00-01:03:00'",
		})
	}
	var title string
	info, err := yt.GetVideoInfo(url)
	if err != nil {
		title = "Не вдалося отримати назву відео"
	}
	title = info.Title

	ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
		ID:      sentMsg.GetID(),
		Message: "Завантаження відео і вирізання потрібного фрагменту: \n[◼◼◼◼◻◻◻◻]",
	})

	viperMutex.RLock()
	filter := viper.GetString("yt-dlp_filter")
	viperMutex.RUnlock()
	outputFile := "outputFrag.%(ext)s"
	cmd := exec.Command(
		"yt-dlp",
		"--download-sections", fmt.Sprintf("*%s", fragment),
		"-f", filter,
		"-o", outputFile,
		url,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("yt-dlp error: %v\nOutput: %s", err, string(output))
	}

	log.Printf("Завантаження %s завершено успішно", url)

	ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
		ID:      sentMsg.GetID(),
		Message: "Перевірка і формування медіа перед відправкою: \n[◼◼◼◼◼◼◻◻]",
	})

	outputFile = "outputFrag.mp4"
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		_, err := ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
			ID:      sentMsg.GetID(),
			Message: "Не вдалося завантажити фрагмент",
		})
		return err
	}

	videoFile, err := uploader.NewUploader(ctx.Raw).FromPath(ctx, outputFile)
	if err != nil {
		log.Printf("Помилка завантаження відео в Telegram: %v", err)
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
				FileName: outputFile,
			},
		},
	}

	if thumbName = yt.GetThumb(url); thumbName != "" {
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

	ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
		ID:      sentMsg.GetID(),
		Message: "Перевірка і формування медіа перед відправкою: \n[◼◼◼◼◼◼◼◻]",
	})
	_, err = ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
		ID:      sentMsg.GetID(),
		Message: title,
		Media:   media,
	})
	if err != nil {
		log.Printf("Помилка відправки медіа: %v", err)
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

	return err
}

func isValidTimeFormat(t string) bool {
	_, err := time.Parse("15:04:05", t)
	return err == nil
}

func access(ctx *ext.Context, update *ext.Update) int64 {
	allowedChatId, _ := strconv.Atoi(os.Getenv("CHAT_ID"))
	chatID := update.EffectiveChat().GetID()
	user := update.EffectiveUser()

	viperMutex.RLock()
	allowedChats := viper.GetIntSlice("allowed_chat")
	viperMutex.RUnlock()

	isAuthorized := false
	for _, chat := range allowedChats {
		if int64(chat) == chatID {
			isAuthorized = true
			break
		}
	}
	if chatID == int64(allowedChatId) {
		isAuthorized = true
	} else {
		viperMutex.RLock()
		allowedUsers := viper.GetIntSlice("allowed_user")
		viperMutex.RUnlock()
		for _, allowedUserID := range allowedUsers {
			if int64(allowedUserID) == user.ID {
				isAuthorized = true
				break
			}
		}
	}
	if !isAuthorized {
		log.Printf("Неавторизований доступ: %s (UserID: %d, ChatID: %d)", user.Username, user.ID, chatID)
		return 0
	}
	return chatID
}
