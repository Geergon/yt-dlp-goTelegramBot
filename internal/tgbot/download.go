package tgbot

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"image/jpeg"
	"io"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Geergon/yt-dlp-goTelegramBot/internal/database"
	"github.com/Geergon/yt-dlp-goTelegramBot/internal/yt"
	"github.com/celestix/gotgproto/ext"
	"github.com/celestix/gotgproto/types"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"github.com/spf13/viper"
)

type URLRequest struct {
	URL      string
	Platform yt.Platform
	Command  string
	Fragment string
	Context  *ext.Context
	Update   *ext.Update
	Spoiler  bool
}

var errNoAudio = errors.New("no audio track")

func init() {
	err := os.MkdirAll("video", 0755)
	if err != nil {
		log.Fatalf("Помилка створення папки video: %v", err)
	}
	err = os.MkdirAll("photo", 0755)
	if err != nil {
		log.Fatalf("Помилка створення папки photo: %v", err)
	}
	err = os.MkdirAll("audio", 0755)
	if err != nil {
		log.Fatalf("Помилка створення папки audio: %v", err)
	}
	err = os.MkdirAll("cache", 0755)
	if err != nil {
		log.Fatalf("Помилка створення папки cache: %v", err)
	}

	botToken := os.Getenv("BOT_TOKEN")
	if botToken == "" {
		log.Fatal("BOT_TOKEN не задано")
	}
}

func extractDocumentFromMessage(msg *types.Message) *tg.Document {
	if msg == nil {
		return nil
	}
	m, ok := msg.Message.Media.(*tg.MessageMediaDocument)
	if !ok {
		return nil
	}
	doc, ok := m.Document.(*tg.Document)
	if !ok {
		return nil
	}
	return doc
}

func saveToCache(db *sql.DB, url string, c database.CachedMedia) {
	cacheDir := "/cache"

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		log.Printf("Помилка створення папки кешу: %v", err)
		return
	}

	ext := filepath.Ext(c.FilePath)
	hash := fmt.Sprintf("%x", md5.Sum([]byte(url)))
	cachedPath := filepath.Join(cacheDir, hash+ext)

	if err := copyFile(c.FilePath, cachedPath); err != nil {
		log.Printf("Помилка копіювання в кеш: %v", err)
		return
	}

	c.FilePath = cachedPath
	if err := database.SetCachedFile(db, url, c); err != nil {
		log.Printf("Помилка збереження в БД кешу: %v", err)
	}
	log.Printf("Збережено в кеш: %s -> %s", url, cachedPath)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func ProcessURL(cacheDb *sql.DB, req URLRequest) error {
	// Створюємо контекст із таймаутом у 10 хвилин
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	log.Printf("URLRequest: %s, %s", req.URL, req.Command)
	// Виконуємо обробку в окремій горутині, щоб перевіряти таймаут
	errChan := make(chan error, 1)
	go func() {
		log.Printf("Починаємо обробку URL %s (команда: %s)", req.URL, req.Command)
		errChan <- processURLWithContext(cacheDb, req)
	}()

	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		// Якщо минув таймаут, надсилаємо повідомлення про помилку
		chatID := req.Update.EffectiveChat().GetID()
		if chatID != 0 {
			_, err := req.Context.SendMessage(chatID, &tg.MessagesSendMessageRequest{
				Message: fmt.Sprintf("Обробка URL %s (команда: %s) перервана через таймаут (10 хвилин)", req.URL, req.Command),
			})
			if err != nil {
				log.Printf("Помилка надсилання повідомлення про таймаут: %v", err)
			}
		}
		log.Printf("Таймаут обробки URL %s (команда: %s) після 10 хвилин", req.URL, req.Command)
		return ctx.Err()
	}
}

