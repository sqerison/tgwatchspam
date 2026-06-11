package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Storage struct {
	db *sql.DB
}

// Settings holds per-chat configuration [FEAT-006]
type Settings struct {
	ChatID               int64
	Action               string
	MuteDurationH        int
	SandboxEnabled       bool
	SandboxHours         int
	NameFilter           bool
	VerificationEnabled  bool   // [FEAT-015]
	VerificationTimeoutM int    // [FEAT-015]
	Language             string // [FEAT-016]
}

// PendingVerification tracks a new member awaiting button verification. [FEAT-015]
type PendingVerification struct {
	ChatID    int64
	UserID    int64
	MessageID int // bot's verification message to delete after outcome
	ExpiresAt time.Time
}

// SpamEntry is a single spam log record [FEAT-009]
type SpamEntry struct {
	ID          int64
	ChatID      int64
	UserID      int64
	Username    string
	MatchedRule string
	MessageText string
	ActionTaken string
	CreatedAt   time.Time
}

// PendingUnrestrict tracks scheduled unrestrictions (sandbox + mute) [FEAT-007]
type PendingUnrestrict struct {
	ChatID int64
	UserID int64
	Until  time.Time
	Reason string
}

func New(path string) (*Storage, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// SQLite does not support concurrent writes
	db.SetMaxOpenConns(1)

	s := &Storage{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Storage) Close() error {
	return s.db.Close()
}

func (s *Storage) migrate() error {
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS words (
			id      INTEGER PRIMARY KEY AUTOINCREMENT,
			chat_id INTEGER NOT NULL,
			word    TEXT    NOT NULL,
			UNIQUE(chat_id, word)
		);
		CREATE TABLE IF NOT EXISTS regexes (
			id      INTEGER PRIMARY KEY AUTOINCREMENT,
			chat_id INTEGER NOT NULL,
			pattern TEXT    NOT NULL,
			UNIQUE(chat_id, pattern)
		);
		CREATE TABLE IF NOT EXISTS settings (
			chat_id               INTEGER PRIMARY KEY,
			action                TEXT    NOT NULL DEFAULT 'ban',
			mute_duration_h       INTEGER NOT NULL DEFAULT 24,
			sandbox_enabled       INTEGER NOT NULL DEFAULT 0,
			sandbox_hours         INTEGER NOT NULL DEFAULT 24,
			name_filter           INTEGER NOT NULL DEFAULT 1,
			verification_enabled  INTEGER NOT NULL DEFAULT 0,
			verification_timeout_m INTEGER NOT NULL DEFAULT 2
		);
		CREATE TABLE IF NOT EXISTS spam_log (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			chat_id      INTEGER NOT NULL,
			user_id      INTEGER NOT NULL,
			username     TEXT,
			matched_rule TEXT    NOT NULL,
			message_text TEXT    NOT NULL,
			action_taken TEXT    NOT NULL,
			created_at   INTEGER NOT NULL
		);
		CREATE TABLE IF NOT EXISTS pending_unrestricts (
			chat_id  INTEGER NOT NULL,
			user_id  INTEGER NOT NULL,
			until_ts INTEGER NOT NULL,
			reason   TEXT    NOT NULL,
			PRIMARY KEY (chat_id, user_id)
		);
		CREATE TABLE IF NOT EXISTS tracked_messages (
			chat_id    INTEGER NOT NULL,
			message_id INTEGER NOT NULL,
			PRIMARY KEY (chat_id, message_id)
		);
		CREATE TABLE IF NOT EXISTS pending_verifications (
			chat_id    INTEGER NOT NULL,
			user_id    INTEGER NOT NULL,
			message_id INTEGER NOT NULL,
			expires_at INTEGER NOT NULL,
			PRIMARY KEY (chat_id, user_id)
		);
	`); err != nil {
		return err
	}

	// Add new columns to existing settings tables (idempotent). [FEAT-015, FEAT-016]
	for _, stmt := range []string{
		`ALTER TABLE settings ADD COLUMN verification_enabled INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE settings ADD COLUMN verification_timeout_m INTEGER NOT NULL DEFAULT 2`,
		`ALTER TABLE settings ADD COLUMN language TEXT NOT NULL DEFAULT 'en'`,
	} {
		if _, err := s.db.Exec(stmt); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			return err
		}
	}
	return nil
}

func (s *Storage) ensureSettings(chatID int64) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO settings (chat_id) VALUES (?)`, chatID)
	return err
}

