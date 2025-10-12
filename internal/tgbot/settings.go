package tgbot

import (
	"log"

	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/tg"
	"github.com/spf13/viper"
)

func Settings(ctx *ext.Context, update *ext.Update) error {
	chatID := update.EffectiveChat().GetID()

	viperMutex.RLock()
	rows := []tg.KeyboardButtonRow{
		{
			Buttons: []tg.KeyboardButtonClass{
				&tg.KeyboardButtonCallback{
					Text: "Автозавантаження відео: " + boolToEmoji(viper.GetBool("auto_download")),
					Data: []byte("cb_settings_auto_download"),
				},
			},
		},
		{
			Buttons: []tg.KeyboardButtonClass{
				&tg.KeyboardButtonCallback{
					Text: "Видалення посилань: " + boolToEmoji(viper.GetBool("delete_url")),
					Data: []byte("cb_settings_delete_links"),
				},
			},
		},
		{
			Buttons: []tg.KeyboardButtonClass{
				&tg.KeyboardButtonCallback{
					Text: "Завантаження довгих відео: " + boolToEmoji(viper.GetBool("long_video_download")),
					Data: []byte("cb_settings_long_video_download"),
				},
			},
		},
	}
	viperMutex.RUnlock()

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

	viperMutex.Lock()
	autoDownload := viper.GetBool("auto_download")
	deleteUrl := viper.GetBool("delete_url")
	longVideoDownload := viper.GetBool("long_video_download")
	switch string(data) {
	case "cb_settings_auto_download":
		viper.Set("auto_download", !autoDownload)
	case "cb_settings_delete_links":
		viper.Set("delete_url", !deleteUrl)
	case "cb_settings_long_video_download":
		viper.Set("long_video_download", !longVideoDownload)
	default:
		log.Printf("Невідомий callback: %s", data)
		viperMutex.Unlock()
		return nil
	}
	if err := viper.WriteConfig(); err != nil {
		log.Printf("Помилка збереження конфігурації: %v", err)
		viperMutex.Unlock()
		return err
	}
	viperMutex.Unlock()

	viperMutex.RLock()
	rows := []tg.KeyboardButtonRow{
		{
			Buttons: []tg.KeyboardButtonClass{
				&tg.KeyboardButtonCallback{
					Text: "Автозавантаження відео: " + boolToEmoji(viper.GetBool("auto_download")),
					Data: []byte("cb_settings_auto_download"),
				},
			},
		},
		{
			Buttons: []tg.KeyboardButtonClass{
				&tg.KeyboardButtonCallback{
					Text: "Видалення посилань: " + boolToEmoji(viper.GetBool("delete_url")),
					Data: []byte("cb_settings_delete_links"),
				},
			},
		},
		{
			Buttons: []tg.KeyboardButtonClass{
				&tg.KeyboardButtonCallback{
					Text: "Завантаження довгих відео: " + boolToEmoji(viper.GetBool("long_video_download")),
					Data: []byte("cb_settings_long_video_download"),
				},
			},
		},
	}
	viperMutex.RUnlock()

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
