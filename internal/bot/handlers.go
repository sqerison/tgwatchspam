package bot

import (
	"fmt"
	"html"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sqerison/tgwatchspam/internal/admin"
	"github.com/sqerison/tgwatchspam/internal/config"
	"github.com/sqerison/tgwatchspam/internal/filter"
	"github.com/sqerison/tgwatchspam/internal/i18n"
	"github.com/sqerison/tgwatchspam/internal/storage"
)

type handler struct {
	bot      *tgbotapi.BotAPI
	storage  *storage.Storage
	filter   *filter.Filter
	admin    *admin.Admin
	cfg      *config.Config
	confirms confirmStore
}

// confirmStore holds pending /clear and /copy confirmations (in-memory, 60s TTL).
type confirmStore struct {
	mu sync.Mutex
	m  map[string]confirmEntry
}

type confirmEntry struct {
	action  string
	expires time.Time
}

func (c *confirmStore) set(chatID, userID int64, action string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = make(map[string]confirmEntry)
	}
	c.m[fmt.Sprintf("%d:%d", chatID, userID)] = confirmEntry{
		action:  action,
		expires: time.Now().Add(60 * time.Second),
	}
}

func (c *confirmStore) pop(chatID, userID int64) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := fmt.Sprintf("%d:%d", chatID, userID)
	e, ok := c.m[key]
	if !ok || time.Now().After(e.expires) {
		delete(c.m, key)
		return "", false
	}
	delete(c.m, key)
	return e.action, true
}

func newHandler(bot *tgbotapi.BotAPI, st *storage.Storage, flt *filter.Filter, adm *admin.Admin, cfg *config.Config) *handler {
	return &handler{bot: bot, storage: st, filter: flt, admin: adm, cfg: cfg}
}

// lang returns the configured language for a chat.
func (h *handler) lang(chatID int64) string {
	return h.storage.GetLanguage(chatID)
}

// t returns the translation for key in the chat's language.
func (h *handler) t(chatID int64, key string) string {
	return i18n.T(h.lang(chatID), key)
}

// tf returns a formatted translation for key in the chat's language.
func (h *handler) tf(chatID int64, key string, args ...interface{}) string {
	return i18n.Tf(h.lang(chatID), key, args...)
}

func (h *handler) send(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	sent, err := h.bot.Send(msg)
	if err != nil {
		log.Printf("send error chatID=%d: %v", chatID, err)
		return
	}
	// Track bot messages so /clean can delete them [FEAT-013]
	if err := h.storage.TrackMessage(chatID, sent.MessageID); err != nil {
		log.Printf("track message: %v", err)
	}
}

func (h *handler) sendHTML(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	sent, err := h.bot.Send(msg)
	if err != nil {
		log.Printf("sendHTML error chatID=%d: %v", chatID, err)
		return
	}
	if err := h.storage.TrackMessage(chatID, sent.MessageID); err != nil {
		log.Printf("track message: %v", err)
	}
}

// actionLabel returns a human-readable description of the action taken. [FEAT-014]
func actionLabel(lang, action string, muteDurationH int) string {
	switch action {
	case "delete":
		return i18n.T(lang, "action_delete")
	case "mute":
		return i18n.Tf(lang, "action_mute", muteDurationH)
	case "kick":
		return i18n.T(lang, "action_kick")
	case "ban":
		return i18n.T(lang, "action_ban")
	default:
		return action
	}
}

// handle is the top-level dispatcher for incoming updates.
func (h *handler) handle(update tgbotapi.Update) {
	// Inline button presses (verification + lang + confirm) [FEAT-015, FEAT-016]
	if update.CallbackQuery != nil {
		h.handleCallbackQuery(update.CallbackQuery)
		return
	}

	// New member join via ChatMemberUpdated — covers invite-link joins in supergroups [FEAT-015]
	if update.ChatMember != nil {
		cm := update.ChatMember
		if (cm.OldChatMember.HasLeft() || cm.OldChatMember.WasKicked()) && cm.NewChatMember.Status == "member" {
			h.handleNewMember(cm.Chat.ID, cm.NewChatMember.User)
		}
		return
	}

	if update.Message == nil {
		return
	}
	msg := update.Message

	// New member join via service message — covers direct adds and some group types [FEAT-007, FEAT-010]
	if len(msg.NewChatMembers) > 0 {
		for i := range msg.NewChatMembers {
			h.handleNewMember(msg.Chat.ID, &msg.NewChatMembers[i])
		}
		return
	}

	if msg.From == nil {
		return
	}

	// YES confirmation for /copy [FEAT-012]
	if strings.ToUpper(strings.TrimSpace(msg.Text)) == "YES" && !msg.IsCommand() {
		isAdmin, _ := h.admin.IsAdmin(msg.Chat.ID, msg.From.ID)
		if isAdmin || h.admin.IsSuperAdmin(msg.From.ID) {
			h.handleConfirm(msg)
		}
		return
	}

	if msg.IsCommand() {
		h.handleCommand(msg)
		return
	}

	// Regular message: apply spam filter [FEAT-001, FEAT-002, FEAT-003, FEAT-004]
	h.handleMessage(msg)
}