// --- Words [FEAT-001] ---

// AddWords inserts normalized words, returning counts of added and skipped duplicates.
func (s *Storage) AddWords(chatID int64, words []string) (added, skipped int, err error) {
	for _, w := range words {
		res, e := s.db.Exec(`INSERT OR IGNORE INTO words (chat_id, word) VALUES (?, ?)`, chatID, w)
		if e != nil {
			return added, skipped, e
		}
		n, _ := res.RowsAffected()
		if n > 0 {
			added++
		} else {
			skipped++
		}
	}
	return
}

func (s *Storage) RemoveWord(chatID int64, word string) (bool, error) {
	res, err := s.db.Exec(`DELETE FROM words WHERE chat_id = ? AND word = ?`, chatID, word)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (s *Storage) GetWords(chatID int64) ([]string, error) {
	rows, err := s.db.Query(`SELECT word FROM words WHERE chat_id = ? ORDER BY word`, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var words []string
	for rows.Next() {
		var w string
		if err := rows.Scan(&w); err != nil {
			return nil, err
		}
		words = append(words, w)
	}
	return words, rows.Err()
}

func (s *Storage) ClearWords(chatID int64) error {
	_, err := s.db.Exec(`DELETE FROM words WHERE chat_id = ?`, chatID)
	return err
}

// --- Regexes [FEAT-002] ---

func (s *Storage) AddRegex(chatID int64, pattern string) (bool, error) {
	res, err := s.db.Exec(`INSERT OR IGNORE INTO regexes (chat_id, pattern) VALUES (?, ?)`, chatID, pattern)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (s *Storage) RemoveRegex(chatID int64, pattern string) (bool, error) {
	res, err := s.db.Exec(`DELETE FROM regexes WHERE chat_id = ? AND pattern = ?`, chatID, pattern)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (s *Storage) GetRegexes(chatID int64) ([]string, error) {
	rows, err := s.db.Query(`SELECT pattern FROM regexes WHERE chat_id = ? ORDER BY id`, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var patterns []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		patterns = append(patterns, p)
	}
	return patterns, rows.Err()
}

func (s *Storage) ClearRegexes(chatID int64) error {
	_, err := s.db.Exec(`DELETE FROM regexes WHERE chat_id = ?`, chatID)
	return err
}

// --- Settings [FEAT-006] ---

func (s *Storage) GetSettings(chatID int64) (*Settings, error) {
	if err := s.ensureSettings(chatID); err != nil {
		return nil, err
	}
	row := s.db.QueryRow(
		`SELECT action, mute_duration_h, sandbox_enabled, sandbox_hours, name_filter, verification_enabled, verification_timeout_m, language
		 FROM settings WHERE chat_id = ?`,
		chatID,
	)
	var st Settings
	st.ChatID = chatID
	var sandboxEnabled, nameFilter, verificationEnabled int
	if err := row.Scan(&st.Action, &st.MuteDurationH, &sandboxEnabled, &st.SandboxHours, &nameFilter, &verificationEnabled, &st.VerificationTimeoutM, &st.Language); err != nil {
		return nil, err
	}
	st.SandboxEnabled = sandboxEnabled != 0
	st.NameFilter = nameFilter != 0
	st.VerificationEnabled = verificationEnabled != 0
	if st.Language == "" {
		st.Language = "en"
	}
	return &st, nil
}

// GetLanguage returns the configured language for a chat, defaulting to "en". [FEAT-016]
func (s *Storage) GetLanguage(chatID int64) string {
	if err := s.ensureSettings(chatID); err != nil {
		return "en"
	}
	var lang string
	if err := s.db.QueryRow(`SELECT language FROM settings WHERE chat_id = ?`, chatID).Scan(&lang); err != nil || lang == "" {
		return "en"
	}
	return lang
}

// SetLanguage persists the language preference for a chat. [FEAT-016]
func (s *Storage) SetLanguage(chatID int64, lang string) error {
	if err := s.ensureSettings(chatID); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE settings SET language = ? WHERE chat_id = ?`, lang, chatID)
	return err
}

func (s *Storage) SetAction(chatID int64, action string) error {
	if err := s.ensureSettings(chatID); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE settings SET action = ? WHERE chat_id = ?`, action, chatID)
	return err
}

func (s *Storage) SetMuteDuration(chatID int64, hours int) error {
	if err := s.ensureSettings(chatID); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE settings SET mute_duration_h = ? WHERE chat_id = ?`, hours, chatID)
	return err
}

func (s *Storage) SetSandbox(chatID int64, enabled bool) error {
	if err := s.ensureSettings(chatID); err != nil {
		return err
	}
	v := 0
	if enabled {
		v = 1
	}
	_, err := s.db.Exec(`UPDATE settings SET sandbox_enabled = ? WHERE chat_id = ?`, v, chatID)
	return err
}

func (s *Storage) SetSandboxDuration(chatID int64, hours int) error {
	if err := s.ensureSettings(chatID); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE settings SET sandbox_hours = ? WHERE chat_id = ?`, hours, chatID)
	return err
}

func (s *Storage) SetNameFilter(chatID int64, enabled bool) error {
	if err := s.ensureSettings(chatID); err != nil {
		return err
	}
	v := 0
	if enabled {
		v = 1
	}
	_, err := s.db.Exec(`UPDATE settings SET name_filter = ? WHERE chat_id = ?`, v, chatID)
	return err
}

// --- Spam log [FEAT-009] ---

func (s *Storage) LogSpam(entry SpamEntry) error {
	_, err := s.db.Exec(
		`INSERT INTO spam_log (chat_id, user_id, username, matched_rule, message_text, action_taken, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		entry.ChatID, entry.UserID, entry.Username, entry.MatchedRule, entry.MessageText, entry.ActionTaken, entry.CreatedAt.Unix(),
	)
	return err
}

func (s *Storage) GetSpamLog(chatID int64, limit int) ([]SpamEntry, error) {
	rows, err := s.db.Query(
		`SELECT id, user_id, username, matched_rule, message_text, action_taken, created_at FROM spam_log WHERE chat_id = ? ORDER BY created_at DESC LIMIT ?`,
		chatID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []SpamEntry
	for rows.Next() {
		var e SpamEntry
		var ts int64
		if err := rows.Scan(&e.ID, &e.UserID, &e.Username, &e.MatchedRule, &e.MessageText, &e.ActionTaken, &ts); err != nil {
			return nil, err
		}
		e.ChatID = chatID
		e.CreatedAt = time.Unix(ts, 0)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (s *Storage) ClearSpamLog(chatID int64) error {
	_, err := s.db.Exec(`DELETE FROM spam_log WHERE chat_id = ?`, chatID)
	return err
}

// --- Pending unrestricts (sandbox + mute timers) [FEAT-007] ---

func (s *Storage) AddPendingUnrestrict(chatID, userID int64, until time.Time, reason string) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO pending_unrestricts (chat_id, user_id, until_ts, reason) VALUES (?, ?, ?, ?)`,
		chatID, userID, until.Unix(), reason,
	)
	return err
}

func (s *Storage) RemovePendingUnrestrict(chatID, userID int64) error {
	_, err := s.db.Exec(`DELETE FROM pending_unrestricts WHERE chat_id = ? AND user_id = ?`, chatID, userID)
	return err
}

func (s *Storage) GetExpiredUnrestricts() ([]PendingUnrestrict, error) {
	rows, err := s.db.Query(
		`SELECT chat_id, user_id, until_ts, reason FROM pending_unrestricts WHERE until_ts <= ?`,
		time.Now().Unix(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []PendingUnrestrict
	for rows.Next() {
		var p PendingUnrestrict
		var ts int64
		if err := rows.Scan(&p.ChatID, &p.UserID, &ts, &p.Reason); err != nil {
			return nil, err
		}
		p.Until = time.Unix(ts, 0)
		items = append(items, p)
	}
	return items, rows.Err()
}

// --- Copy between chats [FEAT-012] ---

func (s *Storage) CopySettings(srcChatID, dstChatID int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM words WHERE chat_id = ?`, dstChatID); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO words (chat_id, word) SELECT ?, word FROM words WHERE chat_id = ?`,
		dstChatID, srcChatID,
	); err != nil {
		return err
	}

	if _, err := tx.Exec(`DELETE FROM regexes WHERE chat_id = ?`, dstChatID); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO regexes (chat_id, pattern) SELECT ?, pattern FROM regexes WHERE chat_id = ?`,
		dstChatID, srcChatID,
	); err != nil {
		return err
	}

	if _, err := tx.Exec(`DELETE FROM settings WHERE chat_id = ?`, dstChatID); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO settings (chat_id, action, mute_duration_h, sandbox_enabled, sandbox_hours, name_filter)
		 SELECT ?, action, mute_duration_h, sandbox_enabled, sandbox_hours, name_filter FROM settings WHERE chat_id = ?`,
		dstChatID, srcChatID,
	); err != nil {
		return err
	}

	return tx.Commit()
}

// --- Tracked messages for /clean [FEAT-013] ---

// TrackMessage records a bot response or admin command message ID for later cleanup.
func (s *Storage) TrackMessage(chatID int64, messageID int) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO tracked_messages (chat_id, message_id) VALUES (?, ?)`,
		chatID, messageID,
	)
	return err
}

// PopTrackedMessages returns all tracked message IDs for a chat and deletes them atomically.
func (s *Storage) PopTrackedMessages(chatID int64) ([]int, error) {
	rows, err := s.db.Query(`SELECT message_id FROM tracked_messages WHERE chat_id = ?`, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	_, err = s.db.Exec(`DELETE FROM tracked_messages WHERE chat_id = ?`, chatID)
	return ids, err
}

// --- Verification settings [FEAT-015] ---

func (s *Storage) SetVerification(chatID int64, enabled bool) error {
	if err := s.ensureSettings(chatID); err != nil {
		return err
	}
	v := 0
	if enabled {
		v = 1
	}
	_, err := s.db.Exec(`UPDATE settings SET verification_enabled = ? WHERE chat_id = ?`, v, chatID)
	return err
}

func (s *Storage) SetVerificationTimeout(chatID int64, minutes int) error {
	if err := s.ensureSettings(chatID); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE settings SET verification_timeout_m = ? WHERE chat_id = ?`, minutes, chatID)
	return err
}

