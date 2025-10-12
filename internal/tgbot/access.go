package tgbot

import (
	"database/sql"
	"log"
	"os"
	"strconv"
	"sync"

	"github.com/Geergon/yt-dlp-goTelegramBot/internal/database"
	"github.com/celestix/gotgproto/ext"
	"github.com/spf13/viper"
)

var viperMutex sync.RWMutex

func Access(ctx *ext.Context, update *ext.Update, whitelistDb *sql.DB) int64 {
	allowedChatId, _ := strconv.Atoi(os.Getenv("CHAT_ID"))
	chatID := update.EffectiveChat().GetID()
	user := update.EffectiveUser()

	viperMutex.RLock()
	allowedChats := viper.GetIntSlice("allowed_chat")
	viperMutex.RUnlock()

	isAuthorized := false
	for _, chat := range allowedChats {
		if int64(chat) == chatID {
			isAuthorized = true
			break
		}
	}
	if chatID == int64(allowedChatId) {
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
		log.Printf("Неавторизований доступ до адмінських команд: %s (UserID: %d, ChatID: %d)", user.Username, user.ID)
		return false
	}
	return true
}