// handleNewMember runs name filter, verification, and sandbox on join. [FEAT-007, FEAT-010, FEAT-015]
func (h *handler) handleNewMember(chatID int64, user *tgbotapi.User) {
	settings, err := h.storage.GetSettings(chatID)
	if err != nil {
		log.Printf("get settings chatID=%d: %v", chatID, err)
		return
	}
	lang := settings.Language

	// Name/username filter — kick spammers before they can do anything [FEAT-010]
	if settings.NameFilter {
		nameText := user.FirstName + " " + user.LastName + " " + user.UserName
		result, err := h.filter.Check(chatID, nameText)
		if err != nil {
			log.Printf("filter check: %v", err)
		} else if result != nil {
			h.banUser(chatID, user.ID)
			h.unbanUser(chatID, user.ID) // kick = ban + immediate unban (can rejoin via invite)
			log.Printf("kicked user %d (@%s) on join: name matched %s %q", user.ID, user.UserName, result.Type, result.Pattern)
			return
		}
	}

	// Button verification: fully restrict, send button, wait for click [FEAT-015]
	// Sandbox (if also enabled) is applied after successful verification.
	if settings.VerificationEnabled {
		h.restrictUser(chatID, user.ID)

		displayName := user.FirstName
		if displayName == "" {
			displayName = fmt.Sprintf("User #%d", user.ID)
		}

		msgText := i18n.Tf(lang, "verify_message", html.EscapeString(displayName), settings.VerificationTimeoutM)
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(
					i18n.T(lang, "verify_button"),
					fmt.Sprintf("verify:%d:%d", chatID, user.ID),
				),
			),
		)
		sent, err := h.sendWithKeyboard(chatID, msgText, keyboard)
		if err != nil {
			log.Printf("send verification message: %v", err)
			return
		}
		expires := time.Now().Add(time.Duration(settings.VerificationTimeoutM) * time.Minute)
		if err := h.storage.AddPendingVerification(chatID, user.ID, sent.MessageID, expires); err != nil {
			log.Printf("add pending verification: %v", err)
		}
		return
	}

	// Sandbox only (no verification): allow text immediately, block media for sandbox_hours [FEAT-007]
	if settings.SandboxEnabled {
		h.sandboxRestrictUser(chatID, user.ID)
		until := time.Now().Add(time.Duration(settings.SandboxHours) * time.Hour)
		if err := h.storage.AddPendingUnrestrict(chatID, user.ID, until, "sandbox"); err != nil {
			log.Printf("add pending unrestrict: %v", err)
		}
	}
}

// handleMessage applies spam filters to a regular message. [FEAT-004]
func (h *handler) handleMessage(msg *tgbotapi.Message) {
	// Skip admin messages
	isAdmin, _ := h.admin.IsAdmin(msg.Chat.ID, msg.From.ID)
	if isAdmin {
		return
	}

	text := msg.Text
	if text == "" {
		text = msg.Caption
	}
	if text == "" {
		return
	}

	result, err := h.filter.Check(msg.Chat.ID, text)
	if err != nil {
		log.Printf("filter check: %v", err)
		return
	}
	if result == nil {
		return
	}

	settings, err := h.storage.GetSettings(msg.Chat.ID)
	if err != nil {
		log.Printf("get settings: %v", err)
		return
	}
	lang := settings.Language

	h.deleteMessage(msg.Chat.ID, msg.MessageID)

	actionTaken := settings.Action
	switch settings.Action {
	case "ban":
		h.banUser(msg.Chat.ID, msg.From.ID)
	case "kick":
		h.banUser(msg.Chat.ID, msg.From.ID)
		h.unbanUser(msg.Chat.ID, msg.From.ID)
	case "mute":
		until := time.Now().Add(time.Duration(settings.MuteDurationH) * time.Hour)
		h.muteUser(msg.Chat.ID, msg.From.ID, until)
		if err := h.storage.AddPendingUnrestrict(msg.Chat.ID, msg.From.ID, until, "mute"); err != nil {
			log.Printf("add pending unrestrict: %v", err)
		}
	case "delete":
		// message already deleted above
	default:
		h.banUser(msg.Chat.ID, msg.From.ID)
		actionTaken = "ban"
	}

	entry := storage.SpamEntry{
		ChatID:      msg.Chat.ID,
		UserID:      msg.From.ID,
		Username:    msg.From.UserName,
		MatchedRule: fmt.Sprintf("%s:%q", result.Type, result.Pattern),
		MessageText: text,
		ActionTaken: actionTaken,
		CreatedAt:   time.Now(),
	}
	if err := h.storage.LogSpam(entry); err != nil {
		log.Printf("log spam: %v", err)
	}
	log.Printf("spam: user=%d @%s chat=%d rule=%s:%q action=%s",
		msg.From.ID, msg.From.UserName, msg.Chat.ID, result.Type, result.Pattern, actionTaken)

	// Notify the group what happened and why [FEAT-014]
	userRef := fmt.Sprintf("user #%d", msg.From.ID)
	if msg.From.UserName != "" {
		userRef = "@" + html.EscapeString(msg.From.UserName)
	} else if msg.From.FirstName != "" {
		userRef = html.EscapeString(msg.From.FirstName)
	}
	h.sendHTML(msg.Chat.ID, i18n.Tf(lang, "spam_notification",
		userRef,
		result.Type,
		html.EscapeString(result.Pattern),
		actionLabel(lang, actionTaken, settings.MuteDurationH),
	))
}

