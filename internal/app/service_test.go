package app

import (
	"fmt"
	"strings"
	"testing"

	"github.com/vlsi/troubleshooting-cli/internal/domain"
)

// memStore is a minimal in-memory store for testing.
type memStore struct {
	sessions   map[string]domain.Session
	findings   map[string][]domain.Finding
	hypotheses map[string]domain.Hypothesis
	hypBysess  map[string][]string
	timeline   map[string][]domain.TimelineEvent
}

func newMemStore() *memStore {
	return &memStore{
		sessions:   make(map[string]domain.Session),
		findings:   make(map[string][]domain.Finding),
		hypotheses: make(map[string]domain.Hypothesis),
		hypBysess:  make(map[string][]string),
		timeline:   make(map[string][]domain.TimelineEvent),
	}
}

func (m *memStore) CreateSession(s domain.Session) error {
	m.sessions[s.ID] = s
	return nil
}
func (m *memStore) GetSession(id string) (domain.Session, error) {
	s, ok := m.sessions[id]
	if !ok {
		return domain.Session{}, fmt.Errorf("session not found: %s", id)
	}
	return s, nil
}
func (m *memStore) UpdateSession(s domain.Session) error {
	m.sessions[s.ID] = s
	return nil
}
func (m *memStore) AddFinding(f domain.Finding) error {
	m.findings[f.SessionID] = append(m.findings[f.SessionID], f)
	return nil
}
func (m *memStore) ListFindings(sessionID string) ([]domain.Finding, error) {
	return m.findings[sessionID], nil
}
func (m *memStore) AddHypothesis(h domain.Hypothesis) error {
	m.hypotheses[h.ID] = h
	m.hypBysess[h.SessionID] = append(m.hypBysess[h.SessionID], h.ID)
	return nil
}
func (m *memStore) GetHypothesis(id string) (domain.Hypothesis, error) {
	h, ok := m.hypotheses[id]
	if !ok {
		return domain.Hypothesis{}, fmt.Errorf("hypothesis not found: %s", id)
	}
	return h, nil
}
func (m *memStore) UpdateHypothesis(h domain.Hypothesis) error {
	m.hypotheses[h.ID] = h
	return nil
}
func (m *memStore) ListHypotheses(sessionID string) ([]domain.Hypothesis, error) {
	var result []domain.Hypothesis
	for _, id := range m.hypBysess[sessionID] {
		result = append(result, m.hypotheses[id])
	}
	return result, nil
}
func (m *memStore) AddTimelineEvent(e domain.TimelineEvent) error {
	m.timeline[e.SessionID] = append(m.timeline[e.SessionID], e)
	return nil
}
func (m *memStore) ListTimeline(sessionID string) ([]domain.TimelineEvent, error) {
	return m.timeline[sessionID], nil
}

var idCounter int

func testIDGen() string {
	idCounter++
	return fmt.Sprintf("id-%d", idCounter)
}

func setup() *Service {
	idCounter = 0
	return NewService(newMemStore(), testIDGen)
}

