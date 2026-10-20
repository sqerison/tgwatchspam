package bot

import (
	"context"
	"fmt"
	"html"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sqerison/tgwatchspam/internal/ai"
	"github.com/sqerison/tgwatchspam/internal/filter"
	"github.com/sqerison/tgwatchspam/internal/i18n"
	"github.com/sqerison/tgwatchspam/internal/storage"
)

const (
	aiMinRunes        = 20               // shorter messages are never sent to the model [FEAT-024]
	newMemberDays     = 30               // ai_scope=new covers members who joined within this many days
	reviewTTL         = 24 * time.Hour   // open review requests expire after this [FEAT-025]
	reviewCleanup     = 10 * time.Minute // decided requests in the group are deleted after this
	analyzeCooldown   = 10 * time.Second // per-chat cooldown for /tgwatch_analyze [FEAT-021]
	aiCallTimeout     = 60 * time.Second
	maxReviewTextLen  = 1000
	maxAINotesLen     = 1000
	exampleSpamCount  = 8 // few-shot examples per label [FEAT-023]
	exampleRuleCount  = 4
	exampleOKCount    = 8
	falseHitCorpusMax = 300
)

// cooldown is a small per-chat rate limiter. [FEAT-021]
type cooldown struct {
	mu   sync.Mutex
	last map[int64]time.Time
}

func (c *cooldown) allow(chatID int64, every time.Duration) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.last == nil {
		c.last = make(map[int64]time.Time)
	}
	if time.Since(c.last[chatID]) < every {
		return false
	}
	c.last[chatID] = time.Now()
	return true
}

// userRef returns an escaped @username, first name, or numeric fallback.
func userRef(u *tgbotapi.User) string {
	if u == nil {
		return "?"
	}
	if u.UserName != "" {
		return "@" + html.EscapeString(u.UserName)
	}
	if u.FirstName != "" {
		return html.EscapeString(u.FirstName)
	}
	return fmt.Sprintf("user #%d", u.ID)
}

// messageText returns the text or caption of a message.
func messageText(m *tgbotapi.Message) string {
	if m.Text != "" {
		return m.Text
	}
	return m.Caption
}

// messageLink builds a t.me link to a group message, or "" for basic groups.
func messageLink(chat *tgbotapi.Chat, messageID int) string {
	if chat.UserName != "" {
		return fmt.Sprintf("https://t.me/%s/%d", chat.UserName, messageID)
	}
	id := strconv.FormatInt(chat.ID, 10)
	if strings.HasPrefix(id, "-100") {
		return fmt.Sprintf("https://t.me/c/%s/%d", strings.TrimPrefix(id, "-100"), messageID)
	}
	return ""
}

// --- Classification [FEAT-020, FEAT-023] ---

// classify sends a message with the chat's rules, notes and labelled examples to the model.
func (h *handler) classify(s *storage.Settings, msg *tgbotapi.Message, text string) (*ai.Verdict, error) {
	req := ai.Request{
		Lang:     s.Language,
		Rules:    s.RulesText,
		Notes:    s.AINotes,
		Examples: h.fewShotExamples(s.ChatID),
		Meta:     h.messageMeta(msg),
		Message:  text,
	}
	ctx, cancel := context.WithTimeout(context.Background(), aiCallTimeout)
	defer cancel()
	return h.ai.Classify(ctx, req)
}

// fewShotExamples returns the newest admin-labelled messages of this chat. [FEAT-023]
// When admins have confirmed little spam yet, messages blocked by the keyword filter fill in.
func (h *handler) fewShotExamples(chatID int64) []ai.Example {
	var out []ai.Example
	add := func(limit int, labels ...string) int {
		items, err := h.storage.GetFlaggedTexts(chatID, limit, labels...)
		if err != nil {
			log.Printf("get flagged examples: %v", err)
			return 0
		}
		for _, m := range items {
			out = append(out, ai.Example{Text: m.Text, Label: m.Label})
		}
		return len(items)
	}
	spam := add(exampleSpamCount, ai.VerdictSpam, ai.VerdictScam)
	add(exampleRuleCount, ai.VerdictRule)
	add(exampleOKCount, ai.VerdictOK)
	if spam < exampleSpamCount/2 {
		texts, err := h.storage.GetRecentSpamTexts(chatID, exampleSpamCount/2-spam)
		if err != nil {
			log.Printf("get spam log examples: %v", err)
		}
		for _, t := range texts {
			out = append(out, ai.Example{Text: t, Label: ai.VerdictSpam})
		}
	}
	return out
}