// handleCommand routes slash commands to the appropriate handler.
func (h *handler) handleCommand(msg *tgbotapi.Message) {
	if msg.From == nil {
		return
	}
	chatID := msg.Chat.ID
	userID := msg.From.ID
	raw := msg.Command()
	// Only handle prefixed commands to avoid conflicts with other bots in the chat.
	// Exception: @botname suffix is stripped by the library, so /tgwatch_add@watch_spam_bot also works.
	if !strings.HasPrefix(raw, "tgwatch_") {
		return
	}
	cmd := strings.TrimPrefix(raw, "tgwatch_")
	args := msg.CommandArguments()

	// /copy is superadmin-only, works in DM [FEAT-012]
	if cmd == "copy" {
		if !h.admin.IsSuperAdmin(userID) {
			return
		}
		h.handleCopy(msg, args)
		return
	}

	// All other commands require group admin; DMs from non-superadmin are ignored
	if msg.Chat.IsPrivate() {
		return
	}
	isAdmin, err := h.admin.IsAdmin(chatID, userID)
	if err != nil {
		log.Printf("admin check chatID=%d userID=%d: %v", chatID, userID, err)
		return
	}
	if !isAdmin {
		return // silent ignore — do not tip off spammers [FEAT-005]
	}

	// Track admin command messages so /clean can delete them [FEAT-013]
	if err := h.storage.TrackMessage(chatID, msg.MessageID); err != nil {
		log.Printf("track message: %v", err)
	}

	switch cmd {
	case "add":
		h.handleAdd(msg, args)
	case "remove", "del":
		h.handleRemove(msg, args)
	case "list":
		h.handleList(chatID, args)
	case "clear":
		h.handleClear(msg, args)
	case "set":
		h.handleSet(msg, args)
	case "show":
		if strings.TrimSpace(args) == "settings" {
			h.handleShowSettings(chatID)
		}
	case "check":
		h.handleCheck(chatID, args) // [FEAT-008]
	case "unrestrict":
		h.handleUnrestrict(msg)
	case "log":
		h.handleLog(msg, args) // [FEAT-009]
	case "clean":
		h.handleClean(chatID) // [FEAT-013]
	case "lang":
		h.handleLang(msg) // [FEAT-016]
	case "help":
		h.handleHelp(chatID)
	}
}

// handleConfirm executes a pending action after the user replies YES.
func (h *handler) handleConfirm(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	action, ok := h.confirms.pop(chatID, msg.From.ID)
	if !ok {
		return
	}
	lang := h.lang(chatID)

	switch action {
	case "clear words":
		if err := h.storage.ClearWords(chatID); err != nil {
			h.send(chatID, i18n.T(lang, "clear_words_fail"))
		} else {
			h.send(chatID, i18n.T(lang, "clear_words_ok_full"))
		}
	case "clear regex":
		if err := h.storage.ClearRegexes(chatID); err != nil {
			h.send(chatID, i18n.T(lang, "clear_regex_fail"))
		} else {
			h.send(chatID, i18n.T(lang, "clear_regex_ok_full"))
		}
	case "clear log":
		if err := h.storage.ClearSpamLog(chatID); err != nil {
			h.send(chatID, i18n.T(lang, "clear_log_fail"))
		} else {
			h.send(chatID, i18n.T(lang, "clear_log_ok"))
		}
	default:
		if strings.HasPrefix(action, "copy:") {
			parts := strings.SplitN(action, ":", 3)
			if len(parts) != 3 {
				return
			}
			srcID, _ := strconv.ParseInt(parts[1], 10, 64)
			dstID, _ := strconv.ParseInt(parts[2], 10, 64)
			if err := h.storage.CopySettings(srcID, dstID); err != nil {
				h.send(chatID, i18n.Tf(lang, "copy_result_fail", err))
			} else {
				h.sendHTML(chatID, i18n.Tf(lang, "copy_result_ok", srcID, dstID))
			}
		}
	}
}

// handleAdd handles /tgwatch_add word and /tgwatch_add regex [FEAT-001, FEAT-002, FEAT-011]
func (h *handler) handleAdd(msg *tgbotapi.Message, args string) {
	chatID := msg.Chat.ID
	lang := h.lang(chatID)
	// Split on first whitespace character (space OR newline) to get the sub-command.
	// CommandArguments() returns the full args including newlines, so splitting on
	// space alone fails for multiline messages like "/tgwatch_add word\nline1\nline2".
	firstWS := strings.IndexAny(args, " \t\n\r")
	var subCmd, rest string
	if firstWS < 0 {
		subCmd = strings.ToLower(strings.TrimSpace(args))
	} else {
		subCmd = strings.ToLower(strings.TrimSpace(args[:firstWS]))
		rest = strings.TrimLeft(args[firstWS+1:], " \t")
	}

	switch subCmd {
	case "word":
		var words []string
		firstLine := strings.SplitN(rest, "\n", 2)[0]
		if strings.TrimSpace(firstLine) != "" && !strings.Contains(rest, "\n") {
			// Single word/phrase on the same line: /tgwatch_add word bitcoin
			words = []string{strings.TrimSpace(rest)}
		} else if strings.Contains(rest, "\n") || strings.TrimSpace(firstLine) == "" {
			// Multiline bulk import: each line is a word/phrase [FEAT-011]
			for _, line := range strings.Split(rest, "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					words = append(words, line)
				}
			}
		}
		if len(words) == 0 {
			h.sendHTML(chatID, i18n.T(lang, "add_word_usage"))
			return
		}
		// Normalize at insert time so matching is consistent [FEAT-003]
		for i, w := range words {
			words[i] = filter.Normalize(w)
		}
		added, skipped, err := h.storage.AddWords(chatID, words)
		if err != nil {
			h.send(chatID, i18n.T(lang, "add_word_fail"))
			return
		}
		if skipped > 0 {
			h.sendHTML(chatID, i18n.Tf(lang, "add_word_ok_skipped", added, skipped))
		} else {
			h.sendHTML(chatID, i18n.Tf(lang, "add_word_ok", added))
		}

	case "regex":
		if strings.TrimSpace(rest) == "" {
			h.sendHTML(chatID, i18n.T(lang, "add_regex_usage"))
			return
		}
		pattern := strings.TrimSpace(rest)
		if _, err := filter.CompileRegex(pattern); err != nil {
			h.sendHTML(chatID, i18n.Tf(lang, "add_regex_invalid", html.EscapeString(err.Error())))
			return
		}
		added, err := h.storage.AddRegex(chatID, pattern)
		if err != nil {
			h.send(chatID, i18n.T(lang, "add_regex_fail"))
			return
		}
		if added {
			h.sendHTML(chatID, i18n.Tf(lang, "add_regex_ok", html.EscapeString(pattern)))
		} else {
			h.send(chatID, i18n.T(lang, "add_regex_exists"))
		}

	default:
		h.sendHTML(chatID, i18n.T(lang, "add_usage"))
	}
}

