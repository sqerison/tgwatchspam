# Changelog

## v1.1.0 — 2026-06-11

- FEAT-016: Per-chat language switching — `/tgwatch_lang` shows inline buttons (🇬🇧 English / 🇺🇦 Українська); all bot messages, verification prompts, and notifications respect the selected language; stored per chat_id in SQLite
- FEAT-017: Custom welcome message and chat rules per chat — `/tgwatch_set welcome`, `/tgwatch_set rules`, `/tgwatch_set rules_url`; rules are shown to new members as a tap-to-expand quote inside the verification message, the button becomes “✅ I accept the rules”, and an optional “📖 Chat rules” link button opens a full article; every change is previewed and validated before it is saved
- FEAT-018: Auto-delete spam notifications — `/tgwatch_set notify_delete <minutes>|off` removes the "Spam removed" message N minutes after it is posted (max 2880 = Telegram's 48-hour limit on deleting bot messages); timers are stored in the new `pending_deletions` table and processed by the minute ticker, so they survive restarts; default is off
- FEAT-019 – FEAT-025: AI spam analysis — optional `AI_PROVIDER` (`gemini` default model `gemini-3.1-flash-lite`, `claude`, `openai`) classifies messages the word/regex filter missed as ok / spam / scam / rule, using the chat rules, admin notes (`ai_notes`) and admin-labelled examples from this chat; `/tgwatch_analyze` for one message; automatic `ai_scan off|review|warn|delete` with confidence threshold, daily limit and new-members-only scope; review requests with ⛔ Spam / 🗑 Delete only / ✅ Not spam go to the group, admins' DMs or any group (`review_to`); after Spam the bot suggests filter words as multi-select buttons with risk markers and a false-hit check — nothing is added without confirmation

## v1.0.0 — 2026-06-08

Initial implementation. All 12 planned features shipped.

- FEAT-001: Word/phrase filtering — case-insensitive substring match, no word boundaries
- FEAT-002: Regex filtering — RE2 syntax, (?i) prepended automatically
- FEAT-003: Cyrillic/Latin lookalike normalization — prevents bypass with mixed scripts
- FEAT-004: Configurable spam actions — delete / mute / ban / kick, default ban
- FEAT-005: Two-level admin auth — group admins + superadmin via .env; non-admins silently ignored
- FEAT-006: Multi-chat support — all data isolated by chat_id
- FEAT-007: New member sandbox — DB-persisted timer, survives bot restarts
- FEAT-008: /check command — dry-run filter test for admins
- FEAT-009: Spam log — stored in SQLite, /log [n|clear]
- FEAT-010: Name/username filtering on join — kicks spammer accounts before first message
- FEAT-011: Bulk word import — multiline /add word
- FEAT-012: /copy command — superadmin copies full config between chats
- FEAT-013: /clean command — deletes all bot replies and admin command messages from the chat
- FEAT-014: Spam notification — after each filtered message, bot posts who was caught, what rule matched, and what action was taken
- FEAT-015: Button verification — new members must tap an inline button before posting; configurable timeout; if they don't verify in time they are kicked; sandbox applies after successful verification
