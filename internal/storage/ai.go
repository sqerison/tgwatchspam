package storage

import (
	"database/sql"
	"encoding/json"
	"time"
)

// --- AI settings [FEAT-024, FEAT-025] ---

func (s *Storage) setColumn(chatID int64, query string, value interface{}) error {
	if err := s.ensureSettings(chatID); err != nil {
		return err
	}
	_, err := s.db.Exec(query, value, chatID)
	return err
}

func (s *Storage) SetAIScan(chatID int64, mode string) error {
	return s.setColumn(chatID, `UPDATE settings SET ai_scan = ? WHERE chat_id = ?`, mode)
}

func (s *Storage) SetAIThreshold(chatID int64, percent int) error {
	return s.setColumn(chatID, `UPDATE settings SET ai_threshold = ? WHERE chat_id = ?`, percent)
}

func (s *Storage) SetAIDailyLimit(chatID int64, limit int) error {
	return s.setColumn(chatID, `UPDATE settings SET ai_daily_limit = ? WHERE chat_id = ?`, limit)
}

func (s *Storage) SetAIScope(chatID int64, scope string) error {
	return s.setColumn(chatID, `UPDATE settings SET ai_scope = ? WHERE chat_id = ?`, scope)
}

func (s *Storage) SetAINotes(chatID int64, notes string) error {
	return s.setColumn(chatID, `UPDATE settings SET ai_notes = ? WHERE chat_id = ?`, notes)
}

func (s *Storage) SetReviewTo(chatID int64, to, target string) error {
	if err := s.ensureSettings(chatID); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE settings SET review_to = ?, review_target = ? WHERE chat_id = ?`, to, target, chatID)
	return err
}

// --- Member join times, for ai_scope=new [FEAT-024] ---

func (s *Storage) RecordJoin(chatID, userID int64, at time.Time) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO members (chat_id, user_id, joined_at) VALUES (?, ?, ?)`,
		chatID, userID, at.Unix(),
	)
	return err
}

// JoinedAt returns when the bot saw the user join, or ok=false if it never did.
func (s *Storage) JoinedAt(chatID, userID int64) (time.Time, bool) {
	var ts int64
	err := s.db.QueryRow(`SELECT joined_at FROM members WHERE chat_id = ? AND user_id = ?`, chatID, userID).Scan(&ts)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(ts, 0), true
}

// --- Daily AI budget [FEAT-024] ---

// ConsumeAICall takes one call from today's budget. ok=false means the budget is spent;
// firstRefusal is true only for the first refused call of the day, so admins are told once.
func (s *Storage) ConsumeAICall(chatID int64, limit int) (ok, firstRefusal bool, err error) {
	day := time.Now().UTC().Format("2006-01-02")
	if _, err = s.db.Exec(`INSERT OR IGNORE INTO ai_usage (chat_id, day) VALUES (?, ?)`, chatID, day); err != nil {
		return false, false, err
	}
	res, err := s.db.Exec(
		`UPDATE ai_usage SET count = count + 1 WHERE chat_id = ? AND day = ? AND count < ?`,
		chatID, day, limit,
	)
	if err != nil {
		return false, false, err
	}
	if n, _ := res.RowsAffected(); n == 1 {
		return true, false, nil
	}
	res, err = s.db.Exec(`UPDATE ai_usage SET notified = 1 WHERE chat_id = ? AND day = ? AND notified = 0`, chatID, day)
	if err != nil {
		return false, false, err
	}
	n, _ := res.RowsAffected()
	return false, n == 1, nil
}

// AIUsageToday returns how many automatic AI checks the chat used today (UTC).
func (s *Storage) AIUsageToday(chatID int64) int {
	var n int
	day := time.Now().UTC().Format("2006-01-02")
	_ = s.db.QueryRow(`SELECT count FROM ai_usage WHERE chat_id = ? AND day = ?`, chatID, day).Scan(&n)
	return n
}

// --- Labelled corpus [FEAT-019, FEAT-023] ---

// FlaggedMessage is a message an admin labelled: spam, scam, rule or ok.
type FlaggedMessage struct {
	ChatID    int64
	UserID    int64
	Username  string
	Text      string
	Label     string
	Source    string // scan | analyze
	CreatedAt time.Time
}