func processURLWithContext(cacheDb *sql.DB, req URLRequest) error {
	chatID := req.Update.EffectiveChat().GetID()

	switch req.Command {
	case "auto":
		return processAutoDownload(cacheDb, req, chatID)
	case "download":
		return processDownload(cacheDb, req, chatID)
	case "audio":
		return processAudio(req, chatID)
	case "fragment":
		return processFragment(req, chatID)
	default:
		log.Println("Невідома команда")
		return fmt.Errorf("невідома команда: %s", req.Command)
	}
}

func processAutoDownload(cacheDb *sql.DB, req URLRequest, chatID int64) error {
	viperMutex.RLock()
	enabled := viper.GetBool("auto_download")
	viperMutex.RUnlock()

	if !enabled || isCommandMessage(req.Update.EffectiveMessage.Text) {
		return nil
	}
	if shouldSkipYouTube(req) {
		return nil
	}
	return downloadAndSend(cacheDb, req, chatID)
}

func processDownload(cacheDb *sql.DB, req URLRequest, chatID int64) error {
	return downloadAndSend(cacheDb, req, chatID)
}

func downloadAndSend(cacheDb *sql.DB, req URLRequest, chatID int64) error {
	sentMsg, err := req.Context.SendMessage(chatID, &tg.MessagesSendMessageRequest{
		Message: "Завантаження медіа: \n[◼◼◼◼◻◻◻◻]",
	})
	if err != nil {
		var rpcErr *tgerr.Error
		if errors.As(err, &rpcErr) && rpcErr.Code == 403 {
			log.Printf("Немає прав писати в чат %d, пропускаємо", chatID)
			return nil
		}
		log.Printf("Помилка надсилання початкового повідомлення: %v", err)
		return err
	}
	sentMsgId := sentMsg.GetID()

	if cached, ok := database.GetCachedFile(cacheDb, req.URL); ok {
		ok, err := sendFromCache(cacheDb, req, chatID, sentMsg, cached)
		if ok && err == nil {
			return nil
		} else {
			log.Printf("помилка при спробі надсилання з кешу: %v", err)
			return err
		}
	}

	downloadResult, downloadErr := downloadMedia(req.Context, chatID, req.URL, req.Platform, sentMsgId)
	if downloadErr != nil {
		log.Printf("Помилка при завантаженні: %v", downloadErr)
		reportFailure(req.Context, chatID, sentMsgId, fmt.Sprintf("Помилка завантаження: %v", downloadErr))
		deleteMedia(req.Context, req.Update, req.URL, chatID, downloadResult.MediaDir, "", true)
		return downloadErr
	}

	_, err = req.Context.EditMessage(chatID, &tg.MessagesEditMessageRequest{
		ID:      sentMsgId,
		Message: "Перевірка і формування медіа перед відправкою: \n[◼◼◼◼◼◼◻◻]",
	})
	if err != nil {
		log.Printf("Помилка редагування повідомлення: %v", err)
		return err
	}

	images, media, thumbName, musicPath, errCheck := mediaCheck(req.Context, chatID, sentMsgId, req.URL, req.Platform, downloadResult.IsPhoto, downloadResult.FilePath, req.Spoiler, downloadResult.MediaDir)
	if errCheck != nil {
		log.Printf("Помилка при обробці медіа: %v", errCheck)
		reportFailure(req.Context, chatID, sentMsgId, fmt.Sprintf("Помилка обробки медіа: %v", errCheck))
		deleteMedia(req.Context, req.Update, req.URL, chatID, downloadResult.MediaDir, thumbName, true)
		return errCheck
	}

	_, err = req.Context.EditMessage(chatID, &tg.MessagesEditMessageRequest{
		ID:      sentMsgId,
		Message: "Надсилання: \n[◼◼◼◼◼◼◼◻]",
	})
	if err != nil {
		log.Printf("Помилка редагування повідомлення: %v", err)
		return err
	}

	doc, err := sendMedia(req.Context, req.Update, req.URL, downloadResult.IsPhoto, false, images, musicPath, media, chatID, sentMsgId)
	if err != nil {
		log.Printf("Помилка при надсиланні повідомлення: %v", err)
		reportFailure(req.Context, chatID, sentMsgId, fmt.Sprintf("Помилка надсилання: %v", err))
		deleteMedia(req.Context, req.Update, req.URL, chatID, downloadResult.MediaDir, thumbName, true)
		return err
	}

	if doc != nil && !downloadResult.IsPhoto {
		saveToCache(cacheDb, req.URL, database.CachedMedia{
			FilePath:      downloadResult.FilePath,
			DocID:         doc.ID,
			AccessHash:    doc.AccessHash,
			FileReference: doc.FileReference,
		})
	}

	deleteMedia(req.Context, req.Update, req.URL, chatID, downloadResult.MediaDir, thumbName, false)
	return nil
}

