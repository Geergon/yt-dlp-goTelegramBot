package tgbot

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Geergon/yt-dlp-goTelegramBot/internal/yt"
	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
	"github.com/spf13/viper"
)

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

	// info, err := yt.GetVideoInfo(url)
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
	}

	ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
		ID:      sentMsg.GetID(),
		Message: "Надсилання відео: \n[◼◼◼◼◼◼◼◻]",
	})
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
	// var title string
	// info, err := yt.GetVideoInfo(url)
	// if err != nil {
	// 	title = "Не вдалося отримати назву відео"
	// }
	// title = info.Title

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
