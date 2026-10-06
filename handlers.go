package main

import (
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Geergon/yt-dlp-goTelegramBot/internal/tgbot"
	"github.com/Geergon/yt-dlp-goTelegramBot/internal/yt"
	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
)

const startText = "Ласкаво просимо! Надішліть URL з YouTube, TikTok, Instagram для завантаження відео і фото.\n\n" + helpText

const helpText = `
Команди:
/fragment - завантажити фрагмент відео. Приклад: /fragment https://www.youtube.com/watch?v=XYZ 05:00-07:00
/download - ручне завантаження відео, дозволяє завантажувати довгі відео з ютуба, а також фото і відео з практичного будь-якого сайту (x.com, reddit і інші), якщо це підтримує yt-dlp і gallery-dl. Приклад: /download https://example.com/...
/audio - завантажити аудіо. Приклад: /audio https://www.youtube.com/watch?v=XYZ
/gif - зробити gif з відео, можна додати текст до гіфки (текст обов'язково має бути в лапках). Щоб зробити гіфку, треба вибрати завантажити "Фото або відео" і в коментарі прописати команду: /gif textbott="нижній текст" texttop="верхній текст" (якщо текст не треба, просто написати /gif і все
/spoiler - робить те саме, що і команда /download, тільки кидає відео під спойлером
`

const adminHelpText = `
Список команд доступних для адмінів.
Команди:
/add_to_whitelist - Додати користувача у вайтлист (підтримує декілька аргументів), приклад: /add_to_whitelist id:@username id:@username ...
/check_whitelist - Переглянути вайтлист
/delete_from_whitelist - Видалити одного або декількох користувачів в вайтлиста, приклад: /delete_from_whitelist @username @username ...
/settings - Налаштувати бота
/logs - Отримати логи бота
/update - Оновити бота
`

func OnText(ctx *ext.Context, update *ext.Update) error {
	chatID := tgbot.Access(ctx, update, whitelistDb)
	if chatID == 0 {
		log.Println("Відмова у доступі")
		return nil
	}

	url, isValid, platform := tgbot.Url(update)

	if update.EffectiveMessage.EditDate != 0 {
		return nil
	}

	if !isValid {
		return nil
	}

	if strings.Contains(url, "list=") {
		url = yt.RemoveYouTubeListParam(url)
		log.Printf("Видалено параметр list, новий URL: %s", url)
	}

	// Перевіряємо, чи URL уже обробляється
	_, loaded := processingURLs.LoadOrStore(url, struct{}{})
	if loaded {
		log.Printf("URL %s уже обробляється, пропускаємо", url)
		return nil
	}

	// Додаємо URL до черги
	urlQueue <- tgbot.URLRequest{URL: url, Platform: platform, Command: "auto", Context: ctx, Update: update, Spoiler: false}
	return nil
}

func Logs(ctx *ext.Context, update *ext.Update) error {
	isAccessAllowed := tgbot.AdminAccess(ctx, update, whitelistDb)
	if !isAccessAllowed {
		log.Println("Відмова у доступі")
		return nil
	}
	tgbot.SendLogs(ctx, update)
	return nil
}

func Update(ctx *ext.Context, update *ext.Update) error {
	chatID := tgbot.Access(ctx, update, whitelistDb)
	if chatID == 0 {
		log.Println("Відмова у доступі")
		return nil
	}
	tgbot.UpdateYtdlp(ctx, update)
	return nil

}

func Gif(ctx *ext.Context, update *ext.Update) error {
	chatID := tgbot.Access(ctx, update, whitelistDb)
	if chatID == 0 {
		log.Println("Відмова у доступі")
		return nil
	}
	msg := update.EffectiveMessage
	text := msg.Text

	dir, filename, err := tgbot.GetVideo(ctx, update)
	if err != nil {
		log.Println(err)
		return err
	}

	if !strings.HasPrefix(text, "/") {
		return nil
	}

	u := strings.Fields(text)
	if len(u) == 0 {
		return nil
	}

	var textBott string
	var textTop string

	re := regexp.MustCompile(`(\w+)=(?:"([^"]*)"|(\S+))`)
	matches := re.FindAllStringSubmatch(text, -1)

	for _, match := range matches {
		if match[1] == "textbott" {
			if match[2] != "" {
				textBott = match[2]
			}
		}
		if match[1] == "texttop" {
			if match[2] != "" {
				textTop = match[2]
			}
		}
		// fmt.Printf("\nСпівпадіння %d:\n", i+1)
		// fmt.Printf("  [0] Повне: '%s'\n", match[0])
		// fmt.Printf("  [1] Ключ:  '%s'\n", match[1])
		// fmt.Printf("  [2] Значення в лапках: '%s'\n", match[2])
		// fmt.Printf("  [3] Значення без лапок: '%s'\n", match[3])
	}

	gifFilename, err := tgbot.MakeGif(dir, filename, textBott, textTop)
	if err != nil {
		log.Println("не вдалося створити гіфку: ", err)
		return err
	}

	gifPath := filepath.Join(dir, gifFilename)

	f, err := uploader.NewUploader(ctx.Raw).FromPath(ctx, gifPath)
	if err != nil {
		return err
	}

	_, err = ctx.SendMedia(chatID, &tg.MessagesSendMediaRequest{
		// Message: "This is your caption",
		Media: &tg.InputMediaUploadedDocument{
			File:     f,
			MimeType: "video/mp4",
			Attributes: []tg.DocumentAttributeClass{
				&tg.DocumentAttributeAnimated{},
			},
		},
	})
	if err != nil {
		os.RemoveAll(dir)
		return err
	}
	os.RemoveAll(dir)
	return nil
}