func processFragment(req URLRequest, chatID int64) error {
	sentMsg, err := req.Context.SendMessage(chatID, &tg.MessagesSendMessageRequest{
		Message: "Завантаження відео і вирізання фрагменту: \n[◼◼◼◼◻◻◻◻]",
	})
	if err != nil {
		log.Printf("Помилка надсилання початкового повідомлення: %v", err)
		return err
	}
	sentMsgId := sentMsg.GetID()

	if req.Fragment == "" {
		reportFailure(req.Context, chatID, sentMsgId, "Помилка: не вказано фрагмент. Використання: /fragment <YouTube_URL> <00:00-00:00>")
		return err
	}

	viperMutex.RLock()
	filter := viper.GetString("yt-dlp_filter")
	viperMutex.RUnlock()

	timeUnix := time.Now().UnixMilli()
	outputFile := fmt.Sprintf("./video/outputFrag%d.mp4", timeUnix)
	cmd := exec.Command(
		"yt-dlp",
		"--download-sections", fmt.Sprintf("*%s", req.Fragment),
		"-f", filter,
		"-o", outputFile,
		req.URL,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("yt-dlp error: %v\nOutput: %s", err, string(output))
		reportFailure(req.Context, chatID, sentMsgId, fmt.Sprintf("Помилка завантаження фрагменту: %v", err))
		return err
	}

	log.Printf("Завантаження фрагменту %s завершено успішно", req.URL)

	_, err = req.Context.EditMessage(chatID, &tg.MessagesEditMessageRequest{
		ID:      sentMsgId,
		Message: "Перевірка і формування медіа перед відправкою: \n[◼◼◼◼◼◼◻◻]",
	})
	if err != nil {
		log.Printf("Помилка редагування повідомлення: %v", err)
		return err
	}

	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		reportFailure(req.Context, chatID, sentMsgId, "Не вдалося завантажити фрагмент")
		return err
	}

	videoFile, err := uploader.NewUploader(req.Context.Raw).FromPath(req.Context, outputFile)
	if err != nil {
		log.Printf("Помилка завантаження відео в Telegram: %v", err)
		reportFailure(req.Context, chatID, sentMsgId, fmt.Sprintf("Помилка завантаження фрагменту в Telegram: %v", err))
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
				FileName: path.Base(outputFile),
			},
		},
	}

	if thumbName = yt.GetThumb(req.URL, yt.YouTube); thumbName != "" {
		if thumbFileStat, err := os.Stat(thumbName); err == nil && !thumbFileStat.IsDir() {
			if thumbFile, err := uploader.NewUploader(req.Context.Raw).FromPath(req.Context, thumbName); err == nil {
				media.(*tg.InputMediaUploadedDocument).Thumb = thumbFile
			} else {
				log.Printf("Помилка завантаження прев’ю %s: %v", thumbName, err)
			}
		} else {
			log.Printf("Прев’ю недоступне або є помилкою: %s", thumbName)
		}
	}

	_, err = req.Context.EditMessage(chatID, &tg.MessagesEditMessageRequest{
		ID:      sentMsgId,
		Message: "Надсилання: \n[◼◼◼◼◼◼◼◻]",
	})
	if err != nil {
		log.Printf("Помилка редагування повідомлення: %v", err)
		return err
	}

	user := req.Update.EffectiveUser()
	username := "@" + user.Username
	title := username + " (link)"
	entities := []tg.MessageEntityClass{
		&tg.MessageEntityTextURL{
			Offset: len(username) + 1,
			Length: 6,
			URL:    req.URL,
		},
	}

	_, err = req.Context.EditMessage(chatID, &tg.MessagesEditMessageRequest{
		ID:       sentMsgId,
		Message:  title,
		Media:    media,
		Entities: entities,
	})
	if err != nil {
		log.Printf("Помилка відправлення фрагменту: %v", err)
		reportFailure(req.Context, chatID, sentMsgId, fmt.Sprintf("Помилка надсилання фрагменту: %v", err))
		deleteMedia(req.Context, req.Update, req.URL, chatID, outputFile, thumbName, true)
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

	return nil
}