// --- Pending verifications [FEAT-015] ---

func (s *Storage) AddPendingVerification(chatID, userID int64, messageID int, expiresAt time.Time) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO pending_verifications (chat_id, user_id, message_id, expires_at) VALUES (?, ?, ?, ?)`,
		chatID, userID, messageID, expiresAt.Unix(),
	)
	return err
}

// RemovePendingVerification deletes the record and returns the stored message ID (to delete the bot message).
func (s *Storage) RemovePendingVerification(chatID, userID int64) (int, error) {
	var messageID int
	err := s.db.QueryRow(
		`SELECT message_id FROM pending_verifications WHERE chat_id = ? AND user_id = ?`,
		chatID, userID,
	).Scan(&messageID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	_, err = s.db.Exec(`DELETE FROM pending_verifications WHERE chat_id = ? AND user_id = ?`, chatID, userID)
	return messageID, err
}

func (s *Storage) GetExpiredVerifications() ([]PendingVerification, error) {
	rows, err := s.db.Query(
		`SELECT chat_id, user_id, message_id, expires_at FROM pending_verifications WHERE expires_at <= ?`,
		time.Now().Unix(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []PendingVerification
	for rows.Next() {
		var p PendingVerification
		var ts int64
		if err := rows.Scan(&p.ChatID, &p.UserID, &p.MessageID, &ts); err != nil {
			return nil, err
		}
		p.ExpiresAt = time.Unix(ts, 0)
		items = append(items, p)
	}
	return items, rows.Err()
}