// messageMeta lists facts about the author and message that help the model. Bots cannot
// see account age, so "time in this chat" is the closest signal.
func (h *handler) messageMeta(msg *tgbotapi.Message) []string {
	var meta []string
	if msg.From != nil {
		if joined, ok := h.storage.JoinedAt(msg.Chat.ID, msg.From.ID); ok {
			days := int(time.Since(joined).Hours() / 24)
			meta = append(meta, fmt.Sprintf("the author joined this chat %d day(s) ago", days))
		} else {
			meta = append(meta, "the author joined this chat more than a few weeks ago or the join was not seen")
		}
		if msg.From.UserName == "" {
			meta = append(meta, "the author has no @username")
		}
	}
	if msg.ForwardFrom != nil || msg.ForwardFromChat != nil || msg.ForwardSenderName != "" {
		meta = append(meta, "the message is forwarded")
	}
	if msg.ReplyToMessage != nil {
		meta = append(meta, "the message is a reply to another member")
	}
	for _, e := range append(msg.Entities, msg.CaptionEntities...) {
		if e.Type == "url" || e.Type == "text_link" {
			meta = append(meta, "the message contains a link")
			break
		}
	}
	return meta
}

// reviewKeywords filters the model's suggestions: drops words already in the filter or too
// short, and counts how many messages admins marked "not spam" each one would have hit. [FEAT-022]
func (h *handler) reviewKeywords(chatID int64, suggested []ai.Keyword) []storage.ReviewKeyword {
	existing, _ := h.storage.GetWords(chatID)
	have := make(map[string]bool, len(existing))
	for _, w := range existing {
		have[w] = true
	}
	hamItems, _ := h.storage.GetFlaggedTexts(chatID, falseHitCorpusMax, ai.VerdictOK)
	ham := make([]string, len(hamItems))
	for i, m := range hamItems {
		ham[i] = filter.Normalize(m.Text)
	}
	var out []storage.ReviewKeyword
	for _, k := range suggested {
		norm := filter.Normalize(k.Text)
		if utf8.RuneCountInString(norm) < 3 || have[norm] {
			continue
		}
		have[norm] = true
		rk := storage.ReviewKeyword{Text: k.Text, Risk: k.Risk}
		for _, t := range ham {
			if strings.Contains(t, norm) {
				rk.FalseHits++
			}
		}
		if rk.FalseHits > 0 {
			rk.Risk = ai.RiskHigh
		}
		out = append(out, rk)
	}
	return out
}

// --- Automatic scan [FEAT-024] ---

// aiScan runs after a message passed the keyword filter.
func (h *handler) aiScan(msg *tgbotapi.Message, s *storage.Settings, text string) {
	if h.ai == nil || s.AIScan == "" || s.AIScan == "off" {
		return
	}
	if utf8.RuneCountInString(strings.TrimSpace(text)) < aiMinRunes {
		return
	}
	if s.AIScope == "new" {
		joined, ok := h.storage.JoinedAt(msg.Chat.ID, msg.From.ID)
		if !ok || time.Since(joined) > newMemberDays*24*time.Hour {
			return
		}
	}
	ok, firstRefusal, err := h.storage.ConsumeAICall(msg.Chat.ID, s.AIDailyLimit)
	if err != nil {
		log.Printf("ai budget chat=%d: %v", msg.Chat.ID, err)
		return
	}
	if !ok {
		if firstRefusal {
			h.notifyReviewers(s, i18n.Tf(s.Language, "ai_limit_reached", s.AIDailyLimit))
		}
		return
	}

	v, err := h.classify(s, msg, text)
	if err != nil {
		log.Printf("ai classify chat=%d msg=%d: %v", msg.Chat.ID, msg.MessageID, err)
		return
	}
	log.Printf("ai scan chat=%d msg=%d user=%d verdict=%s confidence=%.2f",
		msg.Chat.ID, msg.MessageID, msg.From.ID, v.Verdict, v.Confidence)
	if v.Verdict == ai.VerdictOK || int(v.Confidence*100+0.5) < s.AIThreshold {
		return
	}

	switch s.AIScan {
	case "warn":
		h.sendWarning(msg, s, v)
	case "review":
		h.openReview(msg, s, v, "scan", false, 0)
	case "delete":
		h.deleteMessage(msg.Chat.ID, msg.MessageID)
		h.logAISpam(msg, text, v, "delete")
		h.openReview(msg, s, v, "scan", true, 0)
	}
}

// sendWarning replies with a short caution and schedules its removal. [FEAT-024]
func (h *handler) sendWarning(msg *tgbotapi.Message, s *storage.Settings, v *ai.Verdict) {
	text := i18n.T(s.Language, "ai_warn")
	if v.Verdict == ai.VerdictRule {
		text = i18n.T(s.Language, "ai_warn_rule")
		if v.Rule != "" {
			text += "\n<i>" + html.EscapeString(v.Rule) + "</i>"
		}
	}
	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ParseMode = tgbotapi.ModeHTML
	reply.ReplyToMessageID = msg.MessageID
	sent, err := h.bot.Send(reply)
	if err != nil {
		log.Printf("send ai warning: %v", err)
		return
	}
	if err := h.storage.TrackMessage(msg.Chat.ID, sent.MessageID); err != nil {
		log.Printf("track message: %v", err)
	}
	h.scheduleDelete(msg.Chat.ID, sent.MessageID, h.cleanupDelay(s))
}

