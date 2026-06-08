# TGBot - Telegram Spam Filter Bot
## Product Document v1.0

---

## Overview

A self-hosted Telegram bot written in Go, designed to protect Ukrainian community groups in Italy from spam. Runs on a private server, managed entirely through Telegram commands by group admins. No web interface — no extra attack surface, no maintenance overhead.

This document is the single source of truth for the project. A developer receiving this document should have everything needed to build the bot without further clarification.

---

## Background & Problem

The bot owner manages 2-3 Ukrainian community Telegram groups (Ukrainians living in Italy). These groups receive regular spam including:
- Crypto exchange offers (USDT, Bitcoin, Binance, Bybit)
- Job recruitment scams ("earn $200/day", "join my team")
- Relocation/immigration services (Spain, Bulgaria, VNJ)
- Weight loss drug sales (Ozempic, Mounjaro)
- Telegram channel promotion services (invites, mass messaging)
- Ticket reselling scams
- Private tutoring spam

The existing bot (tgdev) has proven unreliable due to:
- Word boundary issues (e.g. `binance/bybit` with a slash not matching)
- Cyrillic/Latin lookalike bypass (e.g. `Rоblox` with Cyrillic "o")
- Opaque regex behavior
- No debugging tools

---

## Target Environment

- **Chats:** 2-3 Telegram groups (Ukrainian community, Italy)
- **Languages in chats:** Ukrainian (primary), Russian, Italian, some English
- **Server:** Self-hosted Linux VPS — owner has 3 servers available
- **Expected load:** Low (community groups, not public channels)
- **Runtime:** 24/7, unattended

---

## Tech Stack

| Component  | Choice                     | Reason                                      |
|------------|----------------------------|---------------------------------------------|
| Language   | Go                         | Low memory (~10-20MB), single binary, fast  |
| TG Library | go-telegram-bot-api v5     | Stable, well-maintained, long polling       |
| Database   | SQLite (modernc.org/sqlite)| Zero setup, file-based, no external DB      |
| Config     | .env file                  | Simple, standard, easy to manage on server  |

---

## Project Structure

```
tgbot/
  cmd/
    bot/
      main.go                  # Entry point. Loads config, inits bot, starts polling.
  internal/
    bot/
      bot.go                   # Bot initialization and polling loop
      handlers.go              # Routes incoming messages and commands to correct handler
    filter/
      filter.go                # Core filtering engine - orchestrates all filter types
      words.go                 # Word/phrase matching logic [FEAT-001]
      regex.go                 # Regex pattern matching logic [FEAT-002]
      normalize.go             # Cyrillic/Latin normalization before matching [FEAT-003]
    admin/
      admin.go                 # Admin authorization checks and command routing [FEAT-005]
    storage/
      storage.go               # SQLite storage - all reads/writes go through here
      schema.sql               # Database schema
    config/
      config.go                # Loads and validates .env config
  .env.example                 # Template for environment variables (commit this)
  .env                         # Actual secrets (do NOT commit)
  .gitignore
  go.mod
  go.sum
  PRODUCT.md                   # This file
  FEATURES.md                  # Auto-index: feature ID -> file:line (maintained by developer)
  CHANGELOG.md                 # Version history
  README.md                    # Setup and deployment steps
```

---

## Environment Variables (.env)

```env
BOT_TOKEN=7123456789:AAFxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
SUPERADMIN_ID=123456789
DATABASE_PATH=./data/bot.db
LOG_LEVEL=info
```

| Variable       | Required | Description                                              |
|----------------|----------|----------------------------------------------------------|
| BOT_TOKEN      | Yes      | Telegram bot token from @BotFather                       |
| SUPERADMIN_ID  | Yes      | Telegram user ID of the owner (from @userinfobot)        |
| DATABASE_PATH  | No       | Path to SQLite file. Default: ./data/bot.db              |
| LOG_LEVEL      | No       | debug / info / warn / error. Default: info               |

---

## Database Schema

