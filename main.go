package main

import (
	"database/sql"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Geergon/yt-dlp-goTelegramBot/internal/database"
	"github.com/Geergon/yt-dlp-goTelegramBot/internal/tgbot"
	"github.com/Geergon/yt-dlp-goTelegramBot/internal/yt"
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
	whitelistDb    *sql.DB
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
		log.Printf("CHAT_ID не задано")
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

	whitelistDb, err = database.InitDB("./db/whitelist.db")
	if err != nil {
		log.Fatal(err)
	}
	defer whitelistDb.Close()

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

	dispatcher.AddHandler(handlers.NewCommand("gif", func(ctx *ext.Context, update *ext.Update) error {
		chatID := tgbot.Access(ctx, update, whitelistDb)
		if chatID == 0 {
			log.Println("Відмова у доступі")
			return nil
		}
		msg := update.EffectiveMessage
		text := msg.Text

		dir, filename, err := tgbot.GetVideo(ctx, update)
		if err != nil {
			log.Println(err)
			return err
		}

		if !strings.HasPrefix(text, "/") {
			return nil
		}

		u := strings.Fields(text)
		if len(u) == 0 {
			return nil
		}

		var textBott string
		var textTop string

		re := regexp.MustCompile(`(\w+)=(?:"([^"]*)"|(\S+))`)
		matches := re.FindAllStringSubmatch(text, -1)

		for _, match := range matches {
			if match[1] == "textbott" {
				if match[2] != "" {
					textBott = match[2]
				}
			}
			if match[1] == "texttop" {
				if match[2] != "" {
					textTop = match[2]
				}
			}
			// fmt.Printf("\nСпівпадіння %d:\n", i+1)
			// fmt.Printf("  [0] Повне: '%s'\n", match[0])
			// fmt.Printf("  [1] Ключ:  '%s'\n", match[1])
			// fmt.Printf("  [2] Значення в лапках: '%s'\n", match[2])
			// fmt.Printf("  [3] Значення без лапок: '%s'\n", match[3])
		}

		gifFilename, err := tgbot.MakeGif(dir, filename, textBott, textTop)
		if err != nil {
			log.Println("не вдалося створити гіфку: ", err)
			return err
		}

		gifPath := filepath.Join(dir, gifFilename)

		f, err := uploader.NewUploader(ctx.Raw).FromPath(ctx, gifPath)
		if err != nil {
			panic(err)
		}

		_, err = ctx.SendMedia(chatID, &tg.MessagesSendMediaRequest{
			// Message: "This is your caption",
			Media: &tg.InputMediaUploadedDocument{
				File:     f,
				MimeType: "image/gif",
				Attributes: []tg.DocumentAttributeClass{
					&tg.DocumentAttributeAnimated{},
				},
			},
		})
		if err != nil {
			os.RemoveAll(dir)
			return err
		}
		os.RemoveAll(dir)
		return nil
	}))

	dispatcher.AddHandlerToGroup(handlers.NewMessage(filters.Message.Text, func(ctx *ext.Context, u *ext.Update) error {
		chatID := tgbot.Access(ctx, u, whitelistDb)
		if chatID == 0 {
			log.Println("Відмова у доступі")
			return nil
		}

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

	dispatcher.AddHandler(handlers.NewCommand("logs", func(ctx *ext.Context, update *ext.Update) error {
		isAccessAllowed := tgbot.AdminAccess(ctx, update, whitelistDb)
		if !isAccessAllowed {
			log.Println("Відмова у доступі")
			return nil
		}
		tgbot.SendLogs(ctx, update)
		return nil
	}))

	dispatcher.AddHandler(handlers.NewCommand("update", func(ctx *ext.Context, update *ext.Update) error {
		chatID := tgbot.Access(ctx, update, whitelistDb)
		if chatID == 0 {
			log.Println("Відмова у доступі")
			return nil
		}
		tgbot.UpdateYtdlp(ctx, update)
		return nil
	}))

	dispatcher.AddHandler(handlers.NewCommand("fragment", Fragment))
	dispatcher.AddHandler(handlers.NewCommand("audio", Audio))
	dispatcher.AddHandler(handlers.NewCommand("download", Download))
	dispatcher.AddHandler(handlers.NewCommand("start", func(ctx *ext.Context, u *ext.Update) error {
		chatID := u.EffectiveChat().GetID()
		_, err := ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
			Message: `
Ласкаво просимо! Надішліть URL з YouTube, TikTok, Instagram для завантаження відео і фото.
Команди:
/fragment - завантажити фрагмент відео. Приклад: /fragment https://www.youtube.com/watch?v=XYZ 05:00-07:00
/download - ручне завантаження відео, дозволяє завантажувати довгі відео з ютуба, а також фото і відео з практичного будь-якого сайту, якщо це підтримує yt-dlp і gallery-dl. Приклад: /download https://x.com/AndriySadovyi/status/1974485263251582997
/audio - завантажити аудіо. Приклад: /audio https://www.youtube.com/watch?v=guDJvZp5Bqk
`,
		})
		if err != nil {
			log.Printf("Помилка надсилання повідомлення: %v", err)
			return err
		}
		return nil
	}))

	dispatcher.AddHandler(handlers.NewCommand("admin_help", func(ctx *ext.Context, update *ext.Update) error {
		isAccessAllowed := tgbot.AdminAccess(ctx, update, whitelistDb)
		if !isAccessAllowed {
			log.Println("Відмова у доступі")
			return nil
		}
		chatID := update.EffectiveChat().GetID()

		_, err := ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
			Message: `
Список команд доступних для адмінів.
Команди:
/add_to_whitelist - Додати користувача у вайтлист (підтримує декілька аргументів), приклад: /add_to_whitelist id:@username id:@username ... 
/check_whitelist - Переглянути вайтлист
/delete_from_whitelist - Видалити одного або декількох користувачів в вайтлиста, приклад: /delete_from_whitelist @username @username ...
`,
		})
		if err != nil {
			log.Printf("Помилка надсилання повідомлення: %v", err)
			return err
		}
		return nil
	}))

	dispatcher.AddHandler(handlers.NewCommand("help", func(ctx *ext.Context, update *ext.Update) error {
		chatID := update.EffectiveChat().GetID()

		_, err := ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
			Message: `
Ласкаво просимо! Надішліть URL з YouTube, TikTok, Instagram для завантаження відео і фото.
Команди:
/fragment - завантажити фрагмент відео. Приклад: /fragment https://www.youtube.com/watch?v=XYZ 05:00-07:00
/download - ручне завантаження відео, дозволяє завантажувати довгі відео з ютуба, а також фото і відео з практичного будь-якого сайту, якщо це підтримує yt-dlp і gallery-dl. Приклад: /download https://x.com/AndriySadovyi/status/1974485263251582997
/audio - завантажити аудіо. Приклад: /audio https://www.youtube.com/watch?v=guDJvZp5Bqk
/gif Зробити gif з відео, можна додати текст до гіфки (текст обов'язково має бути в лапках). Щоб зробити гіфку, треба вибрати завантажити "Фото або відео" і коментарі прописати команду: /gif textbott="нижній текст" texttop="верхній текст" (якщо текст не треба, просто написати /gif і все
`,
		})
		if err != nil {
			log.Printf("Помилка надсилання повідомлення: %v", err)
			return err
		}
		return nil
	}))

	dispatcher.AddHandler(handlers.NewCommand("add_to_whitelist", func(ctx *ext.Context, u *ext.Update) error {
		err := AddIdToWhitelist(ctx, u, whitelistDb)
		if err != nil {
			log.Println(err)
			return err
		}
		return nil
	}))

	dispatcher.AddHandler(handlers.NewCommand("check_whitelist", func(ctx *ext.Context, u *ext.Update) error {
		err := GetWhitelist(ctx, u, whitelistDb)
		if err != nil {
			log.Println(err)
			return err
		}
		return nil
	}))

	dispatcher.AddHandler(handlers.NewCommand("delete_from_whitelist", func(ctx *ext.Context, u *ext.Update) error {
		err := DeleteFromWhitelist(ctx, u, whitelistDb)
		if err != nil {
			log.Println(err)
			return err
		}
		return nil
	}))

	dispatcher.AddHandler(handlers.NewCommand("settings", func(ctx *ext.Context, update *ext.Update) error {
		chatID := tgbot.Access(ctx, update, whitelistDb)
		if chatID == 0 {
			log.Println("Відмова у доступі")
			return nil
		}

		tgbot.Settings(ctx, update)
		return nil
	}))

	dispatcher.AddHandler(handlers.NewCallbackQuery(filters.CallbackQuery.Prefix("cb_settings_"), func(ctx *ext.Context, update *ext.Update) error {
		chatID := tgbot.Access(ctx, update, whitelistDb)
		if chatID == 0 {
			log.Println("Відмова у доступі")
			return nil
		}

		tgbot.SettingsCallback(ctx, update)
		return nil
	}))

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
	// Remove temp audio dir
	tempDir := os.TempDir()
	files, err := os.ReadDir(tempDir)
	if err != nil {
		return err
	}

	prefix := "audio-download-"
	lifetime := 10 * time.Minute
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

