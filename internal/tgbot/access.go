package tgbot

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/Geergon/yt-dlp-goTelegramBot/internal/config"
	"github.com/Geergon/yt-dlp-goTelegramBot/internal/database"
	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/tg"
)

func normalizeChatID(id int64) int64 {
	s := strconv.FormatInt(id, 10)
	if strings.HasPrefix(s, "-100") {
		trimmed := strings.TrimPrefix(s, "-100")
		if v, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
			return v
		}
	}
	return id
}

func Access(ctx *ext.Context, update *ext.Update, whitelistDb *sql.DB) int64 {
	allowedChatId, _ := strconv.ParseInt(os.Getenv("CHAT_ID"), 10, 64)
	chatID := update.EffectiveChat().GetID()
	normalizedChatID := normalizeChatID(chatID)
	user := update.EffectiveUser()

	allowedChats := config.GetIntSlice("allowed_chat")

	isAuthorized := false
	for _, chat := range allowedChats {
		if normalizeChatID(int64(chat)) == normalizedChatID {
			isAuthorized = true
			break
		}
	}
	if normalizeChatID(allowedChatId) == normalizedChatID {
		isAuthorized = true
	} else {

		allowedUsers := config.GetIntSlice("allowed_user")

		for _, allowedUserID := range allowedUsers {
			if int64(allowedUserID) == user.ID {
				isAuthorized = true
				break
			}
		}

		isUserInWhitelist, err := database.IsUserInWhitelist(whitelistDb, user.ID)
		if err != nil {
			log.Println(err)
		}
		if isUserInWhitelist {
			isAuthorized = true
		}
	}
	if !isAuthorized {
		log.Printf("Неавторизований доступ: %s (UserID: %d, ChatID: %d)", user.Username, user.ID, chatID)
		return 0
	}
	return chatID
}

func AdminAccess(ctx *ext.Context, update *ext.Update, whitelistDb *sql.DB) bool {
	isAuthorized := false
	user := update.EffectiveUser()
	chatID := update.EffectiveChat().GetID()

	allowedUsers := config.GetIntSlice("allowed_user")

	for _, allowedUserID := range allowedUsers {
		if int64(allowedUserID) == user.ID {
			isAuthorized = true
			break
		}
	}

	if !isAuthorized {
		log.Printf("Неавторизований доступ до адмінських команд: %s (UserID: %d, ChatID: %d)", user.Username, user.ID, chatID)
		return false
	}
	return true
}

func DeleteFromWhitelist(ctx *ext.Context, update *ext.Update, whitelistDb *sql.DB) error {
	isAccessAllowed := AdminAccess(ctx, update, whitelistDb)
	if !isAccessAllowed {
		log.Println("Відмова у доступі")
		return nil
	}
	chatID := update.EffectiveChat().GetID()

	msg := update.EffectiveMessage
	text := msg.Text

	if !strings.HasPrefix(text, "/") {
		return nil
	}

	u := strings.Fields(text)
	if len(u) == 0 {
		return nil
	}

	// command := u[0]
	args := u[1:]
	// username := u[1]

	var message string
	for _, username := range args {
		if strings.HasPrefix(username, "@") {
			err := database.DeleteUser(whitelistDb, username)
			if err != nil {
				return err
			}
			s := fmt.Sprintf("Користувач %s був успішно видалений з БД\n", username)
			message += s
		}
	}

	_, err := ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
		Message: message,
	})
	if err != nil {
		log.Printf("Помилка надсилання повідомлення: %v", err)
		return err
	}

	return nil
}

func AddIdToWhitelist(ctx *ext.Context, update *ext.Update, whitelistDb *sql.DB) error {
	isAccessAllowed := AdminAccess(ctx, update, whitelistDb)
	if !isAccessAllowed {
		log.Println("Відмова у доступі")
		return nil
	}
	chatID := update.EffectiveChat().GetID()

	msg := update.EffectiveMessage
	text := msg.Text

	if !strings.HasPrefix(text, "/") {
		return nil
	}

	u := strings.Fields(text)
	if len(u) == 0 {
		return nil
	}

	// command := u[0]
	args := u[1:]
	var message string

	for _, a := range args {
		s := strings.Split(a, ":")
		if len(s) == 2 {
			id := s[0]
			username := s[1]
			if _, err := strconv.Atoi(id); err == nil && strings.HasPrefix(username, "@") {
				idInt64, err := strconv.ParseInt(id, 10, 64)
				if err != nil {
					log.Printf("Не вдалося перетворити id з типу string на int64")
					return err
				}
				err = database.InsertIntoWhitelist(whitelistDb, username, idInt64)
				if err != nil {
					log.Printf("Не вдалося вставити значення в БД: %v", err)
					return err
				}
				s := fmt.Sprintf("Користувач %s був успішно доданий в вайтлист\n", username)
				message += s
			}
		}
	}
	_, err := ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
		Message: message,
	})
	if err != nil {
		log.Printf("Помилка надсилання повідомлення: %v", err)
		return err
	}

	return nil
}

func GetWhitelist(ctx *ext.Context, update *ext.Update, whitelistDb *sql.DB) error {
	isAccessAllowed := AdminAccess(ctx, update, whitelistDb)
	if !isAccessAllowed {
		log.Println("Відмова у доступі")
		return nil
	}
	chatID := update.EffectiveChat().GetID()

	msg := update.EffectiveMessage
	text := msg.Text

	if !strings.HasPrefix(text, "/") {
		return nil
	}

	whitelist, err := database.GetAllWhitelist(whitelistDb)
	if err != nil {
		log.Println(err)
		return err
	}
	if len(whitelist) == 0 {
		_, _ = ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
			Message: "Вайтліст пустий",
		})
		return fmt.Errorf("whitelist is empty")
	}

	var message string
	for _, w := range whitelist {
		s := fmt.Sprintf("%s: %d\n", w.Username, w.Id)
		message += s
	}

	_, err = ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
		Message: message,
	})
	if err != nil {
		log.Printf("Помилка надсилання повідомлення: %v", err)
		return err
	}

	return nil
}
