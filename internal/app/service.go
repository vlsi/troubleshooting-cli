package app

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/vlsi/troubleshooting-cli/internal/domain"
)

// Store defines the persistence interface used by the application service.
type Store interface {
	CreateSession(s domain.Session) error
	GetSession(id string) (domain.Session, error)
	UpdateSession(s domain.Session) error

	AddFinding(f domain.Finding) error
	ListFindings(sessionID string) ([]domain.Finding, error)

	AddHypothesis(h domain.Hypothesis) error
	GetHypothesis(id string) (domain.Hypothesis, error)
	UpdateHypothesis(h domain.Hypothesis) error
	ListHypotheses(sessionID string) ([]domain.Hypothesis, error)

	AddTimelineEvent(e domain.TimelineEvent) error
	ListTimeline(sessionID string) ([]domain.TimelineEvent, error)
}

// IDGenerator produces unique IDs.
type IDGenerator func() string

// Service is the shared application core used by both CLI and MCP.
type Service struct {
	store Store
	newID IDGenerator
}

// NewService creates a new application service.
func NewService(store Store, newID IDGenerator) *Service {
	return &Service{store: store, newID: newID}
}

// StartSession creates a new investigation session.
func (s *Service) StartSession(title, service, env, incidentHint string, labels map[string]string) (domain.Session, error) {
	if title == "" {
		return domain.Session{}, fmt.Errorf("title is required")
	}
	if service == "" {
		return domain.Session{}, fmt.Errorf("service is required")
	}
	if env == "" {
		return domain.Session{}, fmt.Errorf("environment is required")
	}
	now := time.Now().UTC()
	sess := domain.Session{
		ID:           s.newID(),
		Title:        title,
		Service:      service,
		Environment:  env,
		IncidentHint: incidentHint,
		Labels:       labels,
		Status:       domain.SessionOpen,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.store.CreateSession(sess); err != nil {
		return domain.Session{}, fmt.Errorf("create session: %w", err)
	}
	s.addEvent(sess.ID, "session_started", "Session started: "+title, sess.ID)
	return sess, nil
}

// GetState returns the full session state.
func (s *Service) GetState(sessionID string) (domain.SessionState, error) {
	sess, err := s.store.GetSession(sessionID)
	if err != nil {
		return domain.SessionState{}, fmt.Errorf("get session: %w", err)
	}
	findings, _ := s.store.ListFindings(sessionID)
	hypotheses, _ := s.store.ListHypotheses(sessionID)
	timeline, _ := s.store.ListTimeline(sessionID)
	return domain.SessionState{
		Session:    sess,
		Findings:   findings,
		Hypotheses: hypotheses,
		Timeline:   timeline,
	}, nil
}

// AddFinding adds a finding to a session.
func (s *Service) AddFinding(sessionID, kind, summary, details, importance string, tags []string, evidence []domain.Evidence) (domain.Finding, error) {
	if sessionID == "" {
		return domain.Finding{}, fmt.Errorf("session_id is required")
	}
	if summary == "" {
		return domain.Finding{}, fmt.Errorf("summary is required")
	}
	if kind == "" {
		kind = "observation"
	}
	if _, err := s.store.GetSession(sessionID); err != nil {
		return domain.Finding{}, fmt.Errorf("session not found: %w", err)
	}
	f := domain.Finding{
		ID:           s.newID(),
		SessionID:    sessionID,
		CreatedAt:    time.Now().UTC(),
		Kind:         kind,
		Summary:      summary,
		Details:      details,
		Importance:   importance,
		Tags:         tags,
		EvidenceRefs: evidence,
	}
	if err := s.store.AddFinding(f); err != nil {
		return domain.Finding{}, fmt.Errorf("add finding: %w", err)
	}
	s.addEvent(sessionID, "finding_added", summary, f.ID)
	return f, nil
}

// AddHypothesis adds a hypothesis to a session.
func (s *Service) AddHypothesis(sessionID, statement, impact string, confidence *float64, nextChecks []string) (domain.Hypothesis, error) {
	if sessionID == "" {
		return domain.Hypothesis{}, fmt.Errorf("session_id is required")
	}
	if statement == "" {
		return domain.Hypothesis{}, fmt.Errorf("statement is required")
	}
	if confidence != nil && (*confidence < 0 || *confidence > 1) {
		return domain.Hypothesis{}, fmt.Errorf("confidence must be between 0.0 and 1.0")
	}
	if _, err := s.store.GetSession(sessionID); err != nil {
		return domain.Hypothesis{}, fmt.Errorf("session not found: %w", err)
	}
	now := time.Now().UTC()
	h := domain.Hypothesis{
		ID:         s.newID(),
		SessionID:  sessionID,
		CreatedAt:  now,
		UpdatedAt:  now,
		Statement:  statement,
		Status:     domain.HypothesisOpen,
		Confidence: confidence,
		Impact:     impact,
		NextChecks: nextChecks,
	}
	if err := s.store.AddHypothesis(h); err != nil {
		return domain.Hypothesis{}, fmt.Errorf("add hypothesis: %w", err)
	}
	s.addEvent(sessionID, "hypothesis_added", statement, h.ID)
	return h, nil
}

// UpdateHypothesis updates hypothesis status, confidence, linked findings, and next checks.
func (s *Service) UpdateHypothesis(id string, status *domain.HypothesisStatus, confidence *float64, supportIDs, contradictIDs, nextChecks []string) (domain.Hypothesis, error) {
	if id == "" {
		return domain.Hypothesis{}, fmt.Errorf("hypothesis id is required")
	}
	if status != nil {
		valid := map[domain.HypothesisStatus]bool{
			domain.HypothesisOpen: true, domain.HypothesisSupported: true,
			domain.HypothesisContradicted: true, domain.HypothesisConfirmed: true,
			domain.HypothesisRejected: true,
		}
		if !valid[*status] {
			return domain.Hypothesis{}, fmt.Errorf("invalid status %q", *status)
		}
	}
	if confidence != nil && (*confidence < 0 || *confidence > 1) {
		return domain.Hypothesis{}, fmt.Errorf("confidence must be between 0.0 and 1.0")
	}
	h, err := s.store.GetHypothesis(id)
	if err != nil {
		return domain.Hypothesis{}, fmt.Errorf("hypothesis not found: %w", err)
	}
	if status != nil {
		h.Status = *status
	}
	if confidence != nil {
		h.Confidence = confidence
	}
	if supportIDs != nil {
		h.SupportingFindingIDs = append(h.SupportingFindingIDs, supportIDs...)
	}
	if contradictIDs != nil {
		h.ContradictingFindingIDs = append(h.ContradictingFindingIDs, contradictIDs...)
	}
	if nextChecks != nil {
		h.NextChecks = append(h.NextChecks, nextChecks...)
	}
	h.UpdatedAt = time.Now().UTC()
	if err := s.store.UpdateHypothesis(h); err != nil {
		return domain.Hypothesis{}, fmt.Errorf("update hypothesis: %w", err)
	}
	s.addEvent(h.SessionID, "hypothesis_updated", h.Statement, h.ID)
	return h, nil
}

// RankHypotheses returns hypotheses sorted by status priority and confidence.
func (s *Service) RankHypotheses(sessionID string) ([]domain.Hypothesis, error) {
	hyps, err := s.store.ListHypotheses(sessionID)
	if err != nil {
		return nil, fmt.Errorf("list hypotheses: %w", err)
	}
	priority := map[domain.HypothesisStatus]int{
		domain.HypothesisSupported:    0,
		domain.HypothesisOpen:         1,
		domain.HypothesisContradicted: 2,
		domain.HypothesisConfirmed:    3,
		domain.HypothesisRejected:     4,
	}
	sort.Slice(hyps, func(i, j int) bool {
		pi, pj := priority[hyps[i].Status], priority[hyps[j].Status]
		if pi != pj {
			return pi < pj
		}
		ci, cj := 0.0, 0.0
		if hyps[i].Confidence != nil {
			ci = *hyps[i].Confidence
		}
		if hyps[j].Confidence != nil {
			cj = *hyps[j].Confidence
		}
		return ci > cj
	})
	return hyps, nil
}

// RecommendNextSteps returns up to n recommended next actions.
// The logic is deterministic and heuristic:
//  1. Gather next_checks from ranked hypotheses (skipping confirmed/rejected).
//  2. For open/supported hypotheses without next_checks, suggest adding checks.
//  3. If no hypotheses exist, suggest adding hypotheses.
//  4. If no findings exist, suggest adding findings.
func (s *Service) RecommendNextSteps(sessionID string, n int) ([]domain.Recommendation, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	if n <= 0 {
		n = 3
	}

	state, err := s.GetState(sessionID)
	if err != nil {
		return nil, err
	}

	hyps, err := s.RankHypotheses(sessionID)
	if err != nil {
		return nil, err
	}

	var recs []domain.Recommendation

	// Step 1: next_checks from active hypotheses, prioritized by rank order
	for _, h := range hyps {
		if h.Status == domain.HypothesisConfirmed || h.Status == domain.HypothesisRejected {
			continue
		}
		for _, check := range h.NextChecks {
			recs = append(recs, domain.Recommendation{
				Action: check,
				Reason: fmt.Sprintf("Pending check for hypothesis %q (status: %s)", h.Statement, h.Status),
				Goal:   fmt.Sprintf("Validate or refute: %s", h.Statement),
			})
			if len(recs) >= n {
				return recs, nil
			}
		}
	}

	// Step 2: active hypotheses with no next_checks
	for _, h := range hyps {
		if h.Status == domain.HypothesisConfirmed || h.Status == domain.HypothesisRejected {
			continue
		}
		if len(h.NextChecks) == 0 {
			recs = append(recs, domain.Recommendation{
				Action: fmt.Sprintf("Define next checks for hypothesis %q", h.Statement),
				Reason: fmt.Sprintf("Hypothesis %q (status: %s) has no next checks defined", h.Statement, h.Status),
				Goal:   "Ensure every active hypothesis has a path to validation",
			})
			if len(recs) >= n {
				return recs, nil
			}
		}
	}

	// Step 3: no hypotheses at all
	if len(hyps) == 0 && len(state.Findings) > 0 {
		recs = append(recs, domain.Recommendation{
			Action: "Formulate hypotheses based on existing findings",
			Reason: fmt.Sprintf("Session has %d finding(s) but no hypotheses", len(state.Findings)),
			Goal:   "Move from observations to testable explanations",
		})
		if len(recs) >= n {
			return recs, nil
		}
	}

	// Step 4: no findings at all
	if len(state.Findings) == 0 {
		recs = append(recs, domain.Recommendation{
			Action: "Gather initial findings: check logs, metrics, and recent changes",
			Reason: "No findings recorded yet",
			Goal:   "Establish an evidence baseline for the investigation",
		})
	}

	return recs, nil
}

// GetTimeline returns timeline events for a session, ordered by timestamp.
func (s *Service) GetTimeline(sessionID string) ([]domain.TimelineEvent, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	if _, err := s.store.GetSession(sessionID); err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}
	events, err := s.store.ListTimeline(sessionID)
	if err != nil {
		return nil, fmt.Errorf("list timeline: %w", err)
	}
	return events, nil
}