func (s *Storage) AddFlagged(m FlaggedMessage) error {
	_, err := s.db.Exec(
		`INSERT INTO flagged_messages (chat_id, user_id, username, text, label, source, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		m.ChatID, m.UserID, m.Username, m.Text, m.Label, m.Source, m.CreatedAt.Unix(),
	)
	return err
}

// GetFlaggedTexts returns the newest texts with one of the given labels.
func (s *Storage) GetFlaggedTexts(chatID int64, limit int, labels ...string) ([]FlaggedMessage, error) {
	if len(labels) == 0 {
		return nil, nil
	}
	args := []interface{}{chatID}
	placeholders := ""
	for i, l := range labels {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args = append(args, l)
	}
	args = append(args, limit)
	rows, err := s.db.Query(
		`SELECT text, label FROM flagged_messages WHERE chat_id = ? AND label IN (`+placeholders+`) ORDER BY created_at DESC, id DESC LIMIT ?`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FlaggedMessage
	for rows.Next() {
		m := FlaggedMessage{ChatID: chatID}
		if err := rows.Scan(&m.Text, &m.Label); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// GetRecentSpamTexts returns texts the keyword filter blocked, newest first.
func (s *Storage) GetRecentSpamTexts(chatID int64, limit int) ([]string, error) {
	rows, err := s.db.Query(`SELECT message_text FROM spam_log WHERE chat_id = ? ORDER BY created_at DESC LIMIT ?`, chatID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// --- Review requests [FEAT-025] ---

// ReviewKeyword is one suggested filter word with its risk and false-hit count. [FEAT-022]
type ReviewKeyword struct {
	Text      string `json:"text"`
	Risk      string `json:"risk"`
	FalseHits int    `json:"false_hits"`
}

// MessageRef points at one copy of a review request.
type MessageRef struct {
	ChatID    int64 `json:"chat_id"`
	MessageID int   `json:"message_id"`
}

// Review is a suspicious message waiting for (or past) an admin decision.
type Review struct {
	ID             int64
	ChatID         int64
	MessageID      int
	UserID         int64
	Username       string
	DisplayName    string
	Text           string
	Link           string
	Verdict        string
	Confidence     float64
	Reason         string
	Rule           string
	Keywords       []ReviewKeyword
	Source         string // scan | analyze
	MessageDeleted bool
	Status         string // open | spam | rule | ok | expired
	Selected       int    // bitmask of keywords toggled on
	KeywordsDone   bool
	Copies         []MessageRef
	DecidedBy      string
	CreatedAt      time.Time
	ExpiresAt      time.Time
}

func (s *Storage) CreateReview(r *Review) error {
	kw, _ := json.Marshal(r.Keywords)
	res, err := s.db.Exec(
		`INSERT INTO pending_reviews (chat_id, message_id, user_id, username, display_name, text, link, verdict, confidence,
		 reason, rule, keywords, source, message_deleted, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ChatID, r.MessageID, r.UserID, r.Username, r.DisplayName, r.Text, r.Link, r.Verdict, r.Confidence,
		r.Reason, r.Rule, string(kw), r.Source, boolInt(r.MessageDeleted), r.CreatedAt.Unix(), r.ExpiresAt.Unix(),
	)
	if err != nil {
		return err
	}
	r.ID, err = res.LastInsertId()
	r.Status = "open"
	return err
}

const reviewColumns = `id, chat_id, message_id, user_id, username, display_name, text, link, verdict, confidence, reason, rule,
	keywords, source, message_deleted, status, selected, keywords_done, copies, decided_by, created_at, expires_at`

func scanReview(row interface{ Scan(...interface{}) error }) (*Review, error) {
	var r Review
	var kw, copies string
	var deleted, done int
	var created, expires int64
	if err := row.Scan(&r.ID, &r.ChatID, &r.MessageID, &r.UserID, &r.Username, &r.DisplayName, &r.Text, &r.Link,
		&r.Verdict, &r.Confidence, &r.Reason, &r.Rule, &kw, &r.Source, &deleted, &r.Status, &r.Selected, &done,
		&copies, &r.DecidedBy, &created, &expires); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(kw), &r.Keywords)
	_ = json.Unmarshal([]byte(copies), &r.Copies)
	r.MessageDeleted = deleted != 0
	r.KeywordsDone = done != 0
	r.CreatedAt = time.Unix(created, 0)
	r.ExpiresAt = time.Unix(expires, 0)
	return &r, nil
}

// GetReview returns the review, or nil if it does not exist.
func (s *Storage) GetReview(id int64) (*Review, error) {
	r, err := scanReview(s.db.QueryRow(`SELECT `+reviewColumns+` FROM pending_reviews WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return r, err
}

func (s *Storage) SetReviewCopies(id int64, copies []MessageRef) error {
	data, _ := json.Marshal(copies)
	_, err := s.db.Exec(`UPDATE pending_reviews SET copies = ? WHERE id = ?`, string(data), id)
	return err
}

// DecideReview moves an open review to status. It returns false if another admin decided first.
func (s *Storage) DecideReview(id int64, status, decidedBy string) (bool, error) {
	res, err := s.db.Exec(
		`UPDATE pending_reviews SET status = ?, decided_by = ? WHERE id = ? AND status = 'open'`,
		status, decidedBy, id,
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

func (s *Storage) SetReviewSelected(id int64, mask int) error {
	_, err := s.db.Exec(`UPDATE pending_reviews SET selected = ? WHERE id = ?`, mask, id)
	return err
}

// FinishReviewKeywords marks the keyword step done. It returns false if it was already done,
// so a double tap cannot add the words twice.
func (s *Storage) FinishReviewKeywords(id int64) (bool, error) {
	res, err := s.db.Exec(`UPDATE pending_reviews SET keywords_done = 1 WHERE id = ? AND keywords_done = 0`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// ExpireReviews marks open reviews past their deadline as expired and returns them.
func (s *Storage) ExpireReviews() ([]*Review, error) {
	now := time.Now().Unix()
	rows, err := s.db.Query(`SELECT `+reviewColumns+` FROM pending_reviews WHERE status = 'open' AND expires_at <= ?`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Review
	for rows.Next() {
		r, err := scanReview(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	_, err = s.db.Exec(`UPDATE pending_reviews SET status = 'expired' WHERE status = 'open' AND expires_at <= ?`, now)
	return out, err
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