func Audio(ctx *ext.Context, update *ext.Update) error {
	chatID := tgbot.Access(ctx, update, whitelistDb)
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

	_, loaded := processingURLs.LoadOrStore(url, struct{}{})
	if loaded {
		log.Printf("URL %s уже обробляється, пропускаємо", url)
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
	chatID := tgbot.Access(ctx, update, whitelistDb)
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

	_, loaded := processingURLs.LoadOrStore(url, struct{}{})
	if loaded {
		log.Printf("URL %s уже обробляється, пропускаємо", url)
		return nil
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
	chatID := tgbot.Access(ctx, update, whitelistDb)
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

func AddIdToWhitelist(ctx *ext.Context, update *ext.Update, db *sql.DB) error {
	isAccessAllowed := tgbot.AdminAccess(ctx, update, whitelistDb)
	if !isAccessAllowed {
		log.Println("Відмова у доступі")
		return nil
	}
	chatID := update.EffectiveChat().GetID()

	msg := update.EffectiveMessage
	text := msg.Text

	if !strings.HasPrefix(text, "/") {
		return nil
	}

	u := strings.Fields(text)
	if len(u) == 0 {
		return nil
	}

	// command := u[0]
	args := u[1:]
	var message string

	for _, a := range args {
		s := strings.Split(a, ":")
		if len(s) == 2 {
			id := s[0]
			username := s[1]
			if _, err := strconv.Atoi(id); err == nil && strings.HasPrefix(username, "@") {
				idInt64, err := strconv.ParseInt(id, 10, 64)
				if err != nil {
					log.Panicln("Не вдалося перетворити id з типу string на int64")
					return err
				}
				err = database.InsertIntoWhitelist(db, username, idInt64)
				if err != nil {
					log.Printf("Не вдалося вставити значення в БД: %v", err)
					return err
				}
				s := fmt.Sprintf("Користувач %s був успішно доданий в вайтлист\n", username)
				message += s
			}
		}
	}
	_, err := ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
		Message: message,
	})
	if err != nil {
		log.Printf("Помилка надсилання повідомлення: %v", err)
		return err
	}

	return nil
}

func GetWhitelist(ctx *ext.Context, update *ext.Update, db *sql.DB) error {
	isAccessAllowed := tgbot.AdminAccess(ctx, update, whitelistDb)
	if !isAccessAllowed {
		log.Println("Відмова у доступі")
		return nil
	}
	chatID := update.EffectiveChat().GetID()

	msg := update.EffectiveMessage
	text := msg.Text

	if !strings.HasPrefix(text, "/") {
		return nil
	}

	whitelist, err := database.GetAllWhitelist(db)
	if err != nil {
		log.Println(err)
		return err
	}
	if len(whitelist) == 0 {
		_, err = ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
			Message: "Вайтліст пустий",
		})
	}

	var message string
	for _, w := range whitelist {
		s := fmt.Sprintf("%s: %d\n", w.Username, w.Id)
		message += s
	}

	_, err = ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
		Message: message,
	})
	if err != nil {
		log.Printf("Помилка надсилання повідомлення: %v", err)
		return err
	}

	return nil
}

func DeleteFromWhitelist(ctx *ext.Context, update *ext.Update, db *sql.DB) error {
	isAccessAllowed := tgbot.AdminAccess(ctx, update, whitelistDb)
	if !isAccessAllowed {
		log.Println("Відмова у доступі")
		return nil
	}
	chatID := update.EffectiveChat().GetID()

	msg := update.EffectiveMessage
	text := msg.Text

	if !strings.HasPrefix(text, "/") {
		return nil
	}

	u := strings.Fields(text)
	if len(u) == 0 {
		return nil
	}

	// command := u[0]
	args := u[1:]
	// username := u[1]

	var message string
	for _, username := range args {
		if strings.HasPrefix(username, "@") {
			err := database.DeleteUser(db, username)
			if err != nil {
				return err
			}
			s := fmt.Sprintf("Користувач %s був успішно видалений з БД\n", username)
			message += s
		}
	}

	_, err := ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
		Message: message,
	})
	if err != nil {
		log.Printf("Помилка надсилання повідомлення: %v", err)
		return err
	}

	return nil
}
