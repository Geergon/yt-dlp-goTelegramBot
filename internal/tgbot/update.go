package tgbot

import (
	"fmt"

	"github.com/Geergon/yt-dlp-goTelegramBot/internal/yt"
	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/tg"
)

func UpdateYtdlp(ctx *ext.Context, update *ext.Update) error {
	chatID := update.EffectiveChat().GetID()

	ytdlp := yt.UpdateYtdlp()
	gallerydl := yt.UpdateGallerydl()
	msg := fmt.Sprintf("%s\n\n%s", ytdlp, gallerydl)
	ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
		Message: msg,
	})

	return nil
}