func Audio(ctx *ext.Context, update *ext.Update) error {
	chatID := tgbot.Access(ctx, update, whitelistDb)
	if chatID == 0 {
		log.Println("Відмова у доступі")
		return nil
	}

	url, isValid, platform := tgbot.Url(update)
	if !isValid {
		_, err := ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
			Message: "Невалідне URL або платформа не відповідає",
		})
		return err
	}

	if strings.Contains(url, "list=") {
		url = yt.RemoveYouTubeListParam(url)
		log.Printf("Видалено параметр list, новий URL: %s", url)
	}

	_, loaded := processingURLs.LoadOrStore(url, struct{}{})
	if loaded {
		log.Printf("URL %s уже обробляється, пропускаємо", url)
		return nil
	}

	urlQueue <- tgbot.URLRequest{
		URL:      url,
		Platform: platform,
		Command:  "audio",
		Context:  ctx,
		Update:   update,
		Spoiler:  false,
	}
	return nil
}

func Fragment(ctx *ext.Context, update *ext.Update) error {
	chatID := tgbot.Access(ctx, update, whitelistDb)
	if chatID == 0 {
		log.Println("Відмова у доступі")
		return nil
	}

	args := strings.Fields(update.EffectiveMessage.Text)
	if len(args) != 3 {
		_, err := ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
			Message: "Використання: /fragment <YouTube_URL> <00:00-00:00>\nПриклад: /fragment https://www.youtube.com/watch?v=XYZ 05:00-07:00",
		})
		return err
	}

	url := args[1]
	fragment := args[2]

	if strings.Contains(url, "help") {
		_, err := ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
			Message: "Використання: /fragment <YouTube_URL> <00:00-00:00>\nПриклад: /fragment https://www.youtube.com/watch?v=XYZ 05:00-07:00",
		})
		return err
	}

	if strings.Contains(url, "list=") {
		url = yt.RemoveYouTubeListParam(url)
		log.Printf("Видалено параметр list, новий URL: %s", url)
	}

	_, loaded := processingURLs.LoadOrStore(url, struct{}{})
	if loaded {
		log.Printf("URL %s уже обробляється, пропускаємо", url)
		return nil
	}

	urlQueue <- tgbot.URLRequest{
		URL:      url,
		Command:  "fragment",
		Fragment: fragment,
		Context:  ctx,
		Update:   update,
		Spoiler:  false,
	}
	return nil
}

func Download(ctx *ext.Context, update *ext.Update) error {
	err := enqueue(ctx, update, false)
	if err != nil {
		return err
	}
	return nil
}

func Spoiler(ctx *ext.Context, update *ext.Update) error {
	err := enqueue(ctx, update, true)
	if err != nil {
		return err
	}
	return nil
}