// handleRemove handles /tgwatch_remove word and /tgwatch_remove regex
func (h *handler) handleRemove(msg *tgbotapi.Message, args string) {
	chatID := msg.Chat.ID
	lang := h.lang(chatID)
	parts := strings.SplitN(args, " ", 2)
	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		h.sendHTML(chatID, i18n.T(lang, "remove_usage"))
		return
	}
	subCmd := strings.ToLower(strings.TrimSpace(parts[0]))
	value := strings.TrimSpace(parts[1])

	switch subCmd {
	case "word":
		removed, err := h.storage.RemoveWord(chatID, filter.Normalize(value))
		if err != nil {
			h.send(chatID, i18n.T(lang, "remove_word_fail"))
			return
		}
		if removed {
			h.sendHTML(chatID, i18n.Tf(lang, "remove_word_ok", html.EscapeString(value)))
		} else {
			h.sendHTML(chatID, i18n.Tf(lang, "remove_word_notfound", html.EscapeString(value)))
		}
	case "regex":
		removed, err := h.storage.RemoveRegex(chatID, value)
		if err != nil {
			h.send(chatID, i18n.T(lang, "remove_regex_fail"))
			return
		}
		if removed {
			h.sendHTML(chatID, i18n.Tf(lang, "remove_regex_ok", html.EscapeString(value)))
		} else {
			h.sendHTML(chatID, i18n.Tf(lang, "remove_regex_notfound", html.EscapeString(value)))
		}
	default:
		h.sendHTML(chatID, i18n.T(lang, "remove_usage_sub"))
	}
}

// handleList handles /tgwatch_list words and /tgwatch_list regex
func (h *handler) handleList(chatID int64, args string) {
	lang := h.lang(chatID)
	switch strings.TrimSpace(strings.ToLower(args)) {
	case "words":
		words, err := h.storage.GetWords(chatID)
		if err != nil {
			h.send(chatID, i18n.T(lang, "list_words_fail"))
			return
		}
		if len(words) == 0 {
			h.send(chatID, i18n.T(lang, "list_words_empty"))
			return
		}
		escaped := make([]string, len(words))
		for i, w := range words {
			escaped[i] = html.EscapeString(w)
		}
		h.sendHTML(chatID, i18n.Tf(lang, "list_words_header", len(words), strings.Join(escaped, "\n")))
	case "regex":
		patterns, err := h.storage.GetRegexes(chatID)
		if err != nil {
			h.send(chatID, i18n.T(lang, "list_regex_fail"))
			return
		}
		if len(patterns) == 0 {
			h.send(chatID, i18n.T(lang, "list_regex_empty"))
			return
		}
		lines := make([]string, len(patterns))
		for i, p := range patterns {
			lines[i] = fmt.Sprintf("%d. <code>%s</code>", i+1, html.EscapeString(p))
		}
		h.sendHTML(chatID, i18n.Tf(lang, "list_regex_header", len(patterns), strings.Join(lines, "\n")))
	default:
		h.send(chatID, i18n.T(lang, "list_usage"))
	}
}

// handleClear handles /tgwatch_clear words and /tgwatch_clear regex (with inline button confirmation)
func (h *handler) handleClear(msg *tgbotapi.Message, args string) {
	chatID := msg.Chat.ID
	userID := msg.From.ID
	lang := h.lang(chatID)
	switch strings.TrimSpace(strings.ToLower(args)) {
	case "words":
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "btn_clear_words"), fmt.Sprintf("confirm:words:%d:%d", chatID, userID)),
				tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "btn_cancel"), fmt.Sprintf("confirm:cancel:%d:%d", chatID, userID)),
			),
		)
		h.sendWithKeyboard(chatID, i18n.T(lang, "clear_words_prompt"), keyboard)
	case "regex":
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "btn_clear_regex"), fmt.Sprintf("confirm:regex:%d:%d", chatID, userID)),
				tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "btn_cancel"), fmt.Sprintf("confirm:cancel:%d:%d", chatID, userID)),
			),
		)
		h.sendWithKeyboard(chatID, i18n.T(lang, "clear_regex_prompt"), keyboard)
	default:
		h.send(chatID, i18n.T(lang, "clear_usage"))
	}
}

