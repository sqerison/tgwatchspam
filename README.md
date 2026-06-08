# tgwatchspam

Telegram spam filter bot for Ukrainian community groups. Written in Go, self-hosted, zero external dependencies.

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

## Deployment (systemd)

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
cp tgwatchspam .env /opt/tgwatchspam/
systemctl enable --now tgwatchspam
```

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

### New Member Controls

| Command | Who | Description |
|---------|-----|-------------|
| `/tgwatch_set verification on\|off` | Admin | Require new members to tap a button before posting |
| `/tgwatch_set verification_timeout <min>` | Admin | Minutes to verify before auto-kick (default: 2) |
| `/tgwatch_set sandbox on\|off` | Admin | Restrict new members to text-only (no media/links) |
| `/tgwatch_set sandbox_duration <hours>` | Admin | How long sandbox lasts (default: 24h) |
| `/tgwatch_set name_filter on\|off` | Admin | Kick users whose name/username matches spam filters on join |
| `/tgwatch_unrestrict` | Admin | Reply to a message to manually lift sandbox on that user |

### Management

| Command | Who | Description |
|---------|-----|-------------|
| `/tgwatch_show settings` | Admin | Show all current settings for this chat |
| `/tgwatch_check <text>` | Admin | Test text against filters without taking action |
| `/tgwatch_log [n]` | Admin | Show last N spam log entries (default 10, max 50) |
| `/tgwatch_log clear` | Admin | Clear spam log (asks YES) |
| `/tgwatch_clean` | Admin | Delete all bot replies and admin commands from chat |
| `/tgwatch_copy <src_id> <dst_id>` | Superadmin | Copy all settings from one chat to another (DM only) |