func (h *handler) logAISpam(msg *tgbotapi.Message, text string, v *ai.Verdict, action string) {
	entry := storage.SpamEntry{
		ChatID:      msg.Chat.ID,
		UserID:      msg.From.ID,
		Username:    msg.From.UserName,
		MatchedRule: fmt.Sprintf("ai:%s %.0f%%", v.Verdict, v.Confidence*100),
		MessageText: text,
		ActionTaken: action,
		CreatedAt:   time.Now(),
	}
	if err := h.storage.LogSpam(entry); err != nil {
		log.Printf("log spam: %v", err)
	}
}

// cleanupDelay is how long bot messages about a decision stay in the group.
func (h *handler) cleanupDelay(s *storage.Settings) time.Duration {
	if s.NotifyDeleteM > 0 {
		return time.Duration(s.NotifyDeleteM) * time.Minute
	}
	return reviewCleanup
}

func (h *handler) scheduleDelete(chatID int64, messageID int, after time.Duration) {
	if err := h.storage.ScheduleDeletion(chatID, messageID, time.Now().Add(after)); err != nil {
		log.Printf("schedule deletion: %v", err)
	}
}

// --- /tgwatch_analyze [FEAT-021] ---

func (h *handler) handleAnalyze(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	s, err := h.storage.GetSettings(chatID)
	if err != nil {
		log.Printf("get settings: %v", err)
		return
	}
	lang := s.Language
	if h.ai == nil {
		h.send(chatID, i18n.T(lang, "ai_not_configured"))
		return
	}
	target := msg.ReplyToMessage
	if target == nil || target.From == nil || strings.TrimSpace(messageText(target)) == "" {
		h.send(chatID, i18n.T(lang, "analyze_usage"))
		return
	}
	if !h.analyzeLimit.allow(chatID, analyzeCooldown) {
		h.send(chatID, i18n.T(lang, "analyze_wait"))
		return
	}
	v, err := h.classify(s, target, messageText(target))
	if err != nil {
		log.Printf("ai analyze chat=%d: %v", chatID, err)
		h.sendHTML(chatID, i18n.Tf(lang, "analyze_fail", html.EscapeString(err.Error())))
		return
	}
	log.Printf("ai analyze chat=%d msg=%d verdict=%s confidence=%.2f", chatID, target.MessageID, v.Verdict, v.Confidence)
	h.openReview(target, s, v, "analyze", false, chatID)
}

// --- Review requests [FEAT-025] ---

// openReview stores a review and posts it. destOverride sends it to that chat instead of
// the configured destination (used by /tgwatch_analyze, which answers where it was asked).
func (h *handler) openReview(msg *tgbotapi.Message, s *storage.Settings, v *ai.Verdict, source string, deleted bool, destOverride int64) {
	name := msg.From.FirstName
	if name == "" {
		name = fmt.Sprintf("#%d", msg.From.ID)
	}
	var kws []storage.ReviewKeyword
	if v.Verdict == ai.VerdictSpam || v.Verdict == ai.VerdictScam {
		kws = h.reviewKeywords(msg.Chat.ID, v.Keywords)
	}
	r := &storage.Review{
		ChatID:         msg.Chat.ID,
		MessageID:      msg.MessageID,
		UserID:         msg.From.ID,
		Username:       msg.From.UserName,
		DisplayName:    name,
		Text:           messageText(msg),
		Link:           messageLink(msg.Chat, msg.MessageID),
		Verdict:        v.Verdict,
		Confidence:     v.Confidence,
		Reason:         v.Reason,
		Rule:           v.Rule,
		Keywords:       kws,
		Source:         source,
		MessageDeleted: deleted,
		CreatedAt:      time.Now(),
		ExpiresAt:      time.Now().Add(reviewTTL),
	}
	if err := h.storage.CreateReview(r); err != nil {
		log.Printf("create review: %v", err)
		return
	}

	text, kb := h.renderReview(r, s.Language)
	var copies []storage.MessageRef
	post := func(chatID int64, replyTo int) {
		if id := h.sendReviewMessage(chatID, text, kb, replyTo); id != 0 {
			copies = append(copies, storage.MessageRef{ChatID: chatID, MessageID: id})
		}
	}
	switch {
	case destOverride != 0:
		post(destOverride, msg.MessageID)
	case s.ReviewTo == "dm":
		for _, id := range parseIDs(s.ReviewTarget) {
			post(id, 0)
		}
	case s.ReviewTo == "group":
		if ids := parseIDs(s.ReviewTarget); len(ids) == 1 {
			post(ids[0], 0)
		}
	default: // chat
		replyTo := msg.MessageID
		if deleted {
			replyTo = 0
		}
		post(msg.Chat.ID, replyTo)
	}
	if len(copies) == 0 {
		log.Printf("review %d: could not deliver to any destination (review_to=%s)", r.ID, s.ReviewTo)
		return
	}
	if err := h.storage.SetReviewCopies(r.ID, copies); err != nil {
		log.Printf("set review copies: %v", err)
	}
}