// handleSet handles /tgwatch_set <key> <value>
func (h *handler) handleSet(msg *tgbotapi.Message, args string) {
	chatID := msg.Chat.ID
	lang := h.lang(chatID)
	parts := strings.SplitN(args, " ", 2)
	if len(parts) < 2 {
		h.sendHTML(chatID, i18n.T(lang, "set_usage"))
		return
	}
	key := strings.ToLower(strings.TrimSpace(parts[0]))
	val := strings.TrimSpace(parts[1])

	switch key {
	case "action":
		valid := map[string]bool{"delete": true, "mute": true, "ban": true, "kick": true}
		if !valid[val] {
			h.sendHTML(chatID, i18n.T(lang, "set_action_valid"))
			return
		}
		if err := h.storage.SetAction(chatID, val); err != nil {
			h.send(chatID, i18n.T(lang, "set_action_fail"))
			return
		}
		h.sendHTML(chatID, i18n.Tf(lang, "set_action_ok", html.EscapeString(val)))

	case "mute_duration":
		n, err := strconv.Atoi(val)
		if err != nil || n < 1 {
			h.send(chatID, i18n.T(lang, "set_mute_invalid"))
			return
		}
		if err := h.storage.SetMuteDuration(chatID, n); err != nil {
			h.send(chatID, i18n.T(lang, "set_mute_fail"))
			return
		}
		h.sendHTML(chatID, i18n.Tf(lang, "set_mute_ok", n))

	case "sandbox":
		switch strings.ToLower(val) {
		case "on":
			if err := h.storage.SetSandbox(chatID, true); err != nil {
				h.send(chatID, i18n.T(lang, "set_sandbox_enable_fail"))
				return
			}
			h.send(chatID, i18n.T(lang, "set_sandbox_enabled"))
		case "off":
			if err := h.storage.SetSandbox(chatID, false); err != nil {
				h.send(chatID, i18n.T(lang, "set_sandbox_disable_fail"))
				return
			}
			h.send(chatID, i18n.T(lang, "set_sandbox_disabled"))
		default:
			h.send(chatID, i18n.T(lang, "set_sandbox_usage"))
		}

	case "sandbox_duration":
		n, err := strconv.Atoi(val)
		if err != nil || n < 1 {
			h.send(chatID, i18n.T(lang, "set_sandbox_dur_invalid"))
			return
		}
		if err := h.storage.SetSandboxDuration(chatID, n); err != nil {
			h.send(chatID, i18n.T(lang, "set_sandbox_dur_fail"))
			return
		}
		h.sendHTML(chatID, i18n.Tf(lang, "set_sandbox_dur_ok", n))

	case "name_filter":
		switch strings.ToLower(val) {
		case "on":
			if err := h.storage.SetNameFilter(chatID, true); err != nil {
				h.send(chatID, i18n.T(lang, "set_nf_enable_fail"))
				return
			}
			h.send(chatID, i18n.T(lang, "set_nf_enabled"))
		case "off":
			if err := h.storage.SetNameFilter(chatID, false); err != nil {
				h.send(chatID, i18n.T(lang, "set_nf_disable_fail"))
				return
			}
			h.send(chatID, i18n.T(lang, "set_nf_disabled"))
		default:
			h.send(chatID, i18n.T(lang, "set_nf_usage"))
		}

	case "verification":
		switch strings.ToLower(val) {
		case "on":
			if err := h.storage.SetVerification(chatID, true); err != nil {
				h.send(chatID, i18n.T(lang, "set_verif_enable_fail"))
				return
			}
			h.send(chatID, i18n.T(lang, "set_verif_enabled"))
		case "off":
			if err := h.storage.SetVerification(chatID, false); err != nil {
				h.send(chatID, i18n.T(lang, "set_verif_disable_fail"))
				return
			}
			h.send(chatID, i18n.T(lang, "set_verif_disabled"))
		default:
			h.send(chatID, i18n.T(lang, "set_verif_usage"))
		}

	case "verification_timeout":
		n, err := strconv.Atoi(val)
		if err != nil || n < 1 {
			h.send(chatID, i18n.T(lang, "set_verif_to_invalid"))
			return
		}
		if err := h.storage.SetVerificationTimeout(chatID, n); err != nil {
			h.send(chatID, i18n.T(lang, "set_verif_to_fail"))
			return
		}
		h.sendHTML(chatID, i18n.Tf(lang, "set_verif_to_ok", n))

	default:
		h.sendHTML(chatID, i18n.T(lang, "set_unknown"))
	}
}

// handleShowSettings handles /tgwatch_show settings
func (h *handler) handleShowSettings(chatID int64) {
	s, err := h.storage.GetSettings(chatID)
	if err != nil {
		h.send(chatID, i18n.T(i18n.EN, "show_settings_fail"))
		return
	}
	lang := s.Language
	on := i18n.T(lang, "settings_on")
	off := i18n.T(lang, "settings_off")
	sandbox := off
	if s.SandboxEnabled {
		sandbox = on
	}
	nameFilter := off
	if s.NameFilter {
		nameFilter = on
	}
	verification := off
	if s.VerificationEnabled {
		verification = on
	}
	langLabel := i18n.T(lang, "settings_lang_en")
	if lang == i18n.UK {
		langLabel = i18n.T(lang, "settings_lang_uk")
	}
	h.sendHTML(chatID, i18n.Tf(lang, "show_settings",
		s.Action, s.MuteDurationH, nameFilter,
		verification, s.VerificationTimeoutM,
		sandbox, s.SandboxHours, langLabel,
	))
}

// handleCheck tests a message against filters without taking action. [FEAT-008]
func (h *handler) handleCheck(chatID int64, text string) {
	lang := h.lang(chatID)
	if strings.TrimSpace(text) == "" {
		h.sendHTML(chatID, i18n.T(lang, "check_usage"))
		return
	}
	result, err := h.filter.Check(chatID, text)
	if err != nil {
		h.send(chatID, i18n.T(lang, "check_fail"))
		return
	}
	settings, err := h.storage.GetSettings(chatID)
	if err != nil {
		h.send(chatID, i18n.T(lang, "check_cfg_fail"))
		return
	}
	if result == nil {
		h.send(chatID, i18n.T(lang, "check_no_match"))
		return
	}
	h.sendHTML(chatID, i18n.Tf(lang, "check_match",
		result.Type,
		html.EscapeString(result.Pattern),
		html.EscapeString(filter.Normalize(text)),
		actionLabel(lang, settings.Action, settings.MuteDurationH),
	))
}

