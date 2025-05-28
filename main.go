package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"

	"github.com/Geergon/yt-dlp-goTelegramBot/internal/tgbot"
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
	viper.SetDefault("duration", "600")
	viper.SetDefault("long_video_download", true)
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

	dispatcher.AddHandlerToGroup(handlers.NewMessage(filters.Message.Text, tgbot.Echo), 1)
	dispatcher.AddHandler(handlers.NewCommand("logs", tgbot.SendLogs))
	dispatcher.AddHandler(handlers.NewCommand("update", tgbot.UpdateYtdlp))
	dispatcher.AddHandler(handlers.NewCommand("fragment", tgbot.Fragment))
	dispatcher.AddHandler(handlers.NewCommand("download", tgbot.Download))
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
	dispatcher.AddHandler(handlers.NewCommand("settings", tgbot.Settings))
	dispatcher.AddHandler(handlers.NewCallbackQuery(filters.CallbackQuery.Prefix("cb_settings_"), tgbot.SettingsCallback))

	fmt.Printf("Бот (@%s) стартував...\n", client.Self.Username)

	client.Idle()
}
