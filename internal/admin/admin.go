package admin

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sqerison/tgwatchspam/internal/config"
)

// Admin handles authorization checks. [FEAT-005]
// Two levels: Superadmin (SUPERADMIN_ID in .env) and group Admin (Telegram getChatMember).
type Admin struct {
	bot *tgbotapi.BotAPI
	cfg *config.Config
}

func New(bot *tgbotapi.BotAPI, cfg *config.Config) *Admin {
	return &Admin{bot: bot, cfg: cfg}
}

func (a *Admin) IsSuperAdmin(userID int64) bool {
	return userID == a.cfg.SuperAdminID
}

// IsAdmin returns true if the user is a group creator, administrator, or the superadmin.
// Admin status is verified live via Telegram API on every command call.
func (a *Admin) IsAdmin(chatID, userID int64) (bool, error) {
	if a.IsSuperAdmin(userID) {
		return true, nil
	}
	member, err := a.bot.GetChatMember(tgbotapi.GetChatMemberConfig{
		ChatConfigWithUser: tgbotapi.ChatConfigWithUser{
			ChatID: chatID,
			UserID: userID,
		},
	})
	if err != nil {
		return false, err
	}
	return member.Status == "creator" || member.Status == "administrator", nil
}
