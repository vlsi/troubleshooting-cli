package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vlsi/troubleshooting-cli/internal/domain"
)

func tempDB(t *testing.T) *SQLiteStore {
	t.Helper()
	dir := t.TempDir()
	store, err := NewSQLiteStore(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestSessionRoundTrip(t *testing.T) {
	store := tempDB(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	sess := domain.Session{
		ID:          "s1",
		Title:       "test session",
		Service:     "api",
		Environment: "prod",
		Status:      domain.SessionOpen,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := store.CreateSession(sess); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := store.GetSession("s1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "test session" {
		t.Errorf("title mismatch: %s", got.Title)
	}
	if got.Status != domain.SessionOpen {
		t.Errorf("status mismatch: %s", got.Status)
	}
}

func TestSessionUpdate(t *testing.T) {
	store := tempDB(t)
	now := time.Now().UTC()
	sess := domain.Session{ID: "s1", Title: "t", Status: domain.SessionOpen, CreatedAt: now, UpdatedAt: now}
	store.CreateSession(sess)
	sess.Status = domain.SessionResolved
	store.UpdateSession(sess)
	got, _ := store.GetSession("s1")
	if got.Status != domain.SessionResolved {
		t.Errorf("expected resolved, got %s", got.Status)
	}
}

func TestFindingRoundTrip(t *testing.T) {
	store := tempDB(t)
	f := domain.Finding{
		ID:        "f1",
		SessionID: "s1",
		CreatedAt: time.Now().UTC(),
		Kind:      "observation",
		Summary:   "high latency",
	}
	if err := store.AddFinding(f); err != nil {
		t.Fatalf("add: %v", err)
	}
	findings, err := store.ListFindings("s1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(findings) != 1 || findings[0].Summary != "high latency" {
		t.Error("finding mismatch")
	}
}

func TestHypothesisRoundTrip(t *testing.T) {
	store := tempDB(t)
	now := time.Now().UTC()
	h := domain.Hypothesis{
		ID:        "h1",
		SessionID: "s1",
		CreatedAt: now,
		UpdatedAt: now,
		Statement: "pool exhaustion",
		Status:    domain.HypothesisOpen,
	}
	if err := store.AddHypothesis(h); err != nil {
		t.Fatalf("add: %v", err)
	}
	got, err := store.GetHypothesis("h1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Statement != "pool exhaustion" {
		t.Error("statement mismatch")
	}
	got.Status = domain.HypothesisSupported
	store.UpdateHypothesis(got)
	got2, _ := store.GetHypothesis("h1")
	if got2.Status != domain.HypothesisSupported {
		t.Error("update failed")
	}
}

func TestTimelineOrdering(t *testing.T) {
	store := tempDB(t)
	base := time.Now().UTC()
	for i, kind := range []string{"session_started", "finding_added", "hypothesis_added"} {
		store.AddTimelineEvent(domain.TimelineEvent{
			ID:        kind,
			SessionID: "s1",
			Timestamp: base.Add(time.Duration(i) * time.Second),
			Kind:      kind,
			Summary:   kind,
		})
	}
	events, err := store.ListTimeline("s1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[0].Kind != "session_started" {
		t.Error("expected session_started first")
	}
	if events[2].Kind != "hypothesis_added" {
		t.Error("expected hypothesis_added last")
	}
}

func TestDefaultDBPath(t *testing.T) {
	p, err := DefaultDBPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filepath.Base(p) != "sessions.db" {
		t.Errorf("unexpected db name: %s", p)
	}
	dir := filepath.Dir(p)
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory")
	}
}