func processAudio(req URLRequest, chatID int64) error {
	sentMsg, err := req.Context.SendMessage(chatID, &tg.MessagesSendMessageRequest{
		Message: "Завантаження аудіо: \n[◼◼◼◼◻◻◻◻]",
	})
	if err != nil {
		log.Printf("Помилка надсилання початкового повідомлення: %v", err)
		return err
	}
	sentMsgId := sentMsg.GetID()

	const maxAttempts = 3
	const retryDelay = 5 * time.Second

	var audioName string
	var audioDir string
	var audioPath string
	var downloadErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		audio, musicDir, err := yt.DownloadAudio(req.URL, req.Platform)
		if err != nil {
			log.Printf("attempt №%d (%s) to downloading audio failed: %v", attempt, req.Platform, err)
			downloadErr = err
			if attempt < maxAttempts {
				log.Printf("wait %v seconds before next try...", retryDelay)
				time.Sleep(retryDelay)
			}
			if musicDir != "" {
				os.RemoveAll(musicDir)
			}
			continue
		}

		if len(audio) == 0 {
			log.Printf("audio file after downloading not found: %s (attempt %d)", req.URL, attempt)
			downloadErr = fmt.Errorf("audio file not found")
			if attempt < maxAttempts {
				log.Printf("wait %v seconds before next try...", retryDelay)
				time.Sleep(retryDelay)
			}
			continue
		}

		audioName = audio[0]
		audioDir = musicDir
		audioPath = path.Join(musicDir, audioName)
		log.Printf("audio successfully downloaded on attempt №%d: %s", attempt, audioName)
		downloadErr = nil
		break
	}

	if downloadErr != nil || audioName == "" {
		log.Printf("cannot download audio after %d attempts for URL: %s, last error: %v", maxAttempts, req.URL, downloadErr)
		errMsg := fmt.Sprintf("cannot download audio after %d attempts: %v", maxAttempts, downloadErr)
		reportFailure(req.Context, chatID, sentMsgId, errMsg)
		return fmt.Errorf("download error: %w", downloadErr)
	}

	_, err = req.Context.EditMessage(chatID, &tg.MessagesEditMessageRequest{
		ID:      sentMsgId,
		Message: "Перевірка і формування аудіо перед відправкою: \n[◼◼◼◼◼◼◻◻]",
	})
	if err != nil {
		log.Printf(": %v", err)
		return err
	}

	fileData, err := uploader.NewUploader(req.Context.Raw).FromPath(req.Context, audioPath)
	if err != nil {
		logErr := fmt.Sprintf("Error loading audio in Telegram: %v", err)
		log.Print(logErr)
		reportFailure(req.Context, chatID, sentMsgId, logErr)
		return err
	}

	media := &tg.InputMediaUploadedDocument{
		File:     fileData,
		MimeType: "audio/mpeg",
		Attributes: []tg.DocumentAttributeClass{
			&tg.DocumentAttributeAudio{
				Title: path.Base(audioName),
			},
			&tg.DocumentAttributeFilename{
				FileName: audioName,
			},
		},
	}

	thumbName := yt.GetThumb(req.URL, req.Platform)
	if thumbName != "" {
		if thumbFileStat, err := os.Stat(thumbName); err == nil && !thumbFileStat.IsDir() {
			if thumbFile, err := uploader.NewUploader(req.Context.Raw).FromPath(req.Context, thumbName); err == nil {
				media.Thumb = thumbFile
			} else {
				log.Printf("Помилка завантаження прев’ю %s: %v", thumbName, err)
			}
		} else {
			log.Printf("Прев’ю недоступне або є помилкою: %s", thumbName)
		}
	}

	_, err = req.Context.EditMessage(chatID, &tg.MessagesEditMessageRequest{
		ID:      sentMsgId,
		Message: "Надсилання: \n[◼◼◼◼◼◼◼◻]",
	})
	if err != nil {
		log.Printf("Помилка редагування повідомлення: %v", err)
		return err
	}

	_, err = sendMedia(req.Context, req.Update, req.URL, false, true, nil, "", media, chatID, sentMsgId)
	if err != nil {
		log.Printf("Помилка при надсиланні аудіо: %v", err)
		if err := os.RemoveAll(audioDir); err != nil {
			log.Printf("Помилка видалення тимчасового каталогу %s: %v", audioDir, err)
		} else {
			log.Printf("Тимчасовий каталог %s успішно видалено.", audioDir)
		}

		reportFailure(req.Context, chatID, sentMsgId, fmt.Sprintf("Помилка надсилання аудіо: %v", err))
		deleteMedia(req.Context, req.Update, req.URL, chatID, "", thumbName, true)
		return err
	}

	if err := os.RemoveAll(audioDir); err != nil {
		log.Printf("Помилка видалення тимчасового каталогу %s: %v", audioDir, err)
	} else {
		log.Printf("Тимчасовий каталог %s успішно видалено.", audioDir)
	}
	deleteMedia(req.Context, req.Update, req.URL, chatID, audioDir, thumbName, false)
	return nil
}