```sql
-- Per-chat word/phrase list
CREATE TABLE words (
  id       INTEGER PRIMARY KEY AUTOINCREMENT,
  chat_id  INTEGER NOT NULL,
  word     TEXT NOT NULL,
  UNIQUE(chat_id, word)
);

-- Per-chat regex pattern list
CREATE TABLE regexes (
  id       INTEGER PRIMARY KEY AUTOINCREMENT,
  chat_id  INTEGER NOT NULL,
  pattern  TEXT NOT NULL,
  UNIQUE(chat_id, pattern)
);

-- Per-chat settings
CREATE TABLE settings (
  chat_id          INTEGER PRIMARY KEY,
  action           TEXT NOT NULL DEFAULT 'ban',   -- delete|mute|ban|kick
  mute_duration_h  INTEGER NOT NULL DEFAULT 24,
  sandbox_enabled  BOOLEAN NOT NULL DEFAULT 0,
  sandbox_hours    INTEGER NOT NULL DEFAULT 1,
  name_filter      BOOLEAN NOT NULL DEFAULT 1
);

-- Spam log
CREATE TABLE spam_log (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  chat_id      INTEGER NOT NULL,
  user_id      INTEGER NOT NULL,
  username     TEXT,
  matched_rule TEXT NOT NULL,
  message_text TEXT NOT NULL,
  action_taken TEXT NOT NULL,
  created_at   DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

---

## Features

Each feature is tagged with its ID (e.g. `[FEAT-001]`). Use this ID to:
- Search in code: `grep -r "FEAT-001" .`
- Find in FEATURES.md for exact file:line location
- Reference in git commits: `feat: add word matching [FEAT-001]`

---

### FEAT-001 - Word/Phrase Filtering
**Status:** Planned
**File:** `internal/filter/words.go`

Maintain a forbidden word/phrase list per chat. When an incoming message contains any listed word or phrase, it is flagged as spam.

**Matching rules:**
- Case-insensitive (always)
- Substring match — word does NOT need to be a whole word. `доход` matches `доходності`
- No word boundary restrictions — avoids the main tgdev bug where `Binance/Bybit` (with slash) did not match
- Normalization applied first (see FEAT-003)
- Multi-word phrases supported (e.g. `гарна плата`, `digital nomad`)

**Commands (group admin only):**
```
/add word <word or phrase>     — add one entry
/add word                      — multiline mode: one word per line in message body
/remove word <word or phrase>  — remove one entry
/list words                    — show all words for this chat
/clear words                   — remove all words for this chat (asks confirmation)
```

---

### FEAT-002 - Regex Filtering
**Status:** Planned
**File:** `internal/filter/regex.go`

Maintain a regex pattern list per chat. Patterns are matched against the full message text.

**Matching rules:**
- Go RE2 regex syntax
- Case-insensitive flag (`(?i)`) prepended automatically
- Normalization applied to message before matching (see FEAT-003)
- Matched against full message text (not word by word)

**Example patterns:**
```
u[\.\s]?s[\.\s]?d[\.\s]?t     — matches usdt, u.s.d.t, u s d t
\+3[4-9]\d{7,}                 — matches non-Italian phone numbers
зп.{0,5}\d+\$                  — matches salary spam like "ЗП: 200$"
```

**Commands (group admin only):**
```
/add regex <pattern>    — add a regex pattern
/remove regex <pattern> — remove a pattern
/list regex             — show all patterns for this chat
/clear regex            — remove all patterns (asks confirmation)
```

---

### FEAT-003 - Cyrillic/Latin Lookalike Normalization
**Status:** Planned
**File:** `internal/filter/normalize.go`

Before any matching (words or regex), normalize the message text by mapping visually identical characters between Cyrillic and Latin alphabets.

**Why:** Spammers write `Rоblox` using Cyrillic "о" instead of Latin "o" to bypass word filters.

**Lookalike map (Cyrillic -> Latin):**
```
а -> a,  е -> e,  о -> o,  р -> p,  с -> c,
х -> x,  у -> y,  і -> i,  А -> A,  В -> B,
Е -> E,  К -> K,  М -> M,  Н -> H,  О -> O,
Р -> P,  С -> C,  Т -> T,  Х -> X,  У -> Y
```

Normalization is applied to the message before matching. The stored word list is also normalized at insert time so comparisons are consistent.

---

### FEAT-004 - Spam Action
**Status:** Planned
**File:** `internal/bot/handlers.go`

When a message matches any filter, the bot takes a configurable action.

**Available actions:**

| Action  | What happens                                              |
|---------|-----------------------------------------------------------|
| delete  | Message deleted only                                      |
| mute    | Message deleted + user restricted for N hours             |
| ban     | Message deleted + user permanently banned from group      |
| kick    | Message deleted + user removed (can rejoin via invite)    |

**Default action:** `ban` (most spam comes from throwaway accounts)

**Commands (group admin only):**
```
/set action delete|mute|ban|kick   — set action for this chat
/set mute_duration <hours>         — hours to mute (used when action=mute)
/show settings                     — show all current settings for this chat
```

---

### FEAT-005 - Admin Authorization
**Status:** Planned
**File:** `internal/admin/admin.go`

Only authorized users can run management commands.

**Two levels:**

| Level      | Who                                   | What they can do                             |
|------------|---------------------------------------|----------------------------------------------|
| Admin      | Any Telegram group admin              | Manage words, regex, settings for that chat  |
| Superadmin | User ID set in .env (SUPERADMIN_ID)   | Everything + /copy between chats, global ops |

**Behavior:**
- Admin status verified via Telegram `getChatMember` API on each command call
- Non-admins who send commands are silently ignored — no reply (avoids tipping off spammers)
- Superadmin can operate via DMs with the bot (not just inside groups)

---

### FEAT-006 - Multi-Chat Support
**Status:** Planned
**File:** `internal/storage/storage.go`

One bot instance manages multiple chats at the same time. All data is isolated by `chat_id`.

Each chat independently has:
- Its own word list
- Its own regex list
- Its own action setting
- Its own mute duration
- Its own sandbox settings

No data is shared between chats unless explicitly copied via FEAT-012.

---

### FEAT-007 - New Member Sandbox
**Status:** Planned
**File:** `internal/bot/handlers.go`

When a new user joins the group, restrict their ability to post for a configurable period. After the period expires, restrictions are lifted automatically.

**Behavior:**
- On `new_chat_member` event: restrict user (no messages, no media)
- After `sandbox_hours`: unrestrict user automatically
- Admin can manually unrestrict earlier with `/unrestrict <user>`

**Commands (group admin only):**
```
/set sandbox on|off          — enable or disable sandbox for new members
/set sandbox_duration <hours> — how long new members are restricted (default: 1)
/unrestrict <username or reply> — manually lift restriction on a user
```

---

### FEAT-008 - Message Test Command
**Status:** Planned
**File:** `internal/bot/handlers.go`

Admin can test any text against the current chat's filters without taking any action. Used to debug why messages slip through or to verify new rules before adding them.

**Command (group admin only):**
```
/check <message text>
```

**Bot reply example:**
```
Matched: word "binance" (normalized: "binance")
Action would be: ban
```
or
```
No match. Message would not be filtered.
```

---

### FEAT-009 - Spam Log
**Status:** Planned
**File:** `internal/storage/storage.go` + `internal/bot/handlers.go`

Every filtered message is saved to the spam_log table with full context.

**Logged fields:** timestamp, chat_id, user_id, username, matched rule, original message text, action taken.

**Commands (group admin only):**
```
/log        — last 10 spam entries for this chat
/log <n>    — last N entries (max 50)
/log clear  — clear spam log for this chat
```

---

### FEAT-010 - Username/Name Filtering on Join
**Status:** Planned
**File:** `internal/filter/filter.go`

When a new user joins, run their display name and @username through the same word/regex filters. If matched, kick them immediately before they post anything.

**Why:** Catches spammers with names like `crypto_earn_fast`, `заработок_online`, `usdt_exchanger`.

**Commands (group admin only):**
```
/set name_filter on|off   — enable or disable name filtering (default: on)
```

---

### FEAT-011 - Bulk Word Import
**Status:** Planned
**File:** `internal/bot/handlers.go`

Add multiple words in a single message by putting each word on its own line. Duplicates are silently ignored.

**Usage:**
```
/add word
bitcoin
usdt
крипта
заработок
гарна плата
```

Bot replies with: `Added 5 words. 0 duplicates skipped.`

---

### FEAT-012 - Copy Settings Between Chats
**Status:** Planned
**File:** `internal/admin/admin.go`

Superadmin can copy the full configuration from one chat to another: word list, regex list, and settings.

**Command (superadmin only, via DM):**
```
/copy <source_chat_id> <target_chat_id>
```

Bot asks for confirmation before overwriting. Existing data in target chat is replaced, not merged.

---

## Command Reference (Full List)

| Command                        | Who        | Description                                  |
|-------------------------------|------------|----------------------------------------------|
| /add word <text>               | Admin      | Add word or phrase                           |
| /remove word <text>            | Admin      | Remove word or phrase                        |
| /list words                    | Admin      | List all words for this chat                 |
| /clear words                   | Admin      | Clear all words (with confirmation)          |
| /add regex <pattern>           | Admin      | Add regex pattern                            |
| /remove regex <pattern>        | Admin      | Remove regex pattern                         |
| /list regex                    | Admin      | List all regex patterns                      |
| /clear regex                   | Admin      | Clear all regex patterns (with confirmation) |
| /set action <value>            | Admin      | Set spam action: delete/mute/ban/kick        |
| /set mute_duration <hours>     | Admin      | Set mute duration in hours                   |
| /set sandbox on/off            | Admin      | Enable/disable new member sandbox            |
| /set sandbox_duration <hours>  | Admin      | Set sandbox duration                         |
| /set name_filter on/off        | Admin      | Enable/disable name filtering on join        |
| /show settings                 | Admin      | Show all settings for this chat              |
| /check <text>                  | Admin      | Test text against filters (no action taken)  |
| /unrestrict <user>             | Admin      | Manually lift sandbox on a user              |
| /log                           | Admin      | Show last 10 spam log entries                |
| /log <n>                       | Admin      | Show last N spam log entries                 |
| /log clear                     | Admin      | Clear spam log for this chat                 |
| /copy <src_id> <dst_id>        | Superadmin | Copy all settings from one chat to another   |

---

## Non-Features (Out of Scope for v1)

- **Web admin panel** — all management is via Telegram commands
- **Captcha/human verification** — use @MissRose_bot alongside this bot for that
- **AI/ML spam detection** — rule-based only, behavior must be predictable
- **Rate limiting / flood protection** — not needed for community groups at this scale
- **Multi-language bot responses** — bot replies in English only

---

## Deployment

1. Build binary: `go build -o tgbot ./cmd/bot`
2. Copy binary + `.env` to server
3. Create `systemd` service to run it as a background process
4. Bot uses long polling — no domain, no SSL, no open ports required

---

## Out of Scope Integrations

- @MissRose_bot handles captcha (already set up separately)
- tgdev bot can remain active during transition — they do not conflict