// handleUnrestrict manually lifts sandbox/mute on a user (reply-to required). [FEAT-007]
func (h *handler) handleUnrestrict(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	lang := h.lang(chatID)
	if msg.ReplyToMessage == nil || msg.ReplyToMessage.From == nil {
		h.send(chatID, i18n.T(lang, "unrestrict_usage"))
		return
	}
	userID := msg.ReplyToMessage.From.ID
	if err := h.storage.RemovePendingUnrestrict(chatID, userID); err != nil {
		log.Printf("remove pending unrestrict: %v", err)
	}
	h.unrestrictUser(chatID, userID)
	h.sendHTML(chatID, i18n.Tf(lang, "unrestrict_ok", userID))
}

// handleLog handles /tgwatch_log, /tgwatch_log <n>, /tgwatch_log clear [FEAT-009]
func (h *handler) handleLog(msg *tgbotapi.Message, args string) {
	chatID := msg.Chat.ID
	lang := h.lang(chatID)
	args = strings.TrimSpace(args)

	if strings.ToLower(args) == "clear" {
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "btn_clear_log"), fmt.Sprintf("confirm:log:%d:%d", chatID, msg.From.ID)),
				tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "btn_cancel"), fmt.Sprintf("confirm:cancel:%d:%d", chatID, msg.From.ID)),
			),
		)
		h.sendWithKeyboard(chatID, i18n.T(lang, "log_clear_prompt"), keyboard)
		return
	}

	limit := 10
	if args != "" {
		n, err := strconv.Atoi(args)
		if err != nil || n < 1 {
			h.send(chatID, i18n.T(lang, "log_usage"))
			return
		}
		if n > 50 {
			n = 50
		}
		limit = n
	}

	entries, err := h.storage.GetSpamLog(chatID, limit)
	if err != nil {
		h.send(chatID, i18n.T(lang, "log_fail"))
		return
	}
	if len(entries) == 0 {
		h.send(chatID, i18n.T(lang, "log_empty"))
		return
	}

	lines := make([]string, len(entries))
	for i, e := range entries {
		ts := e.CreatedAt.Format("01-02 15:04")
		user := fmt.Sprintf("#%d", e.UserID)
		if e.Username != "" {
			user = "@" + html.EscapeString(e.Username)
		}
		lines[i] = fmt.Sprintf("<code>%s</code> %s — %s — <b>%s</b>",
			ts, user, html.EscapeString(e.MatchedRule), html.EscapeString(e.ActionTaken))
	}
	h.sendHTML(chatID, i18n.Tf(lang, "log_header", len(entries), strings.Join(lines, "\n")))
}

// handleCopy handles /tgwatch_copy <src_chat_id> <dst_chat_id> (superadmin only). [FEAT-012]
func (h *handler) handleCopy(msg *tgbotapi.Message, args string) {
	chatID := msg.Chat.ID
	lang := h.lang(chatID)
	parts := strings.Fields(args)
	if len(parts) != 2 {
		h.sendHTML(chatID, i18n.T(lang, "copy_usage"))
		return
	}
	srcID, err1 := strconv.ParseInt(parts[0], 10, 64)
	dstID, err2 := strconv.ParseInt(parts[1], 10, 64)
	if err1 != nil || err2 != nil {
		h.send(chatID, i18n.T(lang, "copy_invalid_ids"))
		return
	}
	h.confirms.set(chatID, msg.From.ID, fmt.Sprintf("copy:%d:%d", srcID, dstID))
	h.sendHTML(chatID, i18n.Tf(lang, "copy_prompt", srcID, dstID))
}

// handleLang shows the language selection keyboard. [FEAT-016]
func (h *handler) handleLang(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	userID := msg.From.ID
	lang := h.lang(chatID)
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				i18n.T(lang, "btn_lang_en"),
				fmt.Sprintf("setlang:en:%d:%d", chatID, userID),
			),
			tgbotapi.NewInlineKeyboardButtonData(
				i18n.T(lang, "btn_lang_uk"),
				fmt.Sprintf("setlang:uk:%d:%d", chatID, userID),
			),
		),
	)
	h.sendWithKeyboard(chatID, i18n.T(lang, "lang_prompt"), keyboard)
}

// handleConfirmCallback handles inline button confirmations for clear/log actions.
func (h *handler) handleConfirmCallback(query *tgbotapi.CallbackQuery) {
	parts := strings.SplitN(query.Data, ":", 4)
	if len(parts) != 4 {
		return
	}
	action := parts[1]
	chatID, err1 := strconv.ParseInt(parts[2], 10, 64)
	userID, err2 := strconv.ParseInt(parts[3], 10, 64)
	if err1 != nil || err2 != nil {
		return
	}
	lang := h.lang(chatID)
	// Only the admin who triggered the command can confirm
	if query.From.ID != userID {
		h.bot.Request(tgbotapi.NewCallback(query.ID, i18n.T(lang, "confirm_admin_only")))
		return
	}
	// Delete the confirmation message
	if query.Message != nil {
		h.bot.Request(tgbotapi.NewDeleteMessage(chatID, query.Message.MessageID))
	}
	if action == "cancel" {
		h.bot.Request(tgbotapi.NewCallback(query.ID, i18n.T(lang, "confirm_cancelled")))
		return
	}
	h.bot.Request(tgbotapi.NewCallback(query.ID, ""))
	switch action {
	case "words":
		if err := h.storage.ClearWords(chatID); err != nil {
			h.send(chatID, i18n.T(lang, "clear_words_fail"))
		} else {
			h.send(chatID, i18n.T(lang, "clear_words_ok"))
		}
	case "regex":
		if err := h.storage.ClearRegexes(chatID); err != nil {
			h.send(chatID, i18n.T(lang, "clear_regex_fail"))
		} else {
			h.send(chatID, i18n.T(lang, "clear_regex_ok"))
		}
	case "log":
		if err := h.storage.ClearSpamLog(chatID); err != nil {
			h.send(chatID, i18n.T(lang, "clear_log_fail"))
		} else {
			h.send(chatID, i18n.T(lang, "clear_log_ok"))
		}
	}
}