// notifyReviewers sends a plain notice to wherever review requests go.
func (h *handler) notifyReviewers(s *storage.Settings, text string) {
	switch s.ReviewTo {
	case "dm", "group":
		for _, id := range parseIDs(s.ReviewTarget) {
			h.sendReviewMessage(id, text, nil, 0)
		}
	default:
		if id := h.sendHTML(s.ChatID, text); id != 0 {
			h.scheduleDelete(s.ChatID, id, h.cleanupDelay(s))
		}
	}
}

// sendReviewMessage sends an HTML message without link previews, tracks it for /tgwatch_clean
// and returns its ID (0 on failure).
func (h *handler) sendReviewMessage(chatID int64, text string, kb *tgbotapi.InlineKeyboardMarkup, replyTo int) int {
	m := tgbotapi.NewMessage(chatID, text)
	m.ParseMode = tgbotapi.ModeHTML
	m.DisableWebPagePreview = true
	m.ReplyToMessageID = replyTo
	m.AllowSendingWithoutReply = true
	if kb != nil {
		m.ReplyMarkup = *kb
	}
	sent, err := h.bot.Send(m)
	if err != nil {
		log.Printf("send review message chat=%d: %v", chatID, err)
		return 0
	}
	if err := h.storage.TrackMessage(chatID, sent.MessageID); err != nil {
		log.Printf("track message: %v", err)
	}
	return sent.MessageID
}

func parseIDs(s string) []int64 {
	var out []int64
	for _, part := range strings.Split(s, ",") {
		if id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64); err == nil {
			out = append(out, id)
		}
	}
	return out
}

// review UI stages
const (
	stageDecision = iota // Spam / Delete only / Not spam
	stageKeywords        // toggle suggested words
	stageConfirm         // "Add N words?"
	stageFinal           // no buttons
)

func reviewStage(r *storage.Review) int {
	switch {
	case r.Status == "open":
		return stageDecision
	case r.Status == "spam" && len(r.Keywords) > 0 && !r.KeywordsDone:
		return stageKeywords
	}
	return stageFinal
}

// renderReview builds the text and keyboard for the review's current stage.
func (h *handler) renderReview(r *storage.Review, lang string) (string, *tgbotapi.InlineKeyboardMarkup) {
	return h.renderReviewStage(r, lang, reviewStage(r))
}

func (h *handler) renderReviewStage(r *storage.Review, lang string, stage int) (string, *tgbotapi.InlineKeyboardMarkup) {
	var sb strings.Builder
	sb.WriteString(i18n.Tf(lang, "review_title", i18n.T(lang, "verdict_"+r.Verdict), int(r.Confidence*100+0.5)))
	author := html.EscapeString(r.DisplayName)
	if r.Username != "" {
		author += " (@" + html.EscapeString(r.Username) + ")"
	}
	sb.WriteString("\n" + i18n.Tf(lang, "review_author", author))
	if r.Reason != "" {
		sb.WriteString("\n" + i18n.Tf(lang, "review_reason", html.EscapeString(r.Reason)))
	}
	if r.Rule != "" {
		sb.WriteString("\n" + i18n.Tf(lang, "review_rule", html.EscapeString(r.Rule)))
	}
	text := r.Text
	if utf8.RuneCountInString(text) > maxReviewTextLen {
		text = string([]rune(text)[:maxReviewTextLen]) + "…"
	}
	sb.WriteString("\n<blockquote expandable>" + html.EscapeString(text) + "</blockquote>")
	if r.MessageDeleted {
		sb.WriteString("\n" + i18n.T(lang, "review_deleted"))
	} else if r.Link != "" {
		sb.WriteString("\n" + i18n.Tf(lang, "review_link", html.EscapeString(r.Link)))
	}

	id := r.ID
	cb := func(action string) string { return fmt.Sprintf("ar:%s:%d", action, id) }

	switch stage {
	case stageDecision:
		if r.Status == "expired" {
			sb.WriteString("\n\n" + i18n.T(lang, "review_expired"))
			return sb.String(), nil
		}
		kb := tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "btn_review_spam"), cb("s")),
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "btn_review_delete"), cb("d")),
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "btn_review_ok"), cb("n")),
		))
		return sb.String(), &kb
	}

	sb.WriteString("\n\n" + h.decisionLine(r, lang))

	switch stage {
	case stageKeywords:
		sb.WriteString("\n\n" + i18n.T(lang, "kw_prompt"))
		var rows [][]tgbotapi.InlineKeyboardButton
		for i, k := range r.Keywords {
			mark := "▫️"
			if r.Selected&(1<<i) != 0 {
				mark = "✅"
			}
			label := fmt.Sprintf("%s %s %s", mark, riskMarker(k.Risk), truncateRunes(k.Text, 40))
			if k.FalseHits > 0 {
				label += " " + i18n.Tf(lang, "kw_false_hits", k.FalseHits)
			}
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(label, fmt.Sprintf("ar:t:%d:%d", id, i)),
			))
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(i18n.Tf(lang, "btn_kw_add", popcount(r.Selected)), cb("a")),
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "btn_kw_done"), cb("f")),
		))
		kb := tgbotapi.NewInlineKeyboardMarkup(rows...)
		return sb.String(), &kb
	case stageConfirm:
		var picked []string
		for i, k := range r.Keywords {
			if r.Selected&(1<<i) != 0 {
				picked = append(picked, "<code>"+html.EscapeString(k.Text)+"</code>")
			}
		}
		sb.WriteString("\n\n" + i18n.Tf(lang, "kw_confirm", len(picked), strings.Join(picked, ", ")))
		kb := tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "btn_kw_confirm"), cb("c")),
			tgbotapi.NewInlineKeyboardButtonData(i18n.T(lang, "btn_kw_back"), cb("b")),
		))
		return sb.String(), &kb
	}
	return sb.String(), nil
}

