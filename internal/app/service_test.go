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

func TestAddHypothesisValidation(t *testing.T) {
	svc := setup()
	sess, _ := svc.StartSession("test", "svc", "dev", "", nil)

	_, err := svc.AddHypothesis("", "statement", "", nil, nil)
	if err == nil {
		t.Error("expected error for empty session_id")
	}
	_, err = svc.AddHypothesis(sess.ID, "", "", nil, nil)
	if err == nil {
		t.Error("expected error for empty statement")
	}
	bad := 1.5
	_, err = svc.AddHypothesis(sess.ID, "test", "", &bad, nil)
	if err == nil {
		t.Error("expected error for confidence > 1.0")
	}
	neg := -0.1
	_, err = svc.AddHypothesis(sess.ID, "test", "", &neg, nil)
	if err == nil {
		t.Error("expected error for confidence < 0.0")
	}
	_, err = svc.AddHypothesis("nonexistent", "test", "", nil, nil)
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestUpdateHypothesisValidation(t *testing.T) {
	svc := setup()
	sess, _ := svc.StartSession("test", "svc", "dev", "", nil)
	h, _ := svc.AddHypothesis(sess.ID, "test", "", nil, nil)

	_, err := svc.UpdateHypothesis("", nil, nil, nil, nil, nil)
	if err == nil {
		t.Error("expected error for empty id")
	}
	badStatus := domain.HypothesisStatus("bogus")
	_, err = svc.UpdateHypothesis(h.ID, &badStatus, nil, nil, nil, nil)
	if err == nil {
		t.Error("expected error for invalid status")
	}
	bad := 2.0
	_, err = svc.UpdateHypothesis(h.ID, nil, &bad, nil, nil, nil)
	if err == nil {
		t.Error("expected error for confidence > 1.0")
	}
	_, err = svc.UpdateHypothesis("nonexistent", nil, nil, nil, nil, nil)
	if err == nil {
		t.Error("expected error for nonexistent hypothesis")
	}
}

func TestRankHypothesesByStatusThenConfidence(t *testing.T) {
	svc := setup()
	sess, _ := svc.StartSession("test", "svc", "dev", "", nil)

	c30 := 0.3
	c90 := 0.9
	c50 := 0.5

	// Add hypotheses with different statuses and confidences
	hOpen, _ := svc.AddHypothesis(sess.ID, "open low", "", &c30, nil)
	hSupported, _ := svc.AddHypothesis(sess.ID, "supported high", "", &c90, nil)
	hContradicted, _ := svc.AddHypothesis(sess.ID, "contradicted", "", &c50, nil)
	hRejected, _ := svc.AddHypothesis(sess.ID, "rejected", "", &c90, nil)
	hOpenHigh, _ := svc.AddHypothesis(sess.ID, "open high", "", &c90, nil)

	// Update statuses
	supported := domain.HypothesisSupported
	contradicted := domain.HypothesisContradicted
	rejected := domain.HypothesisRejected
	svc.UpdateHypothesis(hSupported.ID, &supported, nil, nil, nil, nil)
	svc.UpdateHypothesis(hContradicted.ID, &contradicted, nil, nil, nil, nil)
	svc.UpdateHypothesis(hRejected.ID, &rejected, nil, nil, nil, nil)

	ranked, err := svc.RankHypotheses(sess.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ranked) != 5 {
		t.Fatalf("expected 5 hypotheses, got %d", len(ranked))
	}

	// Expected order: supported(0) > open(1) > contradicted(2) > confirmed(3) > rejected(4)
	// Within same status, higher confidence first
	if ranked[0].ID != hSupported.ID {
		t.Errorf("rank 0: expected supported, got %q (status=%s)", ranked[0].Statement, ranked[0].Status)
	}
	// Two open hypotheses: high confidence first
	if ranked[1].ID != hOpenHigh.ID {
		t.Errorf("rank 1: expected open high, got %q", ranked[1].Statement)
	}
	if ranked[2].ID != hOpen.ID {
		t.Errorf("rank 2: expected open low, got %q", ranked[2].Statement)
	}
	if ranked[3].ID != hContradicted.ID {
		t.Errorf("rank 3: expected contradicted, got %q", ranked[3].Statement)
	}
	if ranked[4].ID != hRejected.ID {
		t.Errorf("rank 4: expected rejected, got %q", ranked[4].Statement)
	}
}

func TestRankHypothesesEmpty(t *testing.T) {
	svc := setup()
	sess, _ := svc.StartSession("test", "svc", "dev", "", nil)
	ranked, err := svc.RankHypotheses(sess.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ranked) != 0 {
		t.Errorf("expected 0 hypotheses, got %d", len(ranked))
	}
}

func TestUpdateHypothesisAccumulatesFindings(t *testing.T) {
	svc := setup()
	sess, _ := svc.StartSession("test", "svc", "dev", "", nil)
	h, _ := svc.AddHypothesis(sess.ID, "test", "", nil, nil)

	// First update adds f1
	h, _ = svc.UpdateHypothesis(h.ID, nil, nil, []string{"f1"}, []string{"f2"}, nil)
	if len(h.SupportingFindingIDs) != 1 || h.SupportingFindingIDs[0] != "f1" {
		t.Errorf("expected [f1], got %v", h.SupportingFindingIDs)
	}
	if len(h.ContradictingFindingIDs) != 1 || h.ContradictingFindingIDs[0] != "f2" {
		t.Errorf("expected [f2], got %v", h.ContradictingFindingIDs)
	}

	// Second update appends f3
	h, _ = svc.UpdateHypothesis(h.ID, nil, nil, []string{"f3"}, nil, []string{"check logs"})
	if len(h.SupportingFindingIDs) != 2 {
		t.Errorf("expected 2 supporting findings, got %d", len(h.SupportingFindingIDs))
	}
	if len(h.NextChecks) != 1 || h.NextChecks[0] != "check logs" {
		t.Errorf("expected [check logs], got %v", h.NextChecks)
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

func TestRecommendNextStepsFromChecks(t *testing.T) {
	svc := setup()
	sess, _ := svc.StartSession("test", "svc", "dev", "", nil)
	svc.AddFinding(sess.ID, "observation", "something observed", "", "", nil, nil)
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
	if !strings.Contains(recs[0].Reason, "pool exhaustion") {
		t.Error("reason should reference the hypothesis")
	}
	if !strings.Contains(recs[0].Goal, "pool exhaustion") {
		t.Error("goal should reference the hypothesis")
	}
}

func TestRecommendSkipsConfirmedAndRejected(t *testing.T) {
	svc := setup()
	sess, _ := svc.StartSession("test", "svc", "dev", "", nil)
	h1, _ := svc.AddHypothesis(sess.ID, "confirmed one", "", nil, []string{"should not appear"})
	h2, _ := svc.AddHypothesis(sess.ID, "rejected one", "", nil, []string{"should not appear either"})
	svc.AddHypothesis(sess.ID, "open one", "", nil, []string{"should appear"})

	confirmed := domain.HypothesisConfirmed
	rejected := domain.HypothesisRejected
	svc.UpdateHypothesis(h1.ID, &confirmed, nil, nil, nil, nil)
	svc.UpdateHypothesis(h2.ID, &rejected, nil, nil, nil, nil)

	recs, _ := svc.RecommendNextSteps(sess.ID, 10)
	for _, r := range recs {
		if strings.Contains(r.Action, "should not appear") {
			t.Errorf("recommendation from confirmed/rejected hypothesis should be excluded: %q", r.Action)
		}
	}
	if len(recs) < 1 || recs[0].Action != "should appear" {
		t.Error("expected check from open hypothesis")
	}
}

func TestRecommendSuggestsChecksForHypWithoutChecks(t *testing.T) {
	svc := setup()
	sess, _ := svc.StartSession("test", "svc", "dev", "", nil)
	svc.AddHypothesis(sess.ID, "needs investigation", "", nil, nil)

	recs, _ := svc.RecommendNextSteps(sess.ID, 3)
	if len(recs) == 0 {
		t.Fatal("expected at least one recommendation")
	}
	if !strings.Contains(recs[0].Action, "Define next checks") {
		t.Errorf("expected suggestion to define checks, got %q", recs[0].Action)
	}
}

func TestRecommendSuggestsHypothesesWhenNone(t *testing.T) {
	svc := setup()
	sess, _ := svc.StartSession("test", "svc", "dev", "", nil)
	svc.AddFinding(sess.ID, "observation", "high latency", "", "", nil, nil)

	recs, _ := svc.RecommendNextSteps(sess.ID, 3)
	found := false
	for _, r := range recs {
		if strings.Contains(r.Action, "hypotheses") {
			found = true
		}
	}
	if !found {
		t.Error("expected recommendation to formulate hypotheses")
	}
}

func TestRecommendSuggestsFindingsWhenEmpty(t *testing.T) {
	svc := setup()
	sess, _ := svc.StartSession("test", "svc", "dev", "", nil)

	recs, _ := svc.RecommendNextSteps(sess.ID, 3)
	found := false
	for _, r := range recs {
		if strings.Contains(r.Action, "findings") || strings.Contains(r.Action, "logs") {
			found = true
		}
	}
	if !found {
		t.Error("expected recommendation to gather findings")
	}
}

func TestRecommendRespectsLimit(t *testing.T) {
	svc := setup()
	sess, _ := svc.StartSession("test", "svc", "dev", "", nil)
	svc.AddHypothesis(sess.ID, "h1", "", nil, []string{"a", "b", "c"})
	svc.AddHypothesis(sess.ID, "h2", "", nil, []string{"d", "e"})

	recs, _ := svc.RecommendNextSteps(sess.ID, 2)
	if len(recs) != 2 {
		t.Errorf("expected exactly 2 recommendations, got %d", len(recs))
	}
}

func TestRecommendValidation(t *testing.T) {
	svc := setup()
	_, err := svc.RecommendNextSteps("", 3)
	if err == nil {
		t.Error("expected error for empty session_id")
	}
	_, err = svc.RecommendNextSteps("nonexistent", 3)
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestGenerateSummaryHandoff(t *testing.T) {
	svc := setup()
	sess, _ := svc.StartSession("pod crash", "api-gw", "prod", "INC-42", nil)
	svc.AddFinding(sess.ID, "observation", "OOM kill", "RSS exceeded 2GB", "high", nil,
		[]domain.Evidence{{Type: domain.EvidenceLog, Pointer: "/var/log/app.log", Snippet: "OOM killed process"}})
	c := 0.8
	h, _ := svc.AddHypothesis(sess.ID, "memory leak in handler", "high", &c, []string{"heap profile"})
	supported := domain.HypothesisSupported
	svc.UpdateHypothesis(h.ID, &supported, nil, nil, nil, nil)

	md, err := svc.GenerateSummary(sess.ID, "handoff")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(md, "# Handoff:") {
		t.Error("expected handoff header")
	}
	if !strings.Contains(md, "api-gw") {
		t.Error("expected service in context")
	}
	if !strings.Contains(md, "INC-42") {
		t.Error("expected incident hint in context")
	}
	if !strings.Contains(md, "OOM kill") {
		t.Error("expected finding summary")
	}
	if !strings.Contains(md, "RSS exceeded 2GB") {
		t.Error("expected finding details")
	}
	if !strings.Contains(md, "/var/log/app.log") {
		t.Error("expected evidence pointer")
	}
	if !strings.Contains(md, "OOM killed process") {
		t.Error("expected evidence snippet")
	}
	if !strings.Contains(md, "memory leak") {
		t.Error("expected hypothesis")
	}
	if !strings.Contains(md, "80%") {
		t.Error("expected confidence percentage")
	}
	if !strings.Contains(md, "Recommended Next Steps") {
		t.Error("expected next steps section in handoff")
	}
	if !strings.Contains(md, "heap profile") {
		t.Error("expected next check in recommendations")
	}
	// handoff should NOT have timeline
	if strings.Contains(md, "## Timeline") {
		t.Error("handoff should not include timeline section")
	}
}

func TestGenerateSummaryPostmortem(t *testing.T) {
	svc := setup()
	sess, _ := svc.StartSession("pod crash", "api-gw", "prod", "", nil)
	svc.AddFinding(sess.ID, "observation", "OOM kill", "", "high", nil, nil)
	svc.AddHypothesis(sess.ID, "memory leak", "", nil, nil)
	svc.CloseSession(sess.ID, domain.SessionResolved, "fixed memory leak in handler X")

	md, err := svc.GenerateSummary(sess.ID, "postmortem-draft")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(md, "# Postmortem Draft:") {
		t.Error("expected postmortem header")
	}
	if !strings.Contains(md, "resolved") {
		t.Error("expected resolved status")
	}
	if !strings.Contains(md, "fixed memory leak") {
		t.Error("expected outcome in context")
	}
	if !strings.Contains(md, "## Timeline") {
		t.Error("expected timeline section in postmortem")
	}
	if !strings.Contains(md, "session_started") {
		t.Error("expected session_started in timeline")
	}
	// postmortem should NOT have next steps
	if strings.Contains(md, "Recommended Next Steps") {
		t.Error("postmortem should not include next steps")
	}
}

func TestGenerateSummaryValidation(t *testing.T) {
	svc := setup()
	_, err := svc.GenerateSummary("", "handoff")
	if err == nil {
		t.Error("expected error for empty session_id")
	}
	sess, _ := svc.StartSession("test", "svc", "dev", "", nil)
	_, err = svc.GenerateSummary(sess.ID, "bogus")
	if err == nil {
		t.Error("expected error for invalid mode")
	}
}

func TestGenerateSummaryDefaultMode(t *testing.T) {
	svc := setup()
	sess, _ := svc.StartSession("test", "svc", "dev", "", nil)
	md, err := svc.GenerateSummary(sess.ID, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(md, "# Handoff:") {
		t.Error("expected default mode to be handoff")
	}
}

func TestGenerateSummaryRecordsTimelineEvent(t *testing.T) {
	svc := setup()
	sess, _ := svc.StartSession("test", "svc", "dev", "", nil)
	svc.GenerateSummary(sess.ID, "handoff")

	timeline, _ := svc.GetTimeline(sess.ID)
	found := false
	for _, e := range timeline {
		if e.Kind == "summary_generated" {
			found = true
		}
	}
	if !found {
		t.Error("expected summary_generated timeline event")
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
