package bot

import (
	"log"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sqerison/tgwatchspam/internal/admin"
	"github.com/sqerison/tgwatchspam/internal/config"
	"github.com/sqerison/tgwatchspam/internal/filter"
	"github.com/sqerison/tgwatchspam/internal/storage"
)

func registerCommands(bot *tgbotapi.BotAPI) {
	commands := []tgbotapi.BotCommand{
		{Command: "tgwatch_add", Description: "Add blocked word or regex: /tgwatch_add word <text> | /tgwatch_add regex <pattern>"},
		{Command: "tgwatch_remove", Description: "Remove word or regex: /tgwatch_remove word <text> | /tgwatch_remove regex <pattern>"},
		{Command: "tgwatch_list", Description: "List blocked words or patterns: /tgwatch_list words | /tgwatch_list regex"},
		{Command: "tgwatch_clear", Description: "Clear all words or patterns: /tgwatch_clear words | /tgwatch_clear regex"},
		{Command: "tgwatch_set", Description: "Change settings: action, mute_duration, verification, sandbox, name_filter"},
		{Command: "tgwatch_show", Description: "Show current settings: /tgwatch_show settings"},
		{Command: "tgwatch_check", Description: "Test a message against filters without taking action"},
		{Command: "tgwatch_log", Description: "Show spam log: /tgwatch_log [n] | /tgwatch_log clear"},
		{Command: "tgwatch_unrestrict", Description: "Lift sandbox/mute — reply to the user's message"},
		{Command: "tgwatch_clean", Description: "Delete all bot replies and admin commands from this chat"},
		{Command: "tgwatch_copy", Description: "Superadmin: copy settings between chats (use in DM)"},
		{Command: "tgwatch_lang", Description: "Choose bot language for this chat (🇬🇧 English / 🇺🇦 Українська)"},
		{Command: "tgwatch_help", Description: "Show all available commands"},
	}
	cfg := tgbotapi.SetMyCommandsConfig{Commands: commands}
	if _, err := bot.Request(cfg); err != nil {
		log.Printf("failed to register commands with BotFather: %v", err)
	} else {
		log.Printf("commands registered with BotFather")
	}
}

func Run(cfg *config.Config, st *storage.Storage) {
	bot, err := tgbotapi.NewBotAPI(cfg.BotToken)
	if err != nil {
		log.Fatalf("failed to create bot: %v", err)
	}
	bot.Debug = cfg.LogLevel == "debug"
	log.Printf("authorized on account @%s", bot.Self.UserName)

	registerCommands(bot)

	adm := admin.New(bot, cfg)
	flt := filter.New(st)
	h := newHandler(bot, st, flt, adm, cfg)

	// Background goroutine: lift expired sandbox/mute restrictions every minute [FEAT-007]
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			h.liftExpiredRestrictions()
			h.kickExpiredVerifications() // [FEAT-015]
		}
	}()

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	// chat_member is excluded by default — must be requested explicitly to catch joins via invite link [FEAT-015]
	u.AllowedUpdates = []string{"message", "callback_query", "chat_member"}
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		go h.handle(update)
	}
}