func (h *handler) decisionLine(r *storage.Review, lang string) string {
	by := html.EscapeString(r.DecidedBy)
	switch r.Status {
	case "spam":
		return i18n.Tf(lang, "review_decided_spam", by)
	case "rule":
		return i18n.Tf(lang, "review_decided_rule", by)
	case "ok":
		return i18n.Tf(lang, "review_decided_ok", by)
	}
	return i18n.T(lang, "review_expired")
}

func riskMarker(risk string) string {
	switch risk {
	case ai.RiskLow:
		return "🟢"
	case ai.RiskHigh:
		return "🔴"
	}
	return "🟡"
}

func popcount(n int) int {
	c := 0
	for ; n != 0; n &= n - 1 {
		c++
	}
	return c
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// editMessage replaces a message's text and keyboard; a nil keyboard removes the buttons.
func (h *handler) editMessage(ref storage.MessageRef, text string, kb *tgbotapi.InlineKeyboardMarkup) {
	edit := tgbotapi.NewEditMessageText(ref.ChatID, ref.MessageID, text)
	edit.ParseMode = tgbotapi.ModeHTML
	edit.DisableWebPagePreview = true
	edit.ReplyMarkup = kb
	if _, err := h.bot.Request(edit); err != nil && !strings.Contains(err.Error(), "message is not modified") {
		log.Printf("edit message chat=%d msg=%d: %v", ref.ChatID, ref.MessageID, err)
	}
}

// handleReviewCallback processes the review buttons: ar:<action>:<reviewID>[:<index>]. [FEAT-022, FEAT-025]
func (h *handler) handleReviewCallback(q *tgbotapi.CallbackQuery) {
	parts := strings.Split(q.Data, ":")
	if len(parts) < 3 {
		return
	}
	action := parts[1]
	id, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return
	}
	r, err := h.storage.GetReview(id)
	if err != nil || r == nil {
		h.bot.Request(tgbotapi.NewCallback(q.ID, ""))
		return
	}
	s, err := h.storage.GetSettings(r.ChatID)
	if err != nil {
		log.Printf("get settings: %v", err)
		return
	}
	lang := s.Language

	// Only admins of the chat the message came from may decide — not everyone who can see
	// the request (it may be in the group itself or in a separate review group).
	if isAdmin, _ := h.admin.IsAdmin(r.ChatID, q.From.ID); !isAdmin {
		h.bot.Request(tgbotapi.NewCallback(q.ID, i18n.T(lang, "review_admin_only")))
		return
	}
	var here storage.MessageRef
	if q.Message != nil {
		here = storage.MessageRef{ChatID: q.Message.Chat.ID, MessageID: q.Message.MessageID}
	}
	by := q.From.FirstName
	if q.From.UserName != "" {
		by = "@" + q.From.UserName
	}

	switch action {
	case "s", "d", "n":
		status := map[string]string{"s": "spam", "d": "rule", "n": "ok"}[action]
		won, err := h.storage.DecideReview(id, status, by)
		if err != nil {
			log.Printf("decide review: %v", err)
			return
		}
		if !won {
			h.bot.Request(tgbotapi.NewCallback(q.ID, i18n.Tf(lang, "review_already", r.DecidedBy)))
			return
		}
		h.bot.Request(tgbotapi.NewCallback(q.ID, ""))
		r.Status, r.DecidedBy = status, by
		h.applyReviewDecision(r, s)
		h.refreshReviewCopies(r, s, here)

	case "t":
		if reviewStage(r) != stageKeywords || len(parts) != 4 {
			h.bot.Request(tgbotapi.NewCallback(q.ID, ""))
			return
		}
		i, err := strconv.Atoi(parts[3])
		if err != nil || i < 0 || i >= len(r.Keywords) {
			return
		}
		r.Selected ^= 1 << i
		if err := h.storage.SetReviewSelected(id, r.Selected); err != nil {
			log.Printf("set review selection: %v", err)
		}
		h.bot.Request(tgbotapi.NewCallback(q.ID, ""))
		text, kb := h.renderReview(r, lang)
		h.editMessage(here, text, kb)

	case "a":
		if reviewStage(r) != stageKeywords {
			h.bot.Request(tgbotapi.NewCallback(q.ID, ""))
			return
		}
		if r.Selected == 0 {
			h.bot.Request(tgbotapi.NewCallback(q.ID, i18n.T(lang, "kw_select_one")))
			return
		}
		h.bot.Request(tgbotapi.NewCallback(q.ID, ""))
		text, kb := h.renderReviewStage(r, lang, stageConfirm)
		h.editMessage(here, text, kb)

	case "b":
		h.bot.Request(tgbotapi.NewCallback(q.ID, ""))
		text, kb := h.renderReview(r, lang)
		h.editMessage(here, text, kb)

	case "c", "f":
		first, err := h.storage.FinishReviewKeywords(id)
		if err != nil {
			log.Printf("finish review keywords: %v", err)
			return
		}
		h.bot.Request(tgbotapi.NewCallback(q.ID, ""))
		r.KeywordsDone = true
		text, _ := h.renderReview(r, lang)
		if first && action == "c" {
			var words, shown []string
			for i, k := range r.Keywords {
				if r.Selected&(1<<i) != 0 {
					words = append(words, filter.Normalize(k.Text))
					shown = append(shown, "<code>"+html.EscapeString(k.Text)+"</code>")
				}
			}
			if _, _, err := h.storage.AddWords(r.ChatID, words); err != nil {
				log.Printf("add words from review: %v", err)
			} else {
				text += "\n\n" + i18n.Tf(lang, "kw_added", strings.Join(shown, ", "))
				log.Printf("review %d: %s added %d word(s) to chat %d", id, by, len(words), r.ChatID)
			}
		}
		h.editMessage(here, text, nil)

	default:
		h.bot.Request(tgbotapi.NewCallback(q.ID, ""))
	}
}

