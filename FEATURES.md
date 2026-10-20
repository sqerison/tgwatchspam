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
| FEAT-017 | Done   | internal/bot/handlers.go:verificationContent, handleSetVerificationText, internal/storage/storage.go:SetVerificationText | Custom welcome text + chat rules (expandable quote, accept button, rules link) |
| FEAT-018 | Done   | internal/bot/handlers.go:handleMessage, deleteDueMessages, internal/storage/storage.go:ScheduleDeletion, PopDueDeletions | Auto-delete spam notifications after N minutes (`/tgwatch_set notify_delete`) |
| FEAT-019 | Done   | internal/storage/ai.go | Labelled corpus (`flagged_messages`), `pending_reviews`, `members` join times, `ai_usage` daily budget |
| FEAT-020 | Done   | internal/ai/ai.go, internal/ai/prompt.go | Provider-agnostic AI client (gemini / openai / claude) + prompt builder |
| FEAT-021 | Done   | internal/bot/ai.go:handleAnalyze | `/tgwatch_analyze` — reply to a message to classify it |
| FEAT-022 | Done   | internal/bot/ai.go:reviewKeywords, handleReviewCallback | Admin-picked filter words: multi-select, risk markers, false-hit check, confirm step |
| FEAT-023 | Done   | internal/bot/ai.go:fewShotExamples | Admin decisions used as few-shot examples in the prompt |
| FEAT-024 | Done   | internal/bot/ai.go:aiScan, handleSetAI | Automatic AI scan of unmatched messages (`ai_scan off\|review\|warn\|delete`, threshold, daily limit, scope, notes) |
| FEAT-025 | Done   | internal/bot/ai.go:openReview, handleReviewCallback, expireReviews, handleSetReviewTo | Review requests: `review_to chat\|dm\|group`, Spam / Delete only / Not spam (rule verdicts: Delete / Not a violation only), `review_timeout` (default 24h) |
