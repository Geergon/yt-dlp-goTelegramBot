package tgbot

import (
	"log"
	"math"
	"strings"
	"time"

	"github.com/Geergon/yt-dlp-goTelegramBot/internal/yt"
	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/tg"
)

type platformMatcher struct {
	name  string
	match func(text string) (string, bool)
}

var platformMatchers = []platformMatcher{
	{"YouTube", yt.GetYoutubeURL},
	{"TikTok", yt.GetTikTokURL},
	{"Instagram", yt.GetInstaURL},
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

func Url(update *ext.Update) (string, bool, string) {
	url, platform, ok := parseMediaUrl(update.EffectiveMessage.Text, secondField)
	return url, ok, platform
}

func UrlFromText(text string) (string, bool, string) {
	url, platform, ok := parseMediaUrl(text, firstField)
	return url, ok, platform
}

func deleteMsgTimer(ctx *ext.Context, chatID int64, sentMsgId int) {
	const errorMessageTimeout = 60 * time.Second

	time.AfterFunc(errorMessageTimeout, func() {
		ctx.DeleteMessages(chatID, []int{sentMsgId})
	})
}

func parseMediaUrl(text string, fallback pickFallback) (url, platform string, ok bool) {
	if strings.Contains(text, "/fragment") {
		return "", "", false
	}

	for _, m := range platformMatchers {
		if u, found := m.match(text); found {
			if !yt.IsUrl(u) {
				return "", "", false
			}
			return u, m.name, true
		}
	}

	if u, found := fallback(strings.Fields(text)); found && yt.IsUrl(u) {
		return u, "", true
	}
	return "", "", false
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
