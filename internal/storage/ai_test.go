package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func newTestStorage(t *testing.T) *Storage {
	t.Helper()
	st, err := New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestAISettingsDefaultsAndCopy(t *testing.T) {
	st := newTestStorage(t)
	s, err := st.GetSettings(1)
	if err != nil {
		t.Fatal(err)
	}
	if s.AIScan != "off" || s.AIThreshold != 80 || s.AIDailyLimit != 200 || s.AIScope != "all" || s.ReviewTo != "chat" {
		t.Fatalf("defaults: %+v", s)
	}
	st.SetAIScan(1, "review")
	st.SetAINotes(1, "notes")
	st.SetReviewTo(1, "dm", "10,20")
	if err := st.CopySettings(1, 2); err != nil {
		t.Fatal(err)
	}
	d, _ := st.GetSettings(2)
	if d.AIScan != "review" || d.AINotes != "notes" {
		t.Fatalf("AI settings not copied: %+v", d)
	}
	if d.ReviewTo != "chat" || d.ReviewTarget != "" {
		t.Fatalf("review destination must not be copied: %+v", d)
	}
}

func TestConsumeAICall(t *testing.T) {
	st := newTestStorage(t)
	for i := 0; i < 2; i++ {
		if ok, _, _ := st.ConsumeAICall(1, 2); !ok {
			t.Fatalf("call %d refused", i)
		}
	}
	ok, first, _ := st.ConsumeAICall(1, 2)
	if ok || !first {
		t.Fatalf("third call: ok=%v first=%v", ok, first)
	}
	ok, first, _ = st.ConsumeAICall(1, 2)
	if ok || first {
		t.Fatalf("fourth call: ok=%v first=%v (admins must be told only once)", ok, first)
	}
	if n := st.AIUsageToday(1); n != 2 {
		t.Fatalf("usage = %d", n)
	}
}

func TestReviewLifecycle(t *testing.T) {
	st := newTestStorage(t)
	r := &Review{
		ChatID: 1, MessageID: 5, UserID: 7, Text: "pay deposit", Verdict: "scam", Confidence: 0.9,
		Keywords:  []ReviewKeyword{{Text: "deposit", Risk: "high", FalseHits: 2}},
		Source:    "scan",
		CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := st.CreateReview(r); err != nil {
		t.Fatal(err)
	}
	st.SetReviewCopies(r.ID, []MessageRef{{ChatID: 1, MessageID: 99}})

	won, _ := st.DecideReview(r.ID, "spam", "@a")
	lost, _ := st.DecideReview(r.ID, "ok", "@b")
	if !won || lost {
		t.Fatalf("decide race: won=%v lost=%v", won, lost)
	}
	got, _ := st.GetReview(r.ID)
	if got.Status != "spam" || got.DecidedBy != "@a" || len(got.Copies) != 1 || got.Keywords[0].FalseHits != 2 {
		t.Fatalf("got %+v", got)
	}
	st.SetReviewSelected(r.ID, 1)
	first, _ := st.FinishReviewKeywords(r.ID)
	again, _ := st.FinishReviewKeywords(r.ID)
	if !first || again {
		t.Fatal("keywords could be added twice")
	}

	old := &Review{ChatID: 1, Text: "x", Verdict: "spam", Source: "scan",
		CreatedAt: time.Now().Add(-2 * time.Hour), ExpiresAt: time.Now().Add(-time.Hour)}
	st.CreateReview(old)
	expired, err := st.ExpireReviews()
	if err != nil || len(expired) != 1 || expired[0].ID != old.ID {
		t.Fatalf("expired=%v err=%v", expired, err)
	}
	if missing, _ := st.GetReview(12345); missing != nil {
		t.Fatal("missing review returned")
	}
}

func TestCorpusAndJoins(t *testing.T) {
	st := newTestStorage(t)
	for i, label := range []string{"spam", "ok", "scam", "rule"} {
		st.AddFlagged(FlaggedMessage{ChatID: 1, UserID: 1, Text: label, Label: label, Source: "scan",
			CreatedAt: time.Now().Add(time.Duration(i) * time.Second)})
	}
	spam, _ := st.GetFlaggedTexts(1, 10, "spam", "scam")
	if len(spam) != 2 || spam[0].Label != "scam" {
		t.Fatalf("spam examples: %+v", spam)
	}
	if _, ok := st.JoinedAt(1, 7); ok {
		t.Fatal("unknown member has a join time")
	}
	st.RecordJoin(1, 7, time.Now())
	if _, ok := st.JoinedAt(1, 7); !ok {
		t.Fatal("join not recorded")
	}
}
