package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/Geergon/yt-dlp-goTelegramBot/internal/config"
	"github.com/Geergon/yt-dlp-goTelegramBot/internal/database"
	"github.com/glebarez/sqlite"
	"github.com/gotd/contrib/middleware/floodwait"
	"github.com/gotd/td/telegram"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/celestix/gotgproto"
	"github.com/celestix/gotgproto/dispatcher/handlers"
	"github.com/celestix/gotgproto/dispatcher/handlers/filters"
	"github.com/celestix/gotgproto/sessionMaker"
)

func init() {
	log.SetOutput(&lumberjack.Logger{
		Filename:   "bot.log",
		MaxSize:    10, // МБ
		MaxBackups: 3,
		MaxAge:     28, // дні
		Compress:   true,
	})
}

var (
	whitelistDb *sql.DB
	cacheDb     *sql.DB
)

func main() {
	config.SetConfig()

	go startCleanupRoutine()

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

	config.Watch()

	whitelistDb, err = database.InitDB("./db/whitelist.db")
	if err != nil {
		log.Fatal(err)
	}
	defer whitelistDb.Close()

	cacheDb, err = database.InitCacheDB("./db/cache.db")
	if err != nil {
		log.Fatal(err)
	}
	defer cacheDb.Close()

	go startCacheCleanup(cacheDb)

	waiter := floodwait.NewWaiter().WithMaxRetries(5).WithMaxWait(time.Minute).WithCallback(func(ctx context.Context, wait floodwait.FloodWait) {
		log.Printf("FLOOD_WAIT: чекаємо %v", wait.Duration)
	})

	client, err := gotgproto.NewClient(
		// Get AppID from https://my.telegram.org/apps
		appId,
		// Get ApiHash from https://my.telegram.org/apps
		apiHash,
		// ClientType, as we defined above
		gotgproto.ClientTypeBot(botToken),
		// Optional parameters of client
		&gotgproto.ClientOpts{
			Session:     sessionMaker.SqlSession(sqlite.Open("./db/session")),
			Middlewares: []telegram.Middleware{waiter},
			RunMiddleware: func(
				origRun func(ctx context.Context, f func(ctx context.Context) error) error,
				ctx context.Context,
				f func(ctx context.Context) error,
			) error {
				return origRun(ctx, func(ctx context.Context) error {
					return waiter.Run(ctx, f)
				})
			},
		},
	)
	if err != nil {
		log.Fatalln("Помилка при запуску бота:", err)
	}

	go StartWorkers(client, 2)
	dispatcher := client.Dispatcher

	dispatcher.AddHandler(handlers.NewCommand("gif", Gif))
	dispatcher.AddHandlerToGroup(handlers.NewMessage(filters.Message.Text, OnText), 1)
	dispatcher.AddHandler(handlers.NewCommand("logs", Logs))
	dispatcher.AddHandler(handlers.NewCommand("update", Update))
	dispatcher.AddHandler(handlers.NewCommand("fragment", Fragment))
	dispatcher.AddHandler(handlers.NewCommand("audio", Audio))
	dispatcher.AddHandler(handlers.NewCommand("download", Download))
	dispatcher.AddHandler(handlers.NewCommand("spoiler", Spoiler))
	dispatcher.AddHandler(handlers.NewCommand("start", Start))
	dispatcher.AddHandler(handlers.NewCommand("admin_help", AdminHelp))
	dispatcher.AddHandler(handlers.NewCommand("help", Help))
	dispatcher.AddHandler(handlers.NewCommand("add_to_whitelist", AddToWhitelist))
	dispatcher.AddHandler(handlers.NewCommand("check_whitelist", CheckWhitelist))
	dispatcher.AddHandler(handlers.NewCommand("delete_from_whitelist", DeleteFromWhitelist))
	dispatcher.AddHandler(handlers.NewCommand("settings", Settings))
	dispatcher.AddHandler(handlers.NewCallbackQuery(filters.CallbackQuery.Prefix("cb_settings_"), CbSettings))

	fmt.Printf("Бот (@%s) стартував...\n", client.Self.Username)

	client.Idle()
}
