# Changelog

## v1.1.0 — 2026-06-11

- FEAT-016: Per-chat language switching — `/tgwatch_lang` shows inline buttons (🇬🇧 English / 🇺🇦 Українська); all bot messages, verification prompts, and notifications respect the selected language; stored per chat_id in SQLite

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
