package main

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Geergon/yt-dlp-goTelegramBot/internal/tgbot"
	"github.com/Geergon/yt-dlp-goTelegramBot/internal/yt"
	"github.com/fsnotify/fsnotify"
	"github.com/glebarez/sqlite"
	"github.com/gotd/td/tg"
	"github.com/spf13/viper"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/celestix/gotgproto"
	"github.com/celestix/gotgproto/dispatcher/handlers"
	"github.com/celestix/gotgproto/dispatcher/handlers/filters"
	"github.com/celestix/gotgproto/ext"
	"github.com/celestix/gotgproto/sessionMaker"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func init() {
	logFile, err := os.OpenFile("bot.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
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

	go startCleanupRoutine()
}

var (
	viperMutex     sync.RWMutex
	bot            *tgbotapi.BotAPI
	urlQueue       = make(chan tgbot.URLRequest, 100)
	semaphore      = make(chan struct{}, 2)
	processingURLs = sync.Map{}
)

func main() {
	viperMutex.Lock()
	viper.SetConfigName("config")
	viper.SetConfigType("toml")
	viper.AddConfigPath("./config")
	viper.SetDefault("auto_download", true)
	viper.SetDefault("delete_url", true)
	viper.SetDefault("allowed_user", []int{})
	viper.SetDefault("allowed_chat", []int{})
	viper.SetDefault("yt-dlp_filter", "bv[filesize<500M][ext=mp4]+ba[ext=m4a]/bv[height=720][filesize<400M][ext=mp4]+ba[ext=m4a]/bv[height=480][filesize<300M][ext=mp4]+ba[ext=m4a]")
	viper.SetDefault("duration", "600")
	viper.SetDefault("long_video_download", false)
	viper.SetDefault("live_filter", "!is_live & !was_live")
	viper.SafeWriteConfig()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Println("Конфіг файл не знайдений")
		} else {
			log.Printf("Помилка з конфіг файлом: %v", err)
		}
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

	bot, err = tgbotapi.NewBotAPI(botToken) // Замініть на ваш токен бота
	if err != nil {
		log.Fatalf("Помилка ініціалізації бота: %v", err)
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

	go StartWorkers(client, 2)
	dispatcher := client.Dispatcher

	dispatcher.AddHandlerToGroup(handlers.NewMessage(filters.Message.Text, func(ctx *ext.Context, u *ext.Update) error {
		url, isValid, platform := tgbot.Url(u)

		if u.EffectiveMessage.EditDate != 0 {
			return nil
		}

		if !isValid {
			return nil
		}

		if strings.Contains(url, "&list=") || strings.Contains(url, "?list=") {
			log.Printf("URL %s містить параметр list, пропускаємо", url)
			return nil
		}

		// Перевіряємо, чи URL уже обробляється
		_, loaded := processingURLs.LoadOrStore(url, struct{}{})
		if loaded {
			log.Printf("URL %s уже обробляється, пропускаємо", url)
			return nil
		}

		// Додаємо URL до черги
		urlQueue <- tgbot.URLRequest{URL: url, Platform: platform, Command: "auto", Context: ctx, Update: u}
		return nil
	}), 1)
	// dispatcher.AddHandlerToGroup(handlers.NewMessage(filters.Message.Text, tgbot.Echo), 1)
	dispatcher.AddHandler(handlers.NewCommand("logs", tgbot.SendLogs))
	dispatcher.AddHandler(handlers.NewCommand("update", tgbot.UpdateYtdlp))
	dispatcher.AddHandler(handlers.NewCommand("fragment", Fragment))
	dispatcher.AddHandler(handlers.NewCommand("audio", Audio))
	dispatcher.AddHandler(handlers.NewCommand("download", Download))
	dispatcher.AddHandler(handlers.NewCommand("start", func(ctx *ext.Context, u *ext.Update) error {
		chatID := u.EffectiveChat().GetID()
		_, err := ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
			Message: `Ласкаво просимо! Надішліть URL з YouTube, TikTok або Instagram для завантаження відео.\n
			Команди:\n
			/logs - отримати логи\n
			/update - оновити yt-dlp і gallery-dl\n
			/fragment - завантажити фрагмент відео\n 
			/download - ручне завантаження відео\n
			/audio - завантажити аудіо`,
		})
		if err != nil {
			log.Printf("Помилка надсилання повідомлення: %v", err)
			return err
		}
		return nil
	}))
	dispatcher.AddHandler(handlers.NewCommand("settings", tgbot.Settings))
	dispatcher.AddHandler(handlers.NewCallbackQuery(filters.CallbackQuery.Prefix("cb_settings_"), tgbot.SettingsCallback))

	fmt.Printf("Бот (@%s) стартував...\n", client.Self.Username)

	client.Idle()
}

func StartWorkers(client *gotgproto.Client, numWorkers int) {
	for i := range numWorkers {
		go func(workerID int) {
			for req := range urlQueue {
				// Захоплюємо семафор
				log.Printf("Черга URL: %v", urlQueue)
				semaphore <- struct{}{}
				log.Printf("Воркер %d обробляє URL: %s (команда: %s)", workerID, req.URL, req.Command)
				err := tgbot.ProcessURL(req)
				if err != nil {
					log.Printf("Помилка обробки URL %s: %v", req.URL, err)
					processingURLs.Delete(req.URL)
				}

				processingURLs.Delete(req.URL)
				// Звільняємо семафор
				<-semaphore
			}
		}(i)
	}
}