// GenerateSummary produces a markdown summary for handoff or postmortem-draft.
//
// Handoff mode includes: context, findings with evidence, hypotheses, and recommended next steps.
// Postmortem-draft mode includes: context, findings with evidence, hypotheses, outcome, and full timeline.
func (s *Service) GenerateSummary(sessionID, mode string) (string, error) {
	if sessionID == "" {
		return "", fmt.Errorf("session_id is required")
	}
	if mode == "" {
		mode = "handoff"
	}
	if mode != "handoff" && mode != "postmortem-draft" {
		return "", fmt.Errorf("mode must be \"handoff\" or \"postmortem-draft\"")
	}

	state, err := s.GetState(sessionID)
	if err != nil {
		return "", err
	}

	var b strings.Builder

	// Header
	switch mode {
	case "handoff":
		b.WriteString(fmt.Sprintf("# Handoff: %s\n\n", state.Session.Title))
	case "postmortem-draft":
		b.WriteString(fmt.Sprintf("# Postmortem Draft: %s\n\n", state.Session.Title))
	}

	// Context
	b.WriteString("## Context\n\n")
	b.WriteString(fmt.Sprintf("- **Service:** %s\n", state.Session.Service))
	b.WriteString(fmt.Sprintf("- **Environment:** %s\n", state.Session.Environment))
	b.WriteString(fmt.Sprintf("- **Status:** %s\n", state.Session.Status))
	if state.Session.IncidentHint != "" {
		b.WriteString(fmt.Sprintf("- **Incident:** %s\n", state.Session.IncidentHint))
	}
	if state.Session.Outcome != "" {
		b.WriteString(fmt.Sprintf("- **Outcome:** %s\n", state.Session.Outcome))
	}
	b.WriteString("\n")

	// Findings with evidence
	if len(state.Findings) > 0 {
		b.WriteString("## Findings\n\n")
		for _, f := range state.Findings {
			b.WriteString(fmt.Sprintf("- **[%s]** %s", f.Kind, f.Summary))
			if f.Importance != "" {
				b.WriteString(fmt.Sprintf(" (importance: %s)", f.Importance))
			}
			b.WriteString("\n")
			if f.Details != "" {
				b.WriteString(fmt.Sprintf("  - Details: %s\n", f.Details))
			}
			for _, ev := range f.EvidenceRefs {
				b.WriteString(fmt.Sprintf("  - Evidence [%s]: %s", ev.Type, ev.Pointer))
				if ev.Snippet != "" {
					b.WriteString(fmt.Sprintf(" — `%s`", ev.Snippet))
				}
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}

	// Hypotheses — use ranked order for clarity
	ranked, _ := s.RankHypotheses(sessionID)
	if len(ranked) > 0 {
		b.WriteString("## Hypotheses\n\n")
		for _, h := range ranked {
			conf := "unknown"
			if h.Confidence != nil {
				conf = fmt.Sprintf("%.0f%%", *h.Confidence*100)
			}
			b.WriteString(fmt.Sprintf("- **%s** — status: %s, confidence: %s\n", h.Statement, h.Status, conf))
			if len(h.SupportingFindingIDs) > 0 {
				b.WriteString(fmt.Sprintf("  - Supporting evidence: %s\n", strings.Join(h.SupportingFindingIDs, ", ")))
			}
			if len(h.ContradictingFindingIDs) > 0 {
				b.WriteString(fmt.Sprintf("  - Contradicting evidence: %s\n", strings.Join(h.ContradictingFindingIDs, ", ")))
			}
		}
		b.WriteString("\n")
	}

	// Handoff: include recommended next steps
	if mode == "handoff" {
		recs, _ := s.RecommendNextSteps(sessionID, 5)
		if len(recs) > 0 {
			b.WriteString("## Recommended Next Steps\n\n")
			for _, r := range recs {
				b.WriteString(fmt.Sprintf("- **%s**\n", r.Action))
				b.WriteString(fmt.Sprintf("  - Why: %s\n", r.Reason))
				b.WriteString(fmt.Sprintf("  - Goal: %s\n", r.Goal))
			}
			b.WriteString("\n")
		}
	}

	// Postmortem: include timeline
	if mode == "postmortem-draft" && len(state.Timeline) > 0 {
		b.WriteString("## Timeline\n\n")
		for _, e := range state.Timeline {
			b.WriteString(fmt.Sprintf("- %s — **%s**: %s\n", e.Timestamp.Format(time.RFC3339), e.Kind, e.Summary))
		}
		b.WriteString("\n")
	}

	s.addEvent(sessionID, "summary_generated", fmt.Sprintf("Summary generated: %s", mode), sessionID)
	return b.String(), nil
}

// CloseSession closes a session with a final status and outcome.
func (s *Service) CloseSession(sessionID string, finalStatus domain.SessionStatus, outcome string) (domain.Session, error) {
	if sessionID == "" {
		return domain.Session{}, fmt.Errorf("session_id is required")
	}
	validStatuses := map[domain.SessionStatus]bool{
		domain.SessionResolved:  true,
		domain.SessionMitigated: true,
		domain.SessionAbandoned: true,
		domain.SessionFollowup:  true,
	}
	if !validStatuses[finalStatus] {
		return domain.Session{}, fmt.Errorf("invalid final status %q; use: resolved, mitigated, abandoned, needs-followup", finalStatus)
	}
	sess, err := s.store.GetSession(sessionID)
	if err != nil {
		return domain.Session{}, fmt.Errorf("session not found: %w", err)
	}
	if sess.Status != domain.SessionOpen {
		return domain.Session{}, fmt.Errorf("session is already closed (status: %s)", sess.Status)
	}
	now := time.Now().UTC()
	sess.Status = finalStatus
	sess.Outcome = outcome
	sess.ClosedAt = &now
	sess.UpdatedAt = now
	if err := s.store.UpdateSession(sess); err != nil {
		return domain.Session{}, fmt.Errorf("close session: %w", err)
	}
	s.addEvent(sessionID, "session_closed", fmt.Sprintf("Session closed: %s", finalStatus), sessionID)
	return sess, nil
}

func (s *Service) addEvent(sessionID, kind, summary, refID string) {
	e := domain.TimelineEvent{
		ID:        s.newID(),
		SessionID: sessionID,
		Timestamp: time.Now().UTC(),
		Kind:      kind,
		Summary:   summary,
		RefID:     refID,
	}
	_ = s.store.AddTimelineEvent(e)
}