func TestStartSessionValidation(t *testing.T) {
	svc := setup()
	tests := []struct {
		name    string
		title   string
		service string
		env     string
	}{
		{"empty title", "", "svc", "dev"},
		{"empty service", "title", "", "dev"},
		{"empty env", "title", "svc", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.StartSession(tc.title, tc.service, tc.env, "", nil)
			if err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestAddFindingValidation(t *testing.T) {
	svc := setup()
	sess, _ := svc.StartSession("test", "svc", "dev", "", nil)
	_, err := svc.AddFinding(sess.ID, "obs", "", "", "", nil, nil)
	if err == nil {
		t.Error("expected error for empty summary")
	}
	_, err = svc.AddFinding("", "obs", "summary", "", "", nil, nil)
	if err == nil {
		t.Error("expected error for empty session_id")
	}
}

func TestAddFindingDefaultKind(t *testing.T) {
	svc := setup()
	sess, _ := svc.StartSession("test", "svc", "dev", "", nil)
	f, err := svc.AddFinding(sess.ID, "", "something", "", "", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Kind != "observation" {
		t.Errorf("expected default kind 'observation', got %q", f.Kind)
	}
}

func TestStartSession(t *testing.T) {
	svc := setup()
	sess, err := svc.StartSession("pod crash", "api-gateway", "prod", "INC-123", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sess.ID == "" {
		t.Error("session ID must not be empty")
	}
	if sess.Status != domain.SessionOpen {
		t.Errorf("expected status open, got %s", sess.Status)
	}
	if sess.Title != "pod crash" {
		t.Errorf("expected title 'pod crash', got %s", sess.Title)
	}
}

func TestAddFindingAndGetState(t *testing.T) {
	svc := setup()
	sess, _ := svc.StartSession("test", "svc", "dev", "", nil)
	_, err := svc.AddFinding(sess.ID, "observation", "high latency on /api", "", "high", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	state, err := svc.GetState(sess.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(state.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(state.Findings))
	}
	if state.Findings[0].Summary != "high latency on /api" {
		t.Error("finding summary mismatch")
	}
}

func TestHypothesisLifecycle(t *testing.T) {
	svc := setup()
	sess, _ := svc.StartSession("test", "svc", "dev", "", nil)
	h, err := svc.AddHypothesis(sess.ID, "DB connection pool exhausted", "high", nil, []string{"check pool metrics"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.Status != domain.HypothesisOpen {
		t.Errorf("expected open status, got %s", h.Status)
	}

	supported := domain.HypothesisSupported
	conf := 0.8
	updated, err := svc.UpdateHypothesis(h.ID, &supported, &conf, []string{"f1"}, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Status != domain.HypothesisSupported {
		t.Error("status not updated")
	}
	if updated.Confidence == nil || *updated.Confidence != 0.8 {
		t.Error("confidence not updated")
	}
	if len(updated.SupportingFindingIDs) != 1 {
		t.Error("supporting findings not updated")
	}
}

func TestRankHypotheses(t *testing.T) {
	svc := setup()
	sess, _ := svc.StartSession("test", "svc", "dev", "", nil)

	high := 0.9
	low := 0.3
	svc.AddHypothesis(sess.ID, "low confidence", "", &low, nil)
	svc.AddHypothesis(sess.ID, "high confidence", "", &high, nil)

	ranked, err := svc.RankHypotheses(sess.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ranked) != 2 {
		t.Fatalf("expected 2 hypotheses, got %d", len(ranked))
	}
	if ranked[0].Statement != "high confidence" {
		t.Error("expected high confidence hypothesis first")
	}
}

func TestRecommendNextSteps(t *testing.T) {
	svc := setup()
	sess, _ := svc.StartSession("test", "svc", "dev", "", nil)
	svc.AddHypothesis(sess.ID, "pool exhaustion", "", nil, []string{"check connection count", "review pool config"})

	recs, err := svc.RecommendNextSteps(sess.ID, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("expected 2 recommendations, got %d", len(recs))
	}
	if recs[0].Action != "check connection count" {
		t.Error("unexpected first recommendation")
	}
}

func TestGenerateSummary(t *testing.T) {
	svc := setup()
	sess, _ := svc.StartSession("pod crash", "api-gw", "prod", "", nil)
	svc.AddFinding(sess.ID, "observation", "OOM kill", "", "high", nil, nil)
	svc.AddHypothesis(sess.ID, "memory leak in handler", "", nil, nil)

	md, err := svc.GenerateSummary(sess.ID, "handoff")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(md, "Handoff") {
		t.Error("expected handoff header")
	}
	if !strings.Contains(md, "OOM kill") {
		t.Error("expected finding in summary")
	}
	if !strings.Contains(md, "memory leak") {
		t.Error("expected hypothesis in summary")
	}
}

func TestCloseSession(t *testing.T) {
	svc := setup()
	sess, _ := svc.StartSession("test", "svc", "dev", "", nil)
	closed, err := svc.CloseSession(sess.ID, domain.SessionResolved, "fixed the pool config")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if closed.Status != domain.SessionResolved {
		t.Error("expected resolved status")
	}
	if closed.ClosedAt == nil {
		t.Error("expected closed_at to be set")
	}
	if closed.Outcome != "fixed the pool config" {
		t.Error("unexpected outcome")
	}
}

func TestGetTimeline(t *testing.T) {
	svc := setup()
	sess, _ := svc.StartSession("test", "svc", "dev", "", nil)
	svc.AddFinding(sess.ID, "obs", "something", "", "", nil, nil)

	timeline, err := svc.GetTimeline(sess.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(timeline) < 2 {
		t.Fatalf("expected at least 2 timeline events (session_started + finding_added), got %d", len(timeline))
	}
}
