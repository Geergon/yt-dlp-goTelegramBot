package tgbot

import (
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Geergon/yt-dlp-goTelegramBot/internal/config"
	"github.com/Geergon/yt-dlp-goTelegramBot/internal/yt"
	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
)

type platformMatcher struct {
	platform yt.Platform
	match    func(text string) (string, bool)
}

var platformMatchers = []platformMatcher{
	{yt.YouTube, yt.GetYoutubeURL},
	{yt.TikTok, yt.GetTikTokURL},
	{yt.Instagram, yt.GetInstaURL},
}

type pickFallback func(fields []string) (string, bool)

func firstField(f []string) (string, bool) {
	if len(f) == 0 {
		return "", false
	}
	return f[0], true
}

func secondField(f []string) (string, bool) {
	if len(f) != 2 {
		return "", false
	}
	return f[1], true
}

func Url(update *ext.Update) (string, bool, yt.Platform) {
	url, platform, ok := parseMediaUrl(update.EffectiveMessage.Text, secondField)
	return url, ok, platform
}

func UrlFromText(text string) (string, bool, yt.Platform) {
	url, platform, ok := parseMediaUrl(text, firstField)
	return url, ok, platform
}

func parseMediaUrl(text string, fallback pickFallback) (url string, platform yt.Platform, ok bool) {
	if strings.Contains(text, "/fragment") {
		return "", yt.Unknown, false
	}

	for _, m := range platformMatchers {
		if u, found := m.match(text); found {
			if !yt.IsUrl(u) {
				return "", yt.Unknown, false
			}
			return u, m.platform, true
		}
	}

	if u, found := fallback(strings.Fields(text)); found && yt.IsUrl(u) {
		return u, yt.Unknown, true
	}
	return "", yt.Unknown, false
}

func deleteMsgTimer(ctx *ext.Context, chatID int64, sentMsgId int) {
	const errorMessageTimeout = 60 * time.Second

	time.AfterFunc(errorMessageTimeout, func() {
		_ = ctx.DeleteMessages(chatID, []int{sentMsgId})
	})
}

func reportFailure(ctx *ext.Context, chatID int64, sentMsgId int, text string) {
	_, editErr := ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
		ID:      sentMsgId,
		Message: text,
	})
	if editErr != nil {
		log.Printf("Помилка редагування повідомлення: %v", editErr)
	}
	deleteMsgTimer(ctx, chatID, sentMsgId)
}

func checkAudio(platform yt.Platform, isPhoto bool, file, mediaDir string) error {
	if !isPhoto && platform == yt.TikTok && !yt.HasAudioTrack(file) {
		_ = os.Remove(file)
		return errNoAudio
	}

	if mediaDir == "" {
		return nil
	}
	if info, err := os.Stat(mediaDir); err != nil || !info.IsDir() {
		return nil
	}
	mp4Files, err := filepath.Glob(filepath.Join(mediaDir, "*.mp4"))
	if err != nil {
		log.Printf("Помилка пошуку mp4 в %s: %v", mediaDir, err)
		return nil
	}
	if len(mp4Files) > 0 && !yt.HasAudioTrack(mp4Files[0]) {
		_ = os.Remove(mp4Files[0])
		return errNoAudio
	}
	return nil
}

func tryDownload(platform yt.Platform, url string) (yt.DownloadResult, error) {
	downloadResult, err := yt.DownloadMedia(url, platform)
	if err != nil && !downloadResult.IsPhoto {
		if rmErr := os.Remove(downloadResult.FilePath + ".part"); rmErr != nil && !os.IsNotExist(rmErr) {
			log.Printf("Не вдалося видалити частковий файл: %v", rmErr)
		}
		return downloadResult, err
	}
	return downloadResult, checkAudio(platform, downloadResult.IsPhoto, downloadResult.FilePath, downloadResult.MediaDir)
}

func shouldSkipYouTube(req URLRequest) bool {
	if req.Platform != yt.YouTube {
		return false
	}
	longVideoDownload := config.GetBool("long_video_download")
	duration := config.GetString("duration")

	limit, err := strconv.Atoi(duration)
	if err != nil {
		log.Printf("Помилка парсингу duration: %v", err)
		return false
	}

	info, err := yt.GetVideoInfo(req.URL, req.Platform)
	if err != nil {
		log.Printf("Не вдалося отримати інформацію про відео: %v", err)
		return false
	}

	tooLong := !longVideoDownload && info.Duration >= limit
	isStream := info.IsLive || info.WasLive
	if tooLong || isStream {
		log.Printf("Пропускаємо: тривалість %d с, стрім: %t", info.Duration, isStream)
	}
	return tooLong || isStream
}

func isCommandMessage(text string) bool {
	return strings.HasPrefix(strings.TrimSpace(text), "/")
}

func setProgress(req URLRequest, chatID int64, msgID int, text string) {
	if _, err := req.Context.EditMessage(chatID, &tg.MessagesEditMessageRequest{ID: msgID, Message: text}); err != nil {
		log.Printf("Не вдалося оновити прогрес: %v", err)
	}
}

func newUploader(ctx *ext.Context) *uploader.Uploader {
	return uploader.NewUploader(ctx.Raw).
		WithPartSize(512 * 1024).
		WithThreads(4)
}
