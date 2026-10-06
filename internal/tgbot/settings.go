package tgbot

import (
	"log"

	"github.com/Geergon/yt-dlp-goTelegramBot/internal/config"
	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/tg"
)

func Settings(ctx *ext.Context, update *ext.Update) error {
	chatID := update.EffectiveChat().GetID()

	rows := []tg.KeyboardButtonRow{
		{
			Buttons: []tg.KeyboardButtonClass{
				&tg.KeyboardButtonCallback{
					Text: "Автозавантаження відео: " + boolToEmoji(config.GetBool("auto_download")),
					Data: []byte("cb_settings_auto_download"),
				},
			},
		},
		{
			Buttons: []tg.KeyboardButtonClass{
				&tg.KeyboardButtonCallback{
					Text: "Видалення посилань: " + boolToEmoji(config.GetBool("delete_url")),
					Data: []byte("cb_settings_delete_links"),
				},
			},
		},
		{
			Buttons: []tg.KeyboardButtonClass{
				&tg.KeyboardButtonCallback{
					Text: "Завантаження довгих відео: " + boolToEmoji(config.GetBool("long_video_download")),
					Data: []byte("cb_settings_long_video_download"),
				},
			},
		},
	}

	_, _ = ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
		Message: "⚙️ Налаштування бота:\nВиберіть опцію для увімкнення/вимкнення.",
		ReplyMarkup: &tg.ReplyInlineMarkup{
			Rows: rows,
		},
	})
	return nil
}

func boolToEmoji(b bool) string {
	if b {
		return "✅"
	}
	return "❌"
}

func SettingsCallback(ctx *ext.Context, u *ext.Update) error {
	chatID := u.EffectiveChat().GetID()

	callback := u.CallbackQuery
	data := callback.Data
	messageID := callback.MsgID

	switch string(data) {
	case "cb_settings_auto_download":
		config.ToggleBool("auto_download")
	case "cb_settings_delete_links":
		config.ToggleBool("delete_url")
	case "cb_settings_long_video_download":
		config.ToggleBool("long_video_download")
	default:
		log.Printf("Невідомий callback: %s", data)
		return nil
	}

	rows := []tg.KeyboardButtonRow{
		{
			Buttons: []tg.KeyboardButtonClass{
				&tg.KeyboardButtonCallback{
					Text: "Автозавантаження відео: " + boolToEmoji(config.GetBool("auto_download")),
					Data: []byte("cb_settings_auto_download"),
				},
			},
		},
		{
			Buttons: []tg.KeyboardButtonClass{
				&tg.KeyboardButtonCallback{
					Text: "Видалення посилань: " + boolToEmoji(config.GetBool("delete_url")),
					Data: []byte("cb_settings_delete_links"),
				},
			},
		},
		{
			Buttons: []tg.KeyboardButtonClass{
				&tg.KeyboardButtonCallback{
					Text: "Завантаження довгих відео: " + boolToEmoji(config.GetBool("long_video_download")),
					Data: []byte("cb_settings_long_video_download"),
				},
			},
		},
	}

	_, _ = ctx.EditMessage(chatID, &tg.MessagesEditMessageRequest{
		ID:      messageID,
		Message: "⚙️ Налаштування бота:",
		ReplyMarkup: &tg.ReplyInlineMarkup{
			Rows: rows,
		},
	})

	_, _ = ctx.AnswerCallback(&tg.MessagesSetBotCallbackAnswerRequest{
		QueryID: callback.QueryID,
	})
	return nil
}
