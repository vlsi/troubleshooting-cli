package domain

import (
	"testing"
	"time"
)

func TestSessionStatusValues(t *testing.T) {
	statuses := []SessionStatus{
		SessionOpen, SessionResolved, SessionMitigated, SessionAbandoned, SessionFollowup,
	}
	for _, s := range statuses {
		if s == "" {
			t.Error("session status must not be empty")
		}
	}
}

func TestHypothesisStatusValues(t *testing.T) {
	statuses := []HypothesisStatus{
		HypothesisOpen, HypothesisSupported, HypothesisContradicted,
		HypothesisConfirmed, HypothesisRejected,
	}
	for _, s := range statuses {
		if s == "" {
			t.Error("hypothesis status must not be empty")
		}
	}
}

func TestEvidenceTypeValues(t *testing.T) {
	types := []EvidenceType{
		EvidenceLog, EvidenceShell, EvidenceSQL, EvidenceFile,
		EvidenceURL, EvidenceTrace, EvidenceMetric, EvidenceK8s,
	}
	for _, et := range types {
		if et == "" {
			t.Error("evidence type must not be empty")
		}
	}
}

func TestSessionStateComposition(t *testing.T) {
	now := time.Now()
	state := SessionState{
		Session: Session{
			ID:        "s1",
			Title:     "test",
			Status:    SessionOpen,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Findings:   []Finding{{ID: "f1", SessionID: "s1", Summary: "found it"}},
		Hypotheses: []Hypothesis{{ID: "h1", SessionID: "s1", Statement: "maybe this"}},
		Timeline:   []TimelineEvent{{ID: "t1", SessionID: "s1", Kind: "session_started"}},
	}
	if state.Session.ID != "s1" {
		t.Error("expected session id s1")
	}
	if len(state.Findings) != 1 {
		t.Error("expected 1 finding")
	}
	if len(state.Hypotheses) != 1 {
		t.Error("expected 1 hypothesis")
	}
	if len(state.Timeline) != 1 {
		t.Error("expected 1 timeline event")
	}
}
