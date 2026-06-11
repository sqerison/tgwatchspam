# Features Index

Auto-maintained index of feature IDs to implementation locations.
Search code with: `grep -r "FEAT-XXX" .`

| ID       | Status | File(s)                                          | Description                              |
|----------|--------|--------------------------------------------------|------------------------------------------|
| FEAT-001 | Done   | internal/filter/words.go, handlers.go:handleAdd  | Word/phrase filtering (substring match)  |
| FEAT-002 | Done   | internal/filter/regex.go, handlers.go:handleAdd  | Regex pattern filtering (RE2, (?i))      |
| FEAT-003 | Done   | internal/filter/normalize.go                     | Cyrillic/Latin lookalike normalization   |
| FEAT-004 | Done   | internal/bot/handlers.go:handleMessage           | Spam actions: delete/mute/ban/kick       |
| FEAT-005 | Done   | internal/admin/admin.go                          | Admin authorization (admin + superadmin) |
| FEAT-006 | Done   | internal/storage/storage.go                      | Multi-chat support (all ops keyed by chat_id) |
| FEAT-007 | Done   | internal/bot/handlers.go:handleNewMember, liftExpiredRestrictions | New member sandbox (DB-persisted timer) |
| FEAT-008 | Done   | internal/bot/handlers.go:handleCheck             | /check test command (dry-run filter)     |
| FEAT-009 | Done   | internal/storage/storage.go:LogSpam, GetSpamLog  | Spam log with /log command               |
| FEAT-010 | Done   | internal/bot/handlers.go:handleNewMember         | Username/name filtering on join          |
| FEAT-011 | Done   | internal/bot/handlers.go:handleAdd               | Bulk word import (multiline /add word)   |
| FEAT-012 | Done   | internal/bot/handlers.go:handleCopy              | Copy settings between chats (superadmin) |
| FEAT-013 | Done   | internal/bot/handlers.go:handleClean, storage.go:TrackMessage | /clean — delete all bot replies and admin commands |
| FEAT-014 | Done   | internal/bot/handlers.go:handleMessage (spam notification)    | Spam notification in chat: who, what matched, action taken |
| FEAT-015 | Done   | internal/bot/handlers.go:handleNewMember, handleCallbackQuery, kickExpiredVerifications | Button verification for new members |
| FEAT-016 | Done   | internal/i18n/i18n.go, internal/storage/storage.go, internal/bot/handlers.go:handleLang | Per-chat language switching (🇬🇧 English / 🇺🇦 Ukrainian) |
