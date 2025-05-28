package tgbot

import (
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

var bot *tgbotapi.BotAPI

func init() {
	botToken := os.Getenv("BOT_TOKEN")
	if botToken == "" {
		log.Fatal("BOT_TOKEN не задано")
	}

	var err error
	bot, err = tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Fatalf("Помилка ініціалізації бота: %v", err)
	}
	bot.Debug = true
}

func Echo(ctx *ext.Context, update *ext.Update) error {
	viperMutex.RLock()
	autoDownload := viper.GetBool("auto_download")
	viperMutex.RUnlock()
	if autoDownload {
		msg := update.EffectiveMessage
		text := msg.Text
		if strings.Contains(text, "/download") {
			return nil
		}
		Download(ctx, update)
	}
	return nil
}

func Download(ctx *ext.Context, update *ext.Update) error {
	chatID := Access(ctx, update)
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

	ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
		ID:      sentMsg.GetID(),
		Message: "Завантаження медіа: \n[◼◼◼◼◻◻◻◻]",
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
		downloadFunc = yt.DownloadTTVideo
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

	var thumbName string
	var media tg.InputMediaClass
	var isExist bool
	var images []string
	if !isPhoto {
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
			return nil
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

	ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
		ID:      sentMsg.GetID(),
		Message: "Надсилання: \n[◼◼◼◼◼◼◼◻]",
	})
	user := update.EffectiveUser()
	username := "@" + user.Username
	title := fmt.Sprintf("%s (\\[%link\\](\\(%s\\)))", username, url) // Екрануємо ( і )
	entities := []tg.MessageEntityClass{
		&tg.MessageEntityTextURL{
			Offset: len(username) + 1,
			Length: 6,
			URL:    url,
		},
	}

	if isPhoto {
		err := ctx.DeleteMessages(chatID, []int{sentMsg.GetID()})
		if err != nil {
			log.Printf("Помилка видалення повідомлення (ID: %d, ChatID: %d): %v", msg.ID, chatID, err)
		} else {
			log.Printf("Повідомлення (ID: %d, ChatID: %d) з URL %s видалено", msg.ID, chatID, url)
		}

		botChatID := chatID
		idAsString := strconv.FormatInt(botChatID, 10)
		prefixedIDString := "-100" + idAsString
		newTelegramID, err := strconv.ParseInt(prefixedIDString, 10, 64)

		log.Printf("Sending %d images to chat ID %d as a media group using telegram-bot-api", len(images), botChatID)
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
		log.Printf("Media group sent successfully with %d images", len(images))
	} else {
		_, err = ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
			ID:       sentMsg.GetID(),
			Message:  title,
			Media:    media,
			Entities: entities,
		})
		if err != nil {
			log.Printf("Помилка редагування повідомлення з відео: %v", err)
			return err
		}
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

	if isPhoto {
		dir := "./photo"
		photo := os.DirFS(dir)
		jpgFiles, err := fs.Glob(photo, "*.jpg")
		if err != nil {
			fmt.Println("error")
		}
		for _, m := range jpgFiles {
			path := path.Join(dir, m)
			os.Remove(path)
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
	return nil
}

func Fragment(ctx *ext.Context, u *ext.Update) error {
	chatID := Access(ctx, u)
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

	if thumbName = yt.GetThumb(url, "YouTube"); thumbName != "" {
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

	user := u.EffectiveUser()
	username := "@" + user.Username
	title := username + " (link)"
	entities := []tg.MessageEntityClass{
		&tg.MessageEntityTextURL{
			Offset: len(username) + 1,
			Length: 6,
			URL:    url,
		},
	}

	_, err = ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
		ID:       sentMsg.GetID(),
		Message:  title,
		Media:    media,
		Entities: entities,
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
