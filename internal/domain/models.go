package domain

import "time"

// Session represents a troubleshooting investigation session.
type Session struct {
	ID           string            `json:"id"`
	Title        string            `json:"title"`
	Service      string            `json:"service"`
	Environment  string            `json:"environment"`
	IncidentHint string            `json:"incident_hint,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
	Status       SessionStatus     `json:"status"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	ClosedAt     *time.Time        `json:"closed_at,omitempty"`
	Outcome      string            `json:"outcome,omitempty"`
}

type SessionStatus string

const (
	SessionOpen       SessionStatus = "open"
	SessionResolved   SessionStatus = "resolved"
	SessionMitigated  SessionStatus = "mitigated"
	SessionAbandoned  SessionStatus = "abandoned"
	SessionFollowup   SessionStatus = "needs-followup"
)

// Finding is a structured observation captured during investigation.
type Finding struct {
	ID           string      `json:"id"`
	SessionID    string      `json:"session_id"`
	CreatedAt    time.Time   `json:"created_at"`
	Kind         string      `json:"kind"`
	Summary      string      `json:"summary"`
	Details      string      `json:"details,omitempty"`
	Importance   string      `json:"importance,omitempty"`
	Tags         []string    `json:"tags,omitempty"`
	EvidenceRefs []Evidence  `json:"evidence_refs,omitempty"`
}

// Evidence is a structured reference to supporting material.
type Evidence struct {
	Type        EvidenceType `json:"type"`
	Pointer     string       `json:"pointer"`
	Snippet     string       `json:"snippet,omitempty"`
	CollectedAt *time.Time   `json:"collected_at,omitempty"`
}

type EvidenceType string

const (
	EvidenceLog    EvidenceType = "log"
	EvidenceShell  EvidenceType = "shell"
	EvidenceSQL    EvidenceType = "sql"
	EvidenceFile   EvidenceType = "file"
	EvidenceURL    EvidenceType = "url"
	EvidenceTrace  EvidenceType = "trace"
	EvidenceMetric EvidenceType = "metric"
	EvidenceK8s    EvidenceType = "k8s"
)

// Hypothesis is a candidate explanation for the observed problem.
type Hypothesis struct {
	ID                    string           `json:"id"`
	SessionID             string           `json:"session_id"`
	CreatedAt             time.Time        `json:"created_at"`
	UpdatedAt             time.Time        `json:"updated_at"`
	Statement             string           `json:"statement"`
	Status                HypothesisStatus `json:"status"`
	Confidence            *float64         `json:"confidence,omitempty"`
	Impact                string           `json:"impact,omitempty"`
	SupportingFindingIDs  []string         `json:"supporting_finding_ids,omitempty"`
	ContradictingFindingIDs []string       `json:"contradicting_finding_ids,omitempty"`
	NextChecks            []string         `json:"next_checks,omitempty"`
}

type HypothesisStatus string

const (
	HypothesisOpen         HypothesisStatus = "open"
	HypothesisSupported    HypothesisStatus = "supported"
	HypothesisContradicted HypothesisStatus = "contradicted"
	HypothesisConfirmed    HypothesisStatus = "confirmed"
	HypothesisRejected     HypothesisStatus = "rejected"
)

// TimelineEvent records a moment in the investigation.
type TimelineEvent struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Timestamp time.Time `json:"timestamp"`
	Kind      string    `json:"kind"`
	Summary   string    `json:"summary"`
	RefID     string    `json:"ref_id,omitempty"`
}

// Recommendation is a suggested next investigative step.
type Recommendation struct {
	Action string `json:"action"`
	Reason string `json:"reason"`
	Goal   string `json:"goal"`
}

// SessionState is the full state returned by get-state.
type SessionState struct {
	Session    Session         `json:"session"`
	Findings   []Finding       `json:"findings"`
	Hypotheses []Hypothesis    `json:"hypotheses"`
	Timeline   []TimelineEvent `json:"recent_timeline"`
}