// applyReviewDecision acts on the original message and stores the label for few-shot use.
func (h *handler) applyReviewDecision(r *storage.Review, s *storage.Settings) {
	label := r.Status
	switch r.Status {
	case "spam":
		label = ai.VerdictSpam
		if r.Verdict == ai.VerdictScam {
			label = ai.VerdictScam
		}
		if !r.MessageDeleted {
			h.deleteMessage(r.ChatID, r.MessageID)
		}
		taken := h.applyAction(r.ChatID, r.UserID, s)
		entry := storage.SpamEntry{
			ChatID:      r.ChatID,
			UserID:      r.UserID,
			Username:    r.Username,
			MatchedRule: fmt.Sprintf("ai:%s (confirmed by %s)", r.Verdict, r.DecidedBy),
			MessageText: r.Text,
			ActionTaken: taken,
			CreatedAt:   time.Now(),
		}
		if err := h.storage.LogSpam(entry); err != nil {
			log.Printf("log spam: %v", err)
		}
	case "rule":
		label = ai.VerdictRule
		if !r.MessageDeleted {
			h.deleteMessage(r.ChatID, r.MessageID)
		}
	case "ok":
		label = ai.VerdictOK
	}
	err := h.storage.AddFlagged(storage.FlaggedMessage{
		ChatID:    r.ChatID,
		UserID:    r.UserID,
		Username:  r.Username,
		Text:      r.Text,
		Label:     label,
		Source:    r.Source,
		CreatedAt: time.Now(),
	})
	if err != nil {
		log.Printf("add flagged message: %v", err)
	}
	log.Printf("review %d: %s decided %s (model said %s %.2f)", r.ID, r.DecidedBy, r.Status, r.Verdict, r.Confidence)
}