func downloadMedia(ctx *ext.Context, chatID int64, url string, platform yt.Platform, sentMsgId int) (yt.DownloadResult, error) {
	const maxAttempts = 3
	const retryDelay = 10 * time.Second

	var downloadErr error
	var downloadResult yt.DownloadResult

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		downloadResult, downloadErr = tryDownload(platform, url)
		if downloadErr == nil {
			return downloadResult, nil
		}

		log.Printf("Спроба %d завантаження (%s) не вдалося: %v", attempt, platform, downloadErr)

		if attempt < maxAttempts {
			time.Sleep(retryDelay)
		}
	}

	errMsg := fmt.Sprintf("Не вдалося завантажити медіа після %d спроб (%s): %v", maxAttempts, platform, downloadErr)
	log.Print(errMsg)
	if errors.Is(downloadErr, errNoAudio) {
		errMsg = fmt.Sprintf("Відео без аудіо після %d спроб", maxAttempts)
	}
	reportFailure(ctx, chatID, sentMsgId, errMsg)
	return downloadResult, downloadErr
}

func mediaCheck(ctx *ext.Context, chatID int64, sentMsgId int, url string, platform yt.Platform, isPhoto bool, mediaFilePath string, spoiler bool, galleryDir string) ([]string, tg.InputMediaClass, string, string, error) {
	var thumbName string
	var media tg.InputMediaClass
	var isExist bool
	var isVideo bool
	var hasMusic bool
	var musicPath string
	var images []string

	if !isPhoto {
		file, err := os.Stat(mediaFilePath)
		if err != nil {
			logMsg := "Помилка перевірки файлу відео"
			if os.IsNotExist(err) {
				logMsg = "Файл не існує: " + mediaFilePath
			}
			log.Println(logMsg)
			reportFailure(ctx, chatID, sentMsgId, "Помилка: не вдалося завантажити відео: "+logMsg)
			return nil, nil, "", "", err
		}

		if file.IsDir() {
			log.Printf("Файл %s є директорією", mediaFilePath)
			reportFailure(ctx, chatID, sentMsgId, "Помилка: завантажений файл є директорією.")
			return nil, nil, "", "", fmt.Errorf("Файл %s є директорією", mediaFilePath)
		}

		fileData, err := uploader.NewUploader(ctx.Raw).FromPath(ctx, mediaFilePath)
		if err != nil {
			log.Printf("Помилка завантаження відео в Telegram: %v", err)
			logErr := fmt.Sprintf("Помилка завантаження відео в Telegram: \n%v", err)
			reportFailure(ctx, chatID, sentMsgId, logErr)
			return nil, nil, "", "", err
		}

		media = &tg.InputMediaUploadedDocument{
			File:     fileData,
			MimeType: "video/mp4",
			Spoiler:  spoiler,
			Attributes: []tg.DocumentAttributeClass{
				&tg.DocumentAttributeVideo{
					SupportsStreaming: true,
				},
				&tg.DocumentAttributeFilename{
					FileName: filepath.Base(mediaFilePath),
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
		images, isExist, isVideo, hasMusic, musicPath = yt.GetPhotoPathList(galleryDir)
		if !isExist {
			log.Println("Помилка при завантаженні фотографій. Їх не існує")
			return nil, nil, "", "", fmt.Errorf("Помилка при завантаженні фотографій")
		}
		if isVideo {
			if len(images) == 0 {
				return nil, nil, "", "", fmt.Errorf("відео файл не знайдено в папці photo")
			}
			_, err := os.Stat(images[0])
			mediaFilePath = images[0]
			if err != nil {
				logMsg := "Помилка перевірки файлу відео"
				if os.IsNotExist(err) {
					logMsg = "Файл не існує: " + mediaFilePath
				}
				log.Println(logMsg)
				reportFailure(ctx, chatID, sentMsgId, "Помилка: не вдалося завантажити відео: "+logMsg)
				return nil, nil, "", "", err
			}

			fileData, err := uploader.NewUploader(ctx.Raw).FromPath(ctx, mediaFilePath)
			if err != nil {
				log.Printf("Помилка завантаження відео в Telegram: %v", err)
				logErr := fmt.Sprintf("Помилка завантаження відео в Telegram: \n%v", err)
				reportFailure(ctx, chatID, sentMsgId, logErr)
				return nil, nil, "", "", err
			}

			media = &tg.InputMediaUploadedDocument{
				File:     fileData,
				MimeType: "video/mp4",
				Spoiler:  spoiler,
				Attributes: []tg.DocumentAttributeClass{
					&tg.DocumentAttributeVideo{
						SupportsStreaming: true,
					},
					&tg.DocumentAttributeFilename{
						FileName: filepath.Base(mediaFilePath),
					},
				},
			}
		}
		if !isVideo {
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
			if hasMusic && musicPath != "" {
				return images, media, thumbName, musicPath, nil
			}
		}
	}
	return images, media, thumbName, "", nil
}

func sendMedia(ctx *ext.Context, update *ext.Update, url string, isPhoto bool, isAudio bool, images []string, musicPath string, media tg.InputMediaClass, chatID int64, sentMsgId int) (*tg.Document, error) {
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

	if user.Username == "" && user.FirstName != "" {
		username = user.FirstName
	}

	imagesIsVideo := false
	if len(images) != 0 {
		path := images[0]
		fileExtension := filepath.Ext(path)
		if fileExtension == ".mp4" {
			imagesIsVideo = true
		} else {
			imagesIsVideo = false
		}
	}

	if isPhoto && !imagesIsVideo {

		var multiMedia []tg.InputSingleMedia
		peerStorage := ctx.PeerStorage
		inputPeer := peerStorage.GetInputPeerById(chatID)
		log.Printf("Peer тип: %T, значення: %+v", inputPeer, inputPeer)

		for i, filePath := range images {

			var uploaded tg.MessageMediaClass
			var err error

			if i > 0 {
				time.Sleep(500 * time.Millisecond)
			}

			stat, err := os.Stat(filePath)
			if err != nil {
				log.Printf("Файл не існує: %s: %v", filePath, err)
				continue
			}
			log.Printf("Надсилаємо фото %d: %s, розмір: %d байт", i, filePath, stat.Size())

			fileBytes, err := os.ReadFile(filePath)
			if err != nil {
				log.Printf("Помилка читання файлу %s: %v", filePath, err)
				return nil, err
			}

			fileData, err := uploader.NewUploader(ctx.Raw).
				WithThreads(1).
				FromBytes(ctx, filepath.Base(filePath), fileBytes)
			if err != nil {
				log.Printf("Помилка завантаження фото %s: %v", filePath, err)
				return nil, err
			}

			for attempt := 0; attempt < 3; attempt++ {
				uploaded, err = ctx.Raw.MessagesUploadMedia(ctx, &tg.MessagesUploadMediaRequest{
					Peer:  inputPeer,
					Media: &tg.InputMediaUploadedPhoto{File: fileData},
				})
				if err == nil {
					break
				}

				var rpcErr *tgerr.Error
				if errors.As(err, &rpcErr) && rpcErr.Type == "FLOOD_WAIT" {
					waitSec := rpcErr.Argument + 1
					log.Printf("FLOOD_WAIT %d секунд, чекаємо...", waitSec)
					time.Sleep(time.Duration(waitSec) * time.Second)
					continue
				}

				log.Printf("Помилка MessagesUploadMedia для %s: %v", filePath, err)
				return nil, err
			}
			if err != nil {
				log.Printf("Не вдалось завантажити фото після 3 спроб: %s", filePath)
				return nil, err
			}
			log.Printf("MessagesUploadMedia результат: %T", uploaded)

			msgMedia, ok := uploaded.(*tg.MessageMediaPhoto)
			if !ok {
				log.Printf("Неочікуваний тип після upload: %T", uploaded)
				return nil, fmt.Errorf("неочікуваний тип медіа після завантаження")
			}
			photo, ok := msgMedia.Photo.(*tg.Photo)
			if !ok {
				return nil, fmt.Errorf("не вдалось отримати Photo")
			}

			var message string
			var entities []tg.MessageEntityClass
			if i == 0 || i == 10 {
				message = fmt.Sprintf("%s (link)", username)
				entities = []tg.MessageEntityClass{
					&tg.MessageEntityTextURL{
						Offset: len(username) + 1,
						Length: 6,
						URL:    url,
					},
				}
			}

			var randomID int64
			binary.Read(rand.Reader, binary.LittleEndian, &randomID)

			multiMedia = append(multiMedia, tg.InputSingleMedia{
				RandomID: randomID,
				Media: &tg.InputMediaPhoto{
					ID: &tg.InputPhoto{
						ID:            photo.ID,
						AccessHash:    photo.AccessHash,
						FileReference: photo.FileReference,
					},
				},
				Message:  message,
				Entities: entities,
			})
		}

		const maxAlbumSize = 10
		for i := 0; i < len(multiMedia); i += maxAlbumSize {
			end := i + maxAlbumSize
			if end > len(multiMedia) {
				end = len(multiMedia)
			}
			chunk := multiMedia[i:end]

			var sendErr error
			for attempt := 0; attempt < 3; attempt++ {
				_, sendErr = ctx.SendMultiMedia(chatID, &tg.MessagesSendMultiMediaRequest{
					Silent:     false,
					ClearDraft: false,
					MultiMedia: chunk,
				})
				if sendErr == nil {
					break
				}

				var rpcErr *tgerr.Error
				if errors.As(sendErr, &rpcErr) && rpcErr.Type == "FLOOD_WAIT" {
					waitSec := rpcErr.Argument + 1
					log.Printf("FLOOD_WAIT %d секунд перед надсиланням альбому, чекаємо...", waitSec)
					time.Sleep(time.Duration(waitSec) * time.Second)
					continue
				}

				log.Printf("Помилка відправлення медіа-групи: %v", sendErr)
				return nil, sendErr
			}
			if sendErr != nil {
				log.Printf("Не вдалось надіслати альбом після 3 спроб: %v", sendErr)
				return nil, sendErr
			}

			log.Printf("Надіслано альбом %d-%d з %d фото", i+1, end, len(multiMedia))

			if end < len(multiMedia) {
				time.Sleep(500 * time.Millisecond)
			}
		}
		log.Printf("Всі %d зображень успішно відправлено", len(images))
		err := ctx.DeleteMessages(chatID, []int{sentMsgId})
		if err != nil {
			log.Println("Помилка видалення повідомлення")
		} else {
			log.Printf("Повідомлення (ID: %d, ChatID: %d) з URL %s видалено", sentMsgId, chatID, url)
		}

		if musicPath != "" {
			fileData, err := uploader.NewUploader(ctx.Raw).FromPath(ctx, musicPath)
			if err != nil {
				log.Printf("Помилка завантаження аудіо в Telegram: %v", err)
				logErr := fmt.Sprintf("Помилка завантаження аудіо в Telegram: %v", err)
				reportFailure(ctx, chatID, sentMsgId, logErr)
				// return nil, editErr
			}

			name := filepath.Base(musicPath)
			media := &tg.InputMediaUploadedDocument{
				File:     fileData,
				MimeType: "audio/mpeg",
				Attributes: []tg.DocumentAttributeClass{
					&tg.DocumentAttributeAudio{
						Title: strings.TrimSuffix(name, filepath.Ext(name)),
					},
					&tg.DocumentAttributeFilename{
						FileName: musicPath,
					},
				},
			}

			_, err = ctx.SendMedia(chatID, &tg.MessagesSendMediaRequest{
				Media: media,
			})
			if err != nil {
				log.Printf("Помилка надсилання повідомлення з аудіо: %v", err)
				return nil, err
			}
		}

	} else if isAudio {

		ctx.DeleteMessages(chatID, []int{sentMsgId})
		_, err := ctx.SendMedia(chatID, &tg.MessagesSendMediaRequest{
			Media: media,
		})
		if err != nil {
			log.Printf("Помилка редагування повідомлення з аудіо: %v", err)
			return nil, err
		}
	} else {
		result, err := ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
			ID:       sentMsgId,
			Message:  title,
			Media:    media,
			Entities: entities,
		})
		if err != nil {
			log.Printf("Помилка редагування повідомлення з відео: %v", err)
			return nil, err
		}
		doc := extractDocumentFromMessage(result)
		return doc, nil
	}
	return nil, nil
}

func deleteMedia(ctx *ext.Context, update *ext.Update, url string, chatID int64, mediaDir string, thumbName string, fail bool) {
	msg := update.EffectiveMessage
	text := msg.Text

	viperMutex.RLock()
	deleteURL := viper.GetBool("delete_url")
	viperMutex.RUnlock()
	if deleteURL && !fail {
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

	if mediaDir != "" {
		if err := os.RemoveAll(mediaDir); err != nil {
			log.Printf("Не вдалося видалити медіа: %v", err)
		}
	}
	if thumbName != "" {
		if err := os.RemoveAll(filepath.Dir(thumbName)); err != nil {
			log.Printf("Не вдалося видалити прев’ю: %v", err)
		}
	}
}
