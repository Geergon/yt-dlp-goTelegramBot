package tgbot

import (
	"database/sql"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/Geergon/yt-dlp-goTelegramBot/internal/database"
	"github.com/celestix/gotgproto/ext"
	"github.com/spf13/viper"
)

var viperMutex sync.RWMutex

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

	viperMutex.RLock()
	allowedChats := viper.GetIntSlice("allowed_chat")
	viperMutex.RUnlock()

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
		viperMutex.RLock()
		allowedUsers := viper.GetIntSlice("allowed_user")
		viperMutex.RUnlock()
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

	viperMutex.RLock()
	allowedUsers := viper.GetIntSlice("allowed_user")
	viperMutex.RUnlock()

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