// refreshReviewCopies updates every copy of a decided request. The copy the admin tapped keeps
// the keyword buttons; the others just show the outcome. Copies inside the group are cleaned up.
func (h *handler) refreshReviewCopies(r *storage.Review, s *storage.Settings, here storage.MessageRef) {
	for _, c := range r.Copies {
		inGroup := c.ChatID == r.ChatID
		if inGroup && r.Status == "ok" {
			h.deleteMessage(c.ChatID, c.MessageID) // not spam: leave no trace in the group
			continue
		}
		if c == here {
			text, kb := h.renderReview(r, s.Language)
			h.editMessage(c, text, kb)
		} else {
			text, _ := h.renderReviewStage(r, s.Language, stageFinal)
			h.editMessage(c, text, nil)
		}
		if inGroup {
			h.scheduleDelete(c.ChatID, c.MessageID, h.cleanupDelay(s))
		}
	}
}

// expireReviews closes requests nobody answered within reviewTTL. [FEAT-025]
func (h *handler) expireReviews() {
	items, err := h.storage.ExpireReviews()
	if err != nil {
		log.Printf("expire reviews: %v", err)
		return
	}
	for _, r := range items {
		r.Status = "expired"
		lang := h.lang(r.ChatID)
		for _, c := range r.Copies {
			if c.ChatID == r.ChatID {
				h.deleteMessage(c.ChatID, c.MessageID)
				continue
			}
			text, _ := h.renderReview(r, lang)
			h.editMessage(c, text, nil)
		}
	}
}

// --- Settings [FEAT-024, FEAT-025] ---

// handleSetAI handles the ai_* and review_to keys of /tgwatch_set. Returns false for other keys.
func (h *handler) handleSetAI(msg *tgbotapi.Message, key, val string) bool {
	chatID := msg.Chat.ID
	lang := h.lang(chatID)
	fail := func(err error) bool {
		log.Printf("set %s chat=%d: %v", key, chatID, err)
		h.send(chatID, i18n.T(lang, "set_ai_fail"))
		return true
	}
	switch key {
	case "ai_scan":
		mode := strings.ToLower(val)
		if mode != "off" && mode != "review" && mode != "warn" && mode != "delete" {
			h.sendHTML(chatID, i18n.T(lang, "set_ai_scan_usage"))
			return true
		}
		if err := h.storage.SetAIScan(chatID, mode); err != nil {
			return fail(err)
		}
		reply := i18n.Tf(lang, "set_ai_scan_ok", mode)
		if h.ai == nil && mode != "off" {
			reply += "\n\n" + i18n.T(lang, "ai_not_configured")
		}
		h.sendHTML(chatID, reply)
	case "ai_threshold":
		n, err := strconv.Atoi(strings.TrimSuffix(val, "%"))
		if err != nil || n < 50 || n > 100 {
			h.sendHTML(chatID, i18n.T(lang, "set_ai_threshold_usage"))
			return true
		}
		if err := h.storage.SetAIThreshold(chatID, n); err != nil {
			return fail(err)
		}
		h.sendHTML(chatID, i18n.Tf(lang, "set_ai_threshold_ok", n))
	case "ai_limit":
		n, err := strconv.Atoi(val)
		if err != nil || n < 1 || n > 10000 {
			h.sendHTML(chatID, i18n.T(lang, "set_ai_limit_usage"))
			return true
		}
		if err := h.storage.SetAIDailyLimit(chatID, n); err != nil {
			return fail(err)
		}
		h.sendHTML(chatID, i18n.Tf(lang, "set_ai_limit_ok", n))
	case "ai_scope":
		scope := strings.ToLower(val)
		if scope != "all" && scope != "new" {
			h.sendHTML(chatID, i18n.Tf(lang, "set_ai_scope_usage", newMemberDays))
			return true
		}
		if err := h.storage.SetAIScope(chatID, scope); err != nil {
			return fail(err)
		}
		if scope == "new" {
			h.sendHTML(chatID, i18n.Tf(lang, "set_ai_scope_ok_new", newMemberDays))
		} else {
			h.sendHTML(chatID, i18n.T(lang, "set_ai_scope_ok_all"))
		}
	case "ai_notes":
		notes := val
		if strings.EqualFold(notes, "off") {
			notes = ""
		}
		if n := utf8.RuneCountInString(notes); n > maxAINotesLen {
			h.send(chatID, i18n.Tf(lang, "set_text_too_long", n, maxAINotesLen))
			return true
		}
		if err := h.storage.SetAINotes(chatID, notes); err != nil {
			return fail(err)
		}
		if notes == "" {
			h.send(chatID, i18n.T(lang, "set_ai_notes_off"))
		} else {
			h.send(chatID, i18n.T(lang, "set_ai_notes_ok"))
		}
	case "review_to":
		h.handleSetReviewTo(msg, val)
	default:
		return false
	}
	return true
}