func enqueue(ctx *ext.Context, update *ext.Update, spoiler bool) error {
	chatID := tgbot.Access(ctx, update, whitelistDb)
	if chatID == 0 {
		log.Println("Відмова у доступі")
		return nil
	}

	var url string
	var platform yt.Platform
	var isValid bool

	if update.EffectiveMessage.ReplyTo != nil {
		log.Println("Команда є відповіддю")

		replyHeader, ok := update.EffectiveMessage.ReplyTo.(*tg.MessageReplyHeader)
		if !ok {
			log.Println("Не вдалось отримати ReplyHeader")
			return nil
		}
		replyToMsgID := replyHeader.ReplyToMsgID
		log.Printf("ReplyToMsgID: %d", replyToMsgID)

		var replyText string

		inputPeer := ctx.PeerStorage.GetInputPeerById(chatID)
		switch peer := inputPeer.(type) {
		case *tg.InputPeerChannel:
			// Супергрупа або канал
			msgs, err := ctx.Raw.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
				Channel: &tg.InputChannel{
					ChannelID:  peer.ChannelID,
					AccessHash: peer.AccessHash,
				},
				ID: []tg.InputMessageClass{
					&tg.InputMessageID{ID: replyToMsgID},
				},
			})
			if err != nil {
				log.Printf("Помилка отримання повідомлення з каналу: %v", err)
				return err
			}
			channelMsgs, ok := msgs.(*tg.MessagesChannelMessages)
			if !ok || len(channelMsgs.Messages) == 0 {
				log.Println("Повідомлення не знайдено")
				return nil
			}
			msg, ok := channelMsgs.Messages[0].(*tg.Message)
			if !ok {
				return nil
			}
			replyText = msg.Message

		case *tg.InputPeerUser, *tg.InputPeerChat:
			// Особистий чат або звичайна група
			msgs, err := ctx.Raw.MessagesGetMessages(ctx, []tg.InputMessageClass{
				&tg.InputMessageID{ID: replyToMsgID},
			})
			if err != nil {
				log.Printf("Помилка отримання повідомлення: %v", err)
				return err
			}
			msgsObj, ok := msgs.(*tg.MessagesMessages)
			if !ok || len(msgsObj.Messages) == 0 {
				log.Println("Повідомлення не знайдено")
				return nil
			}
			msg, ok := msgsObj.Messages[0].(*tg.Message)
			if !ok {
				return nil
			}
			replyText = msg.Message

		default:
			log.Printf("Невідомий тип peer: %T", inputPeer)
			return nil
		}

		log.Printf("ReplyToMessage text: %s", replyText)
		if replyText == "" {
			log.Println("ReplyToMessage не містить тексту")
			return nil
		}

		url, isValid, platform = tgbot.UrlFromText(replyText)
	} else {
		log.Println("Команда не є відповіддю")
		url, isValid, platform = tgbot.Url(update)
		if !isValid {
			log.Println("Невалідне URL або платформа не підтримується")
			_, err := ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
				Message: "Некоректний URL або платформа не підтримується",
			})
			return err
		}
	}

	if !yt.IsUrl(url) {
		log.Println("Повідомлення не містить url")
		return nil
	}

	if strings.Contains(url, "list=") {
		url = yt.RemoveYouTubeListParam(url)
		log.Printf("Видалено параметр list, новий URL: %s", url)
	}

	_, loaded := processingURLs.LoadOrStore(url, struct{}{})
	if loaded {
		log.Printf("URL %s уже обробляється, пропускаємо", url)
		return nil
	}

	if spoiler {
		urlQueue <- tgbot.URLRequest{
			URL:      url,
			Platform: platform,
			Command:  "download",
			Context:  ctx,
			Update:   update,
			Spoiler:  true,
		}
	} else {
		urlQueue <- tgbot.URLRequest{
			URL:      url,
			Platform: platform,
			Command:  "download",
			Context:  ctx,
			Update:   update,
			Spoiler:  false,
		}
	}
	log.Printf("Додано до черги URL: %s, Platform: %s, Command: download", url, platform)
	return nil
}

func Start(ctx *ext.Context, update *ext.Update) error {
	chatID := update.EffectiveChat().GetID()
	_, err := ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
		Message: startText,
	})
	if err != nil {
		log.Printf("Помилка надсилання повідомлення: %v", err)
		return err
	}
	return nil
}

func AdminHelp(ctx *ext.Context, update *ext.Update) error {
	isAccessAllowed := tgbot.AdminAccess(ctx, update, whitelistDb)
	if !isAccessAllowed {
		log.Println("Відмова у доступі")
		return nil
	}
	chatID := update.EffectiveChat().GetID()

	_, err := ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
		Message: adminHelpText,
	})
	if err != nil {
		log.Printf("Помилка надсилання повідомлення: %v", err)
		return err
	}
	return nil
}

func Help(ctx *ext.Context, update *ext.Update) error {
	chatID := update.EffectiveChat().GetID()

	_, err := ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
		Message: helpText,
	})
	if err != nil {
		log.Printf("Помилка надсилання повідомлення: %v", err)
		return err
	}
	return nil
}
func AddToWhitelist(ctx *ext.Context, update *ext.Update) error {
	err := tgbot.AddIdToWhitelist(ctx, update, whitelistDb)
	if err != nil {
		log.Println(err)
		return err
	}
	return nil
}

func CheckWhitelist(ctx *ext.Context, update *ext.Update) error {
	err := tgbot.GetWhitelist(ctx, update, whitelistDb)
	if err != nil {
		log.Println(err)
		return err
	}
	return nil
}

func DeleteFromWhitelist(ctx *ext.Context, update *ext.Update) error {
	err := tgbot.DeleteFromWhitelist(ctx, update, whitelistDb)
	if err != nil {
		log.Println(err)
		return err
	}
	return nil
}

func Settings(ctx *ext.Context, update *ext.Update) error {
	isAccessAllowed := tgbot.AdminAccess(ctx, update, whitelistDb)
	if !isAccessAllowed {
		log.Println("Відмова у доступі")
		return nil
	}

	chatID := tgbot.Access(ctx, update, whitelistDb)
	if chatID == 0 {
		log.Println("Відмова у доступі")
		return nil
	}

	tgbot.Settings(ctx, update)
	return nil
}

func CbSettings(ctx *ext.Context, update *ext.Update) error {
	chatID := tgbot.Access(ctx, update, whitelistDb)
	if chatID == 0 {
		log.Println("Відмова у доступі")
		return nil
	}

	tgbot.SettingsCallback(ctx, update)
	return nil
}