// handleCallbackQuery processes inline button presses. [FEAT-015, FEAT-016]
func (h *handler) handleCallbackQuery(query *tgbotapi.CallbackQuery) {
	if strings.HasPrefix(query.Data, "confirm:") {
		h.handleConfirmCallback(query)
		return
	}

	// Language selection [FEAT-016]
	if strings.HasPrefix(query.Data, "setlang:") {
		parts := strings.SplitN(query.Data, ":", 4)
		if len(parts) != 4 {
			return
		}
		newLang := parts[1]
		chatID, err1 := strconv.ParseInt(parts[2], 10, 64)
		userID, err2 := strconv.ParseInt(parts[3], 10, 64)
		if err1 != nil || err2 != nil {
			return
		}
		// Only the admin who ran /lang can confirm
		if query.From.ID != userID {
			h.bot.Request(tgbotapi.NewCallback(query.ID, i18n.T(h.lang(chatID), "confirm_admin_only")))
			return
		}
		if err := h.storage.SetLanguage(chatID, newLang); err != nil {
			h.bot.Request(tgbotapi.NewCallback(query.ID, i18n.T(newLang, "lang_fail")))
			return
		}
		// Delete the selection message
		if query.Message != nil {
			h.bot.Request(tgbotapi.NewDeleteMessage(chatID, query.Message.MessageID))
		}
		h.bot.Request(tgbotapi.NewCallback(query.ID, ""))
		key := "lang_set_en"
		if newLang == i18n.UK {
			key = "lang_set_uk"
		}
		h.sendHTML(chatID, i18n.T(newLang, key))
		return
	}

	if !strings.HasPrefix(query.Data, "verify:") {
		return
	}
	parts := strings.SplitN(query.Data, ":", 3)
	if len(parts) != 3 {
		return
	}
	chatID, err1 := strconv.ParseInt(parts[1], 10, 64)
	expectedUserID, err2 := strconv.ParseInt(parts[2], 10, 64)
	if err1 != nil || err2 != nil {
		return
	}
	lang := h.lang(chatID)

	// Only the user who joined can click their own button
	if query.From.ID != expectedUserID {
		h.bot.Request(tgbotapi.NewCallback(query.ID, i18n.T(lang, "verify_popup_invalid")))
		return
	}

	// Answer the popup immediately so the button stops spinning
	h.bot.Request(tgbotapi.NewCallback(query.ID, i18n.T(lang, "verify_popup_ok")))

	// Remove pending record and get the verification message ID to delete it
	msgID, err := h.storage.RemovePendingVerification(chatID, expectedUserID)
	if err != nil {
		log.Printf("remove pending verification chatID=%d userID=%d: %v", chatID, expectedUserID, err)
	}
	if msgID != 0 {
		h.bot.Request(tgbotapi.NewDeleteMessage(chatID, msgID))
	}

	// Apply sandbox (text-only) if enabled, otherwise fully unrestrict [FEAT-007 + FEAT-015]
	settings, err := h.storage.GetSettings(chatID)
	if err != nil {
		log.Printf("get settings after verification: %v", err)
		h.unrestrictUser(chatID, expectedUserID)
		return
	}
	if settings.SandboxEnabled {
		h.sandboxRestrictUser(chatID, expectedUserID)
		until := time.Now().Add(time.Duration(settings.SandboxHours) * time.Hour)
		if err := h.storage.AddPendingUnrestrict(chatID, expectedUserID, until, "sandbox"); err != nil {
			log.Printf("add pending unrestrict after verification: %v", err)
		}
		log.Printf("verified user %d in chat %d — sandbox applied for %dh", expectedUserID, chatID, settings.SandboxHours)
	} else {
		h.unrestrictUser(chatID, expectedUserID)
		log.Printf("verified user %d in chat %d — fully unrestricted", expectedUserID, chatID)
	}
}

// kickExpiredVerifications kicks users who never clicked the verification button. [FEAT-015]
func (h *handler) kickExpiredVerifications() {
	items, err := h.storage.GetExpiredVerifications()
	if err != nil {
		log.Printf("get expired verifications: %v", err)
		return
	}
	for _, item := range items {
		// Delete the verification message
		if item.MessageID != 0 {
			h.bot.Request(tgbotapi.NewDeleteMessage(item.ChatID, item.MessageID))
		}
		// Kick (ban + unban so they can rejoin with a new invite if allowed)
		h.banUser(item.ChatID, item.UserID)
		h.unbanUser(item.ChatID, item.UserID)
		if _, err := h.storage.RemovePendingVerification(item.ChatID, item.UserID); err != nil {
			log.Printf("remove pending verification on kick: %v", err)
		}
		log.Printf("verification timeout: kicked user %d from chat %d", item.UserID, item.ChatID)
	}
}

// liftExpiredRestrictions is called every minute by the background goroutine. [FEAT-007]
func (h *handler) liftExpiredRestrictions() {
	items, err := h.storage.GetExpiredUnrestricts()
	if err != nil {
		log.Printf("get expired unrestricts: %v", err)
		return
	}
	for _, item := range items {
		h.unrestrictUser(item.ChatID, item.UserID)
		if err := h.storage.RemovePendingUnrestrict(item.ChatID, item.UserID); err != nil {
			log.Printf("remove pending unrestrict chatID=%d userID=%d: %v", item.ChatID, item.UserID, err)
		}
		log.Printf("lifted %s restriction: userID=%d chatID=%d", item.Reason, item.UserID, item.ChatID)
	}
}