func startCleanupRoutine() {
	const cleanupInterval = 30 * time.Minute  // Очищення кожні 30 хвилин
	const fileAgeThreshold = 10 * time.Minute // Видаляємо файли, старші за 1 годину

	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		// Перевіряємо, чи всі воркери вільні
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

func cleanOldFiles(threshold time.Duration) error {
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

func Audio(ctx *ext.Context, update *ext.Update) error {
	chatID := tgbot.Access(ctx, update)
	if chatID == 0 {
		log.Println("Відмова у доступі")
		return nil
	}

	url, isValid, platform := tgbot.Url(update)
	if !isValid {
		_, err := ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
			Message: "Невалідне URL або платформа не відповідає",
		})
		return err
	}

	if strings.Contains(url, "&list=") || strings.Contains(url, "?list=") {
		log.Printf("URL %s містить параметр list, пропускаємо", url)
		return nil
	}

	urlQueue <- tgbot.URLRequest{
		URL:      url,
		Platform: platform,
		Command:  "audio",
		Context:  ctx,
		Update:   update,
	}
	return nil
}

func Fragment(ctx *ext.Context, update *ext.Update) error {
	chatID := tgbot.Access(ctx, update)
	if chatID == 0 {
		log.Println("Відмова у доступі")
		return nil
	}

	args := strings.Fields(update.EffectiveMessage.Text)
	if len(args) != 3 {
		_, err := ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
			Message: "Використання: /fragment <YouTube_URL> <00:00-00:00>\nПриклад: /fragment https://www.youtube.com/watch?v=XYZ 05:00-07:00",
		})
		return err
	}

	url := args[1]
	fragment := args[2]

	if strings.Contains(url, "help") {
		_, err := ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
			Message: "Використання: /fragment <YouTube_URL> <00:00-00:00>\nПриклад: /fragment https://www.youtube.com/watch?v=XYZ 05:00-07:00",
		})
		return err
	}

	urlQueue <- tgbot.URLRequest{
		URL:      url,
		Command:  "fragment",
		Fragment: fragment,
		Context:  ctx,
		Update:   update,
	}
	return nil
}

func Download(ctx *ext.Context, update *ext.Update) error {
	chatID := tgbot.Access(ctx, update)
	if chatID == 0 {
		log.Println("Відмова у доступі")
		return nil
	}

	var url, platform string
	var isValid bool

	if update.EffectiveMessage.ReplyTo != nil {

		log.Println("Команда є відповіддю")
		replyToMsgID := update.EffectiveMessage.ReplyTo.(*tg.MessageReplyHeader).ReplyToMsgID
		log.Printf("ReplyToMsgID: %d", replyToMsgID)

		config := tgbotapi.NewUpdate(0)
		config.Timeout = 60
		config.Limit = 1
		config.Offset = -1
		msgUpdates, err := bot.GetUpdates(config)
		if err != nil {
			log.Printf("Помилка отримання оновлень: %v", err)
			return err
		}
		if len(msgUpdates) == 0 {
			log.Println("Жодного оновлення не отримано")
			return err
		}
		tgMsg := msgUpdates[0].Message
		if tgMsg == nil || tgMsg.ReplyToMessage == nil {
			log.Printf("Повідомлення з ID %d не містить ReplyToMessage", update.EffectiveMessage.ID)
			return err
		}
		if tgMsg.ReplyToMessage.MessageID != replyToMsgID {
			log.Printf("ReplyToMsgID не співпадає: очікувалося %d, отримано %d", replyToMsgID, tgMsg.ReplyToMessage.MessageID)
			return err
		}
		replyText := tgMsg.ReplyToMessage.Text
		log.Printf("ReplyToMessage text: %s", replyText)
		if replyText == "" {
			log.Println("ReplyToMessage не містить тексту")
			return err
		}
		url, isValid, platform = tgbot.UrlFromText(replyText)
		// platform = "YouTube"
		// url, isValid = yt.GetYoutubeURL(replyText)
	} else {
		log.Println("Команда не є відповіддю")
		url, isValid, platform = tgbot.Url(update)
		if !isValid {
			log.Println("Невалідне URL або платформа не підтримується")
			_, err := ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
				Message: "Некоректний URL або платформа не підтримується",
			})
			return err
		}
	}

	if !yt.IsUrl(url) {
		log.Println("Повідомлення не містить url")
		return nil
	}

	if strings.Contains(url, "&list=") || strings.Contains(url, "?list=") {
		log.Printf("URL %s містить параметр list, пропускаємо", url)
		return nil
	}

	_, loaded := processingURLs.LoadOrStore(url, struct{}{})
	if loaded {
		log.Printf("URL %s уже обробляється, пропускаємо", url)
		return nil
	}

	urlQueue <- tgbot.URLRequest{
		URL:      url,
		Platform: platform,
		Command:  "download",
		Context:  ctx,
		Update:   update,
	}
	log.Printf("Додано до черги URL: %s, Platform: %s, Command: download", url, platform)
	return nil
}
