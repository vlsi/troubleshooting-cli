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
func (s *Service) RecommendNextSteps(sessionID string, n int) ([]domain.Recommendation, error) {
	hyps, err := s.RankHypotheses(sessionID)
	if err != nil {
		return nil, err
	}
	var recs []domain.Recommendation
	for _, h := range hyps {
		if h.Status == domain.HypothesisConfirmed || h.Status == domain.HypothesisRejected {
			continue
		}
		for _, check := range h.NextChecks {
			recs = append(recs, domain.Recommendation{
				Action: check,
				Reason: fmt.Sprintf("Next check for hypothesis: %s (status: %s)", h.Statement, h.Status),
				Goal:   "Advance investigation of this hypothesis",
			})
			if len(recs) >= n {
				return recs, nil
			}
		}
	}
	if len(recs) == 0 {
		recs = append(recs, domain.Recommendation{
			Action: "Add more findings or hypotheses to guide the investigation",
			Reason: "No pending checks found",
			Goal:   "Expand the investigation scope",
		})
	}
	return recs, nil
}

// GetTimeline returns timeline events for a session.
func (s *Service) GetTimeline(sessionID string) ([]domain.TimelineEvent, error) {
	events, err := s.store.ListTimeline(sessionID)
	if err != nil {
		return nil, fmt.Errorf("list timeline: %w", err)
	}
	return events, nil
}

// GenerateSummary produces a markdown summary for handoff or postmortem.
func (s *Service) GenerateSummary(sessionID, mode string) (string, error) {
	state, err := s.GetState(sessionID)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	switch mode {
	case "handoff":
		b.WriteString(fmt.Sprintf("# Handoff: %s\n\n", state.Session.Title))
	case "postmortem-draft":
		b.WriteString(fmt.Sprintf("# Postmortem Draft: %s\n\n", state.Session.Title))
	default:
		b.WriteString(fmt.Sprintf("# Summary: %s\n\n", state.Session.Title))
	}

	b.WriteString(fmt.Sprintf("**Service:** %s  \n", state.Session.Service))
	b.WriteString(fmt.Sprintf("**Environment:** %s  \n", state.Session.Environment))
	b.WriteString(fmt.Sprintf("**Status:** %s  \n\n", state.Session.Status))

	if len(state.Findings) > 0 {
		b.WriteString("## Findings\n\n")
		for _, f := range state.Findings {
			b.WriteString(fmt.Sprintf("- **[%s]** %s", f.Kind, f.Summary))
			if f.Importance != "" {
				b.WriteString(fmt.Sprintf(" (importance: %s)", f.Importance))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if len(state.Hypotheses) > 0 {
		b.WriteString("## Hypotheses\n\n")
		for _, h := range state.Hypotheses {
			conf := "unknown"
			if h.Confidence != nil {
				conf = fmt.Sprintf("%.0f%%", *h.Confidence*100)
			}
			b.WriteString(fmt.Sprintf("- **%s** — status: %s, confidence: %s\n", h.Statement, h.Status, conf))
		}
		b.WriteString("\n")
	}

	if mode == "postmortem-draft" && len(state.Timeline) > 0 {
		b.WriteString("## Timeline\n\n")
		for _, e := range state.Timeline {
			b.WriteString(fmt.Sprintf("- %s — %s: %s\n", e.Timestamp.Format(time.RFC3339), e.Kind, e.Summary))
		}
		b.WriteString("\n")
	}

	return b.String(), nil
}

// CloseSession closes a session with a final status and outcome.
func (s *Service) CloseSession(sessionID string, finalStatus domain.SessionStatus, outcome string) (domain.Session, error) {
	sess, err := s.store.GetSession(sessionID)
	if err != nil {
		return domain.Session{}, fmt.Errorf("session not found: %w", err)
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
