# tgwatchspam

Self-hosted Telegram spam filter bot for community groups. Written in Go, zero external dependencies, managed entirely through Telegram commands.

**Documentation:** [sqerison.github.io/tgwatchspam](https://sqerison.github.io/tgwatchspam)

## Setup

1. Copy `.env.example` to `.env` and fill in your values:
   ```
   BOT_TOKEN=   # from @BotFather
   SUPERADMIN_ID=   # your numeric Telegram user ID (get from @userinfobot)
   ```

2. Build and run:
   ```bash
   go build -o tgwatchspam ./cmd/bot
   ./tgwatchspam
   ```
   On startup the bot automatically registers all commands with BotFather.

3. Add the bot to your group(s) and grant it **admin rights** with:
   - Delete messages
   - Ban users
   - Restrict members

4. *(Optional)* AI analysis — set `AI_PROVIDER` (`gemini`, `claude` or `openai`) and `AI_API_KEY` in `.env`. Gemini Flash-Lite (default model `gemini-3.1-flash-lite`) has a free tier; its inputs may be used by Google for training, which is fine for public groups — use a paid key for private ones. `AI_MODEL` and `AI_BASE_URL` override the model and endpoint (required `AI_MODEL` for `openai`).

## Deployment (systemd)

If you're deploying to a Linux server, build a Linux binary first:

```bash
# Linux AMD64 (most VPS/cloud servers)
GOOS=linux GOARCH=amd64 go build -o tgwatchspam-linux ./cmd/bot

# Linux ARM64 (Raspberry Pi, AWS Graviton, etc.)
GOOS=linux GOARCH=arm64 go build -o tgwatchspam-linux-arm64 ./cmd/bot
```

No CGO required — the binary is fully self-contained.

```ini
[Unit]
Description=tgwatchspam bot
After=network.target

[Service]
WorkingDirectory=/opt/tgwatchspam
ExecStart=/opt/tgwatchspam/tgwatchspam
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
scp tgwatchspam-linux .env user@yourserver:/opt/tgwatchspam/tgwatchspam
systemctl enable --now tgwatchspam
```

Pre-built binaries for all platforms are available on the [Releases](https://github.com/sqerison/tgwatchspam/releases) page.

## Command Reference

All commands use the `/tgwatch_` prefix to avoid conflicts with other bots in the same group.

### Word & Regex Filters

| Command | Who | Description |
|---------|-----|-------------|
| `/tgwatch_add word <text>` | Admin | Add a blocked word or phrase |
| `/tgwatch_add word` + lines | Admin | Bulk add — one word per line after the command |
| `/tgwatch_remove word <text>` | Admin | Remove a word |
| `/tgwatch_list words` | Admin | List all blocked words |
| `/tgwatch_clear words` | Admin | Remove all words (asks YES) |
| `/tgwatch_add regex <pattern>` | Admin | Add a RE2 regex pattern |
| `/tgwatch_remove regex <pattern>` | Admin | Remove a regex pattern |
| `/tgwatch_list regex` | Admin | List all regex patterns |
| `/tgwatch_clear regex` | Admin | Remove all patterns (asks YES) |

### Spam Actions

| Command | Who | Description |
|---------|-----|-------------|
| `/tgwatch_set action delete\|mute\|ban\|kick` | Admin | What to do when spam is detected (default: ban) |
| `/tgwatch_set mute_duration <hours>` | Admin | How long to mute (used when action=mute) |
| `/tgwatch_set notify_delete <min>\|off` | Admin | Auto-delete the "Spam removed" notification after N minutes (max 2880 = 48h, Telegram's limit). Default: off (kept) |

### New Member Controls

| Command | Who | Description |
|---------|-----|-------------|
| `/tgwatch_set verification on\|off` | Admin | Require new members to tap a button before posting |
| `/tgwatch_set verification_timeout <min>` | Admin | Minutes to verify before auto-kick (default: 2) |
| `/tgwatch_set welcome <text>\|off` | Admin | Custom greeting for new members. Placeholders: `{name}`, `{timeout}` |
| `/tgwatch_set rules <text>\|off` | Admin | Chat rules shown as a tap-to-expand quote; the button becomes "I accept the rules" |
| `/tgwatch_set rules_url <link>\|off` | Admin | Adds a "Chat rules" link button (full article, pinned post, etc.) |
| `/tgwatch_set sandbox on\|off` | Admin | Restrict new members to text-only (no media/links) |
| `/tgwatch_set sandbox_duration <hours>` | Admin | How long sandbox lasts (default: 24h) |
| `/tgwatch_set name_filter on\|off` | Admin | Kick users whose name/username matches spam filters on join |
| `/tgwatch_unrestrict` | Admin | Reply to a message to manually lift sandbox on that user |

Welcome text and rules are shown in the verification message, so they need `verification on`. Both accept the HTML tags `<b>`, `<i>` and `<a href>`; rules may span several lines (max 3000 characters, welcome max 600). After each change the bot posts a preview and saves only if Telegram accepts the text.

### AI Spam Analysis

Needs `AI_PROVIDER` + `AI_API_KEY` (see Setup). The AI only sees messages that **passed** the word/regex filter; it answers `ok`, `spam`, `scam` (deposits, "write me in DM", cash-for-card…) or `rule` (breaks a chat rule). The chat rules from `/tgwatch_set rules` are part of every prompt, together with up to 20 messages admins already labelled in this chat.

| Command | Who | Description |
|---------|-----|-------------|
| `/tgwatch_analyze` | Admin | Reply to a message — the AI classifies it and posts a review request here |
| `/tgwatch_set ai_scan off\|review\|warn\|delete` | Admin | Automatic scan (default off). `review`: message stays, admins decide. `warn`: short warning reply. `delete`: message deleted, admins confirm afterwards |
| `/tgwatch_set review_to chat\|dm @admin…\|group <id>` | Admin | Where review requests go (one at a time, default `chat`). `dm` logins must be admins of the group and must press Start in the bot once. `group` = any group the bot is in, even one with just you |
| `/tgwatch_set ai_threshold <50–100>` | Admin | Minimum model confidence in % before the bot acts (default 80) |
| `/tgwatch_set ai_limit <n>` | Admin | Automatic checks per chat per day (default 200); admins are told once when it runs out |
| `/tgwatch_set ai_scope all\|new` | Admin | Check everyone, or only members who joined in the last 30 days |
| `/tgwatch_set ai_notes <text>\|off` | Admin | Instructions for the AI only, e.g. "short-term rental requests are `rule`" |

A review request shows the verdict, confidence, reason and the message, with three buttons: **⛔ Spam** (delete + the chat's spam action), **🗑 Delete only** (no penalty) and **✅ Not spam**. Only admins of the source chat can press them; the first admin to decide resolves every copy. Every decision is saved as a labelled example that goes into future prompts — "Not spam" examples teach the model what normal messages in this chat look like.

After **⛔ Spam** the bot suggests words/phrases for the filter as toggle buttons. **Nothing is selected or added automatically**; each suggestion has a risk marker (🟢 rare, 🟡 sometimes, 🔴 common) and is checked against the messages admins marked "Not spam" — a phrase that would have hit one of them is shown as 🔴 with the number of false hits. Adding asks for one more confirmation. Requests nobody answers expire after 24 hours.

### Management

| Command | Who | Description |
|---------|-----|-------------|
| `/tgwatch_show settings` | Admin | Show all current settings for this chat |
| `/tgwatch_check <text>` | Admin | Test text against filters without taking action |
| `/tgwatch_log [n]` | Admin | Show last N spam log entries (default 10, max 50) |
| `/tgwatch_log clear` | Admin | Clear spam log (asks YES) |
| `/tgwatch_clean` | Admin | Delete all bot replies and admin commands from chat |
| `/tgwatch_lang` | Admin | Choose bot language for this chat (🇬🇧 English / 🇺🇦 Українська) |
| `/tgwatch_copy <src_id> <dst_id>` | Superadmin | Copy all settings from one chat to another (DM only) |

## Built by AppRecode

tgwatchspam is developed and maintained by **[AppRecode](https://apprecode.com)** — a Cloud & DevOps consulting company with 14+ years of IT experience.

If you need help deploying or customizing this bot, or are looking for related services:

- **[MLOps & AI Infrastructure](https://apprecode.com/services/data-engineering-services)** — ML pipelines, model deployment, RAG systems, LLM infrastructure
- **[AI Security](https://apprecode.com/services/ai-security)** — ML-based threat detection, real-time monitoring, automated compliance
- **[DevOps & Cloud](https://apprecode.com/services/devops-development)** — CI/CD, Kubernetes, managed cloud on AWS & Azure
- **[Managed Cloud Services](https://apprecode.com/services/managed-cloud-services)** — 24/7 infrastructure management and support

→ [apprecode.com](https://apprecode.com)