// handleSetReviewTo handles /tgwatch_set review_to chat | dm @a @b | group <chat_id>. [FEAT-025]
func (h *handler) handleSetReviewTo(msg *tgbotapi.Message, val string) {
	chatID := msg.Chat.ID
	lang := h.lang(chatID)
	fields := strings.Fields(val)
	if len(fields) == 0 {
		h.sendHTML(chatID, i18n.T(lang, "review_to_usage"))
		return
	}
	title := html.EscapeString(msg.Chat.Title)
	switch strings.ToLower(fields[0]) {
	case "chat":
		if err := h.storage.SetReviewTo(chatID, "chat", ""); err != nil {
			h.send(chatID, i18n.T(lang, "set_ai_fail"))
			return
		}
		h.sendHTML(chatID, i18n.T(lang, "review_to_chat_ok"))

	case "dm":
		if len(fields) < 2 {
			h.sendHTML(chatID, i18n.T(lang, "review_to_usage"))
			return
		}
		admins, err := h.bot.GetChatAdministrators(tgbotapi.ChatAdministratorsConfig{
			ChatConfig: tgbotapi.ChatConfig{ChatID: chatID},
		})
		if err != nil {
			log.Printf("get chat administrators: %v", err)
			h.send(chatID, i18n.T(lang, "set_ai_fail"))
			return
		}
		byName := map[string]*tgbotapi.User{}
		byID := map[int64]*tgbotapi.User{}
		for _, a := range admins {
			if a.User == nil || a.User.IsBot {
				continue
			}
			byID[a.User.ID] = a.User
			if a.User.UserName != "" {
				byName[strings.ToLower(a.User.UserName)] = a.User
			}
		}
		var ids []string
		var resolved []*tgbotapi.User
		var unknown []string
		for _, f := range fields[1:] {
			name := strings.TrimPrefix(f, "@")
			var u *tgbotapi.User
			if n, err := strconv.ParseInt(name, 10, 64); err == nil {
				u = byID[n]
			} else {
				u = byName[strings.ToLower(name)]
			}
			if u == nil {
				unknown = append(unknown, html.EscapeString(f))
				continue
			}
			ids = append(ids, strconv.FormatInt(u.ID, 10))
			resolved = append(resolved, u)
		}
		if len(unknown) > 0 {
			h.sendHTML(chatID, i18n.Tf(lang, "review_to_dm_unknown", strings.Join(unknown, ", ")))
			return
		}
		var undeliverable []string
		for _, u := range resolved {
			if h.sendReviewMessage(u.ID, i18n.Tf(lang, "review_dm_test", title), nil, 0) == 0 {
				undeliverable = append(undeliverable, userRef(u))
			}
		}
		if err := h.storage.SetReviewTo(chatID, "dm", strings.Join(ids, ",")); err != nil {
			h.send(chatID, i18n.T(lang, "set_ai_fail"))
			return
		}
		names := make([]string, len(resolved))
		for i, u := range resolved {
			names[i] = userRef(u)
		}
		reply := i18n.Tf(lang, "review_to_dm_ok", strings.Join(names, ", "))
		if len(undeliverable) > 0 {
			reply += "\n\n" + i18n.Tf(lang, "review_to_dm_undeliverable",
				strings.Join(undeliverable, ", "), "https://t.me/"+h.bot.Self.UserName+"?start=review")
		}
		h.sendHTML(chatID, reply)

	case "group":
		if len(fields) != 2 {
			h.sendHTML(chatID, i18n.T(lang, "review_to_usage"))
			return
		}
		target, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			h.sendHTML(chatID, i18n.T(lang, "review_to_usage"))
			return
		}
		if h.sendReviewMessage(target, i18n.Tf(lang, "review_group_test", title), nil, 0) == 0 {
			h.sendHTML(chatID, i18n.Tf(lang, "review_to_group_fail", target))
			return
		}
		if err := h.storage.SetReviewTo(chatID, "group", strconv.FormatInt(target, 10)); err != nil {
			h.send(chatID, i18n.T(lang, "set_ai_fail"))
			return
		}
		h.sendHTML(chatID, i18n.Tf(lang, "review_to_group_ok", target))

	default:
		h.sendHTML(chatID, i18n.T(lang, "review_to_usage"))
	}
}

// aiSettingsSummary renders the AI block of /tgwatch_show settings.
func (h *handler) aiSettingsSummary(s *storage.Settings) string {
	lang := s.Language
	provider := i18n.T(lang, "settings_not_set")
	if h.ai != nil {
		provider = html.EscapeString(h.ai.Name())
	}
	notes := i18n.T(lang, "settings_not_set")
	if s.AINotes != "" {
		notes = i18n.T(lang, "settings_set")
	}
	review := s.ReviewTo
	if s.ReviewTo != "chat" && s.ReviewTarget != "" {
		review += " " + html.EscapeString(s.ReviewTarget)
	}
	return i18n.Tf(lang, "show_settings_ai",
		provider, s.AIScan, s.AIThreshold, s.AIDailyLimit, h.storage.AIUsageToday(s.ChatID),
		s.AIScope, notes, review)
}
