package tgbot

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path"
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
	var gallery []tg.InputSingleMedia
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
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				log.Printf("Файл %s не існує", filePath)
				continue
			}

			uploadedFile, err := uploader.NewUploader(ctx.Raw).FromPath(ctx, filePath)
			if err != nil {
				log.Printf("Помилка завантаження фото в Telegram: %v", err)
				logErr := fmt.Sprintf("Помилка завантаження фото в Telegram: \n%v", err)
				_, editErr := ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
					ID:      sentMsg.GetID(),
					Message: logErr,
				})
				if editErr != nil {
					log.Printf("Помилка редагування повідомлення: %v", editErr)
				}
				return err
			}
			uploadedMedia := &tg.InputMediaUploadedPhoto{
				File: uploadedFile,
			}

			gallery = append(gallery, tg.InputSingleMedia{
				Media:    uploadedMedia,
				RandomID: int64(time.Now().UnixNano()),
			})
		}

	}

	ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
		ID:      sentMsg.GetID(),
		Message: "Надсилання: \n[◼◼◼◼◼◼◼◻]",
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

	if isPhoto {
		if len(gallery) == 0 {
			log.Println("Немає зображень для відправлення")
			ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
				ID:      sentMsg.GetID(),
				Message: "Помилка: немає зображень для відправлення",
			})
			return fmt.Errorf("немає зображень для відправлення")
		}

		err := ctx.DeleteMessages(chatID, []int{sentMsg.GetID()})
		if err != nil {
			log.Printf("Помилка видалення повідомлення (ID: %d, ChatID: %d): %v", msg.ID, chatID, err)
		} else {
			log.Printf("Повідомлення (ID: %d, ChatID: %d) з URL %s видалено", msg.ID, chatID, url)
		}

		peer := &tg.InputPeerChannel{
			ChannelID:  update.EffectiveChat().GetID(),
			AccessHash: update.EffectiveChat().GetAccessHash(),
		}

		var multiMedia []tg.InputSingleMedia
		for i, media := range gallery {
			singleMedia := tg.InputSingleMedia{
				Media:    media.Media,
				RandomID: media.RandomID,
			}
			// Додаємо caption лише до першого зображення
			if i == 0 {
				singleMedia.Message = title
				singleMedia.Entities = entities
			}
			multiMedia = append(multiMedia, singleMedia)
			log.Printf("Prepared InputSingleMedia %d: RandomID=%d", i+1, media.RandomID)
		}

		log.Printf("Sending %d images to chat ID %d as a multi-media group", len(gallery), update.EffectiveChat().GetID())
		ctxWithTimeout, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		_, err = ctx.Raw.MessagesSendMultiMedia(ctxWithTimeout, &tg.MessagesSendMultiMediaRequest{
			Peer:       peer,
			MultiMedia: multiMedia,
		})
		if err != nil {
			log.Printf("Помилка відправлення медіа-групи: %v", err)
			// Якщо помилка MEDIA_INVALID, спробуємо відправити по одному
			if strings.Contains(err.Error(), "MEDIA_INVALID") {
				log.Println("Falling back to sending images one by one")
				for i, media := range gallery {
					log.Printf("Sending image %d/%d with RandomID %d", i+1, len(gallery), media.RandomID)
					_, err = ctx.Raw.MessagesSendMedia(ctx, &tg.MessagesSendMediaRequest{
						Peer:     peer,
						Media:    media.Media,
						Message:  title,
						Entities: entities,
						RandomID: media.RandomID,
					})
					if err != nil {
						log.Printf("Помилка відправлення зображення %d: %v", i+1, err)
						return err
					}
					log.Printf("Image %d/%d sent successfully", i+1, len(gallery))
					if i < len(gallery)-1 {
						time.Sleep(100 * time.Millisecond)
					}
				}
			} else {
				return err
			}
		}
		// for i, media := range gallery {
		// 	log.Printf("Sending image %d/%d", i+1, len(gallery))
		// 	_, err = ctx.Raw.MessagesSendMedia(ctx, &tg.MessagesSendMediaRequest{
		// 		Peer:     peer,
		// 		Media:    media.Media,
		// 		RandomID: media.RandomID,
		// 	})
		// 	if err != nil {
		// 		log.Printf("Помилка відправлення зображення %d: %v", i+1, err)
		// 		return err
		// 	}
		// 	// Додаємо невелику затримку між відправленнями, щоб уникнути обмежень
		// 	time.Sleep(500 * time.Millisecond)
		// }

		// log.Println(gallery)
		// peer := &tg.InputPeerChannel{
		// 	ChannelID:  update.EffectiveChat().GetID(),
		// 	AccessHash: update.EffectiveChat().GetAccessHash(),
		// }
		// _, err = ctx.Raw.MessagesSendMultiMedia(ctx, &tg.MessagesSendMultiMediaRequest{
		// 	Peer:       peer,
		// 	MultiMedia: gallery,
		// })
		// _, err = ctx.SendMultiMedia(chatID, &tg.MessagesSendMultiMediaRequest{
		// 	MultiMedia: multiMedia,
		// })
		// if err != nil {
		// 	log.Printf("Помилка відправлення альбому: %v", err)
		// 	return err
		// }
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
