package tgbot

import (
	"fmt"
	"log"

	"github.com/Geergon/yt-dlp-goTelegramBot/internal/yt"
	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/tg"
)

func UpdateYtdlp(ctx *ext.Context, update *ext.Update) error {
	chatID := Access(ctx, update)
	if chatID == 0 {
		log.Println("Відмова у доступі")
		return nil
	}

	ytdlp := yt.UpdateYtdlp()
	gallerydl := yt.UpdateGallerydl()
	msg := fmt.Sprintf("%s\n\n%s", ytdlp, gallerydl)
	ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
		Message: msg,
	})

	return nil
}