// handleClean deletes all tracked bot responses and admin command messages. [FEAT-013]
// Silent — sends no confirmation to avoid leaving yet another message.
func (h *handler) handleClean(chatID int64) {
	ids, err := h.storage.PopTrackedMessages(chatID)
	if err != nil {
		log.Printf("pop tracked messages chatID=%d: %v", chatID, err)
		return
	}
	deleted := 0
	for _, id := range ids {
		if _, err := h.bot.Request(tgbotapi.NewDeleteMessage(chatID, id)); err == nil {
			deleted++
		}
	}
	log.Printf("clean: deleted %d messages in chat %d", deleted, chatID)
}

// handleHelp shows a quick command reference.
func (h *handler) handleHelp(chatID int64) {
	h.sendHTML(chatID, i18n.T(h.lang(chatID), "help"))
}

// --- Telegram API action helpers ---

// sendWithKeyboard sends an HTML message with an inline keyboard and tracks it for /clean.
func (h *handler) sendWithKeyboard(chatID int64, text string, keyboard tgbotapi.InlineKeyboardMarkup) (tgbotapi.Message, error) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.ReplyMarkup = keyboard
	sent, err := h.bot.Send(msg)
	if err != nil {
		return tgbotapi.Message{}, err
	}
	if err := h.storage.TrackMessage(chatID, sent.MessageID); err != nil {
		log.Printf("track message: %v", err)
	}
	return sent, nil
}

// sandboxRestrictUser applies text-only restriction: regular messages allowed, no media/stickers/link previews. [FEAT-015]
func (h *handler) sandboxRestrictUser(chatID, userID int64) {
	cfg := tgbotapi.RestrictChatMemberConfig{
		ChatMemberConfig: tgbotapi.ChatMemberConfig{ChatID: chatID, UserID: userID},
		UntilDate:        0,
		Permissions: &tgbotapi.ChatPermissions{
			CanSendMessages:       true,
			CanSendMediaMessages:  false,
			CanSendOtherMessages:  false,
			CanAddWebPagePreviews: false,
		},
	}
	if _, err := h.bot.Request(cfg); err != nil {
		log.Printf("sandbox restrict user=%d chat=%d: %v", userID, chatID, err)
	}
}

func (h *handler) deleteMessage(chatID int64, messageID int) {
	if _, err := h.bot.Request(tgbotapi.NewDeleteMessage(chatID, messageID)); err != nil {
		log.Printf("delete message chatID=%d msgID=%d: %v", chatID, messageID, err)
	}
}

func (h *handler) banUser(chatID, userID int64) {
	cfg := tgbotapi.BanChatMemberConfig{
		ChatMemberConfig: tgbotapi.ChatMemberConfig{ChatID: chatID, UserID: userID},
		UntilDate:        0,
	}
	if _, err := h.bot.Request(cfg); err != nil {
		log.Printf("ban user=%d chat=%d: %v", userID, chatID, err)
	}
}

func (h *handler) unbanUser(chatID, userID int64) {
	cfg := tgbotapi.UnbanChatMemberConfig{
		ChatMemberConfig: tgbotapi.ChatMemberConfig{ChatID: chatID, UserID: userID},
	}
	if _, err := h.bot.Request(cfg); err != nil {
		log.Printf("unban user=%d chat=%d: %v", userID, chatID, err)
	}
}

func (h *handler) muteUser(chatID, userID int64, until time.Time) {
	cfg := tgbotapi.RestrictChatMemberConfig{
		ChatMemberConfig: tgbotapi.ChatMemberConfig{ChatID: chatID, UserID: userID},
		UntilDate:        until.Unix(),
		Permissions: &tgbotapi.ChatPermissions{
			CanSendMessages:       false,
			CanSendMediaMessages:  false,
			CanSendOtherMessages:  false,
			CanAddWebPagePreviews: false,
		},
	}
	if _, err := h.bot.Request(cfg); err != nil {
		log.Printf("mute user=%d chat=%d: %v", userID, chatID, err)
	}
}

func (h *handler) restrictUser(chatID, userID int64) {
	cfg := tgbotapi.RestrictChatMemberConfig{
		ChatMemberConfig: tgbotapi.ChatMemberConfig{ChatID: chatID, UserID: userID},
		UntilDate:        0,
		Permissions: &tgbotapi.ChatPermissions{
			CanSendMessages:       false,
			CanSendMediaMessages:  false,
			CanSendOtherMessages:  false,
			CanAddWebPagePreviews: false,
		},
	}
	if _, err := h.bot.Request(cfg); err != nil {
		log.Printf("restrict user=%d chat=%d: %v", userID, chatID, err)
	}
}

func (h *handler) unrestrictUser(chatID, userID int64) {
	cfg := tgbotapi.RestrictChatMemberConfig{
		ChatMemberConfig: tgbotapi.ChatMemberConfig{ChatID: chatID, UserID: userID},
		UntilDate:        0,
		Permissions: &tgbotapi.ChatPermissions{
			CanSendMessages:       true,
			CanSendMediaMessages:  true,
			CanSendOtherMessages:  true,
			CanAddWebPagePreviews: true,
		},
	}
	if _, err := h.bot.Request(cfg); err != nil {
		log.Printf("unrestrict user=%d chat=%d: %v", userID, chatID, err)
	}
}
