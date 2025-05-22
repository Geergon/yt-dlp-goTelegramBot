package main

import (
	"fmt"
	"log"
	"os"
	"strconv"

	yt "github.com/Geergon/yt-dlp-goTelegramBot/internal/yt"

	"github.com/celestix/gotgproto"
	"github.com/celestix/gotgproto/dispatcher/handlers"
	"github.com/celestix/gotgproto/dispatcher/handlers/filters"
	"github.com/celestix/gotgproto/ext"
	"github.com/celestix/gotgproto/sessionMaker"
	"github.com/glebarez/sqlite"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Помилка при завантаженні .env файлу")
	}
	appId, err := strconv.Atoi(os.Getenv("APP_ID"))
	if err != nil {
		log.Fatal("Помилка при отриманні APP_ID")
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
			Session: sessionMaker.SqlSession(sqlite.Open("echobot")),
		},
	)
	if err != nil {
		log.Fatalln("Помилка при запуску бота:", err)
	}

	dispatcher := client.Dispatcher

	// Функціонал через команду /start (може потім якось дороблю)
	dispatcher.AddHandlerToGroup(handlers.NewMessage(filters.Message.Text, echo), 1)

	fmt.Printf("Бот (@%s) стартував...\n", client.Self.Username)

	client.Idle()
}

func echo(ctx *ext.Context, update *ext.Update) error {
	allowedChatId, err := strconv.Atoi(os.Getenv("CHAT_ID"))
	user := update.EffectiveUser()
	if err != nil {
		log.Fatalln("Не вдалося отримати chatID")
	}
	chat := update.EffectiveChat()
	chatID := update.EffectiveChat().GetID()
	if chat == nil || chatID != int64(allowedChatId) {
		// Неавторизований доступ
		fmt.Printf("Неавторизований доступ: %s \n", user.Username)
		return nil
	}

	msg := update.EffectiveMessage
	text := msg.Text
	urlYT, isYT := yt.GetYoutubeURL(text)
	urlTT, isTT := yt.GetTikTokURL(text)
	urlInsta, isInsta := yt.GetInstaURL(text)
	if err != nil {
		log.Println("Помилки отриманні інформаії про відео")
	}
	var videoName string
	var info *yt.VideoInfo
	var thumbName string

	if isYT {
		infoYT, err := yt.GetVideoInfo(urlYT)
		if err != nil {
			log.Println("Помилки отриманні інформаії про відео")
		}
		if len(urlYT) > 0 || yt.IsUrl(urlYT) {
			yt.DownloadYTVideo(urlYT, infoYT)
			videoName = yt.GetVideoName(urlYT, infoYT)
			thumbName = yt.GetThumb(urlYT, infoYT)
			info = infoYT
		}
	}
	if isTT {
		infoTT, err := yt.GetVideoInfo(urlTT)
		if err != nil {
			log.Println("Помилки отриманні інформаії про відео")
		}
		if len(urlTT) > 0 || yt.IsUrl(urlTT) {
			yt.DownloadTTVideo(urlTT, infoTT)
			videoName = yt.GetVideoName(urlTT, infoTT)
			thumbName = yt.GetThumb(urlTT, infoTT)
			info = infoTT
		}
	}

	if isInsta {
		infoInsta, err := yt.GetVideoInfo(urlInsta)
		if err != nil {
			log.Println("Помилки отриманні інформаії про відео")
		}
		if len(urlInsta) > 0 || yt.IsUrl(urlInsta) {
			yt.DownloadInstaVideo(urlInsta, infoInsta)
			videoName = yt.GetVideoName(urlInsta, infoInsta)
			thumbName = yt.GetThumb(urlInsta, infoInsta)
			info = infoInsta
		}
	}

	file, err := os.Stat(videoName)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("Файл %s не існує", videoName)
			return err
		} else {
			return err
		}
	}
	if file.IsDir() {
		log.Printf("Файл %s це директорія", videoName)
		return err
	}

	f, err := uploader.NewUploader(ctx.Raw).FromPath(ctx, videoName)
	if err != nil {
		log.Println("Помилка при завантаженні відео")
		return err
	}

	thumbFileStat, err := os.Stat(thumbName)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("Файл прев'ю %s не існує", thumbName)
			return err
		}
		return err
	}

	if thumbFileStat.IsDir() {
		log.Println("Прев'ю це директорія")
		return err
	}

	tf, err := uploader.NewUploader(ctx.Raw).FromPath(ctx, thumbName)
	if err != nil {
		log.Println("Помилка завантаження прев’ю:", err)
	}

	media := &tg.InputMediaUploadedDocument{
		File:     f,
		MimeType: "video/mp4",
		Thumb:    tf,
		Attributes: []tg.DocumentAttributeClass{
			&tg.DocumentAttributeVideo{
				SupportsStreaming: true,
			},
			&tg.DocumentAttributeFilename{
				FileName: videoName,
			},
		},
	}

	_, err = ctx.SendMedia(chatID, &tg.MessagesSendMediaRequest{
		Media:   media,
		Message: info.Title,
	})
	if err != nil {
		log.Println("Помилка при надсиланні відео")
		return err
	}
	err = os.Remove(videoName)
	if err != nil {
		log.Printf("Не вдалося видалити файл: %v", err)
	}
	err = os.Remove(thumbName)
	if err != nil {
		log.Printf("Не вдалося видалити прев’ю: %v", err)
	}
	return err
}
