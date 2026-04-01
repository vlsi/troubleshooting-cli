package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	"github.com/vlsi/troubleshooting-cli/internal/app"
	"github.com/vlsi/troubleshooting-cli/internal/domain"
)

// flexFloat64 unmarshals a JSON number or a string containing a number.
// LLMs sometimes send "0.95" instead of 0.95.
type flexFloat64 float64

func (f *flexFloat64) UnmarshalJSON(data []byte) error {
	var num float64
	if err := json.Unmarshal(data, &num); err == nil {
		*f = flexFloat64(num)
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("expected a number or numeric string, got %s", string(data))
	}
	num, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("invalid numeric string %q", s)
	}
	*f = flexFloat64(num)
	return nil
}

// flexEvidence unmarshals either a full Evidence object or a plain string.
// When a string is provided, it becomes an Evidence with Type "observation"
// and the string as the Pointer.
type flexEvidence domain.Evidence

func (f *flexEvidence) UnmarshalJSON(data []byte) error {
	var ev domain.Evidence
	if err := json.Unmarshal(data, &ev); err == nil {
		*f = flexEvidence(ev)
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("evidence must be an object or string, got %s", string(data))
	}
	*f = flexEvidence(domain.Evidence{Type: "observation", Pointer: s})
	return nil
}

// normalizeHypothesisStatus maps common LLM synonyms to canonical status values.
func normalizeHypothesisStatus(s *domain.HypothesisStatus) {
	if s == nil {
		return
	}
	synonyms := map[domain.HypothesisStatus]domain.HypothesisStatus{
		"refuted":      domain.HypothesisRejected,
		"disproven":    domain.HypothesisRejected,
		"disproved":    domain.HypothesisRejected,
		"ruled_out":    domain.HypothesisRejected,
		"ruled-out":    domain.HypothesisRejected,
		"eliminated":   domain.HypothesisRejected,
		"verified":            domain.HypothesisConfirmed,
		"proven":              domain.HypothesisConfirmed,
		"validated":           domain.HypothesisConfirmed,
		"partially_confirmed": domain.HypothesisSupported,
		"partially-confirmed": domain.HypothesisSupported,
		"likely":       domain.HypothesisSupported,
		"probable":     domain.HypothesisSupported,
		"unlikely":     domain.HypothesisContradicted,
		"disproof":     domain.HypothesisContradicted,
		"investigating": domain.HypothesisOpen,
		"pending":      domain.HypothesisOpen,
		"unknown":      domain.HypothesisOpen,
	}
	if canonical, ok := synonyms[*s]; ok {
		*s = canonical
	}
}

// normalizeSessionStatus maps common LLM synonyms to canonical close statuses.
func normalizeSessionStatus(s *domain.SessionStatus) {
	if s == nil {
		return
	}
	synonyms := map[domain.SessionStatus]domain.SessionStatus{
		"fixed":          domain.SessionResolved,
		"done":           domain.SessionResolved,
		"closed":         domain.SessionResolved,
		"workaround":     domain.SessionMitigated,
		"partial":        domain.SessionMitigated,
		"wontfix":        domain.SessionAbandoned,
		"gave_up":        domain.SessionAbandoned,
		"gave-up":        domain.SessionAbandoned,
		"followup":       domain.SessionFollowup,
		"needs_followup": domain.SessionFollowup,
		"follow-up":      domain.SessionFollowup,
		"needs-follow-up": domain.SessionFollowup,
	}
	if canonical, ok := synonyms[*s]; ok {
		*s = canonical
	}
}

// normalizeSummaryMode maps common LLM synonyms to canonical summary modes.
func normalizeSummaryMode(mode *string) {
	if mode == nil {
		return
	}
	synonyms := map[string]string{
		"postmortem":       "postmortem-draft",
		"post-mortem":      "postmortem-draft",
		"post_mortem":      "postmortem-draft",
		"postmortem_draft": "postmortem-draft",
		"post-mortem-draft": "postmortem-draft",
		"post_mortem_draft": "postmortem-draft",
	}
	if canonical, ok := synonyms[*mode]; ok {
		*mode = canonical
	}
}

// flexInt unmarshals a JSON number or a string containing an integer.
type flexInt int

func (f *flexInt) UnmarshalJSON(data []byte) error {
	var num int
	if err := json.Unmarshal(data, &num); err == nil {
		*f = flexInt(num)
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("expected a number or numeric string, got %s", string(data))
	}
	num64, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("invalid integer string %q", s)
	}
	*f = flexInt(num64)
	return nil
}

// JSON-RPC types for MCP stdio protocol.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string     `json:"jsonrpc"`
	ID      any        `json:"id,omitempty"`
	Result  any        `json:"result,omitempty"`
	Error   *Error     `json:"error,omitempty"`
}

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"inputSchema"`
}

// Server is an MCP stdio server backed by the shared application service.
type Server struct {
	svc *app.Service
}

// NewServer creates a new MCP server.
func NewServer(svc *app.Service) *Server {
	return &Server{svc: svc}
}

// Run reads JSON-RPC requests from in and writes responses to out.
func (s *Server) Run(in io.Reader, out io.Writer) {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	enc := json.NewEncoder(out)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}
		resp := s.handle(req)
		if resp != nil {
			enc.Encode(resp)
		}
	}
}

func (s *Server) handle(req Request) *Response {
	switch req.Method {
	case "initialize":
		return s.respond(req.ID, map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":   map[string]any{"tools": map[string]any{}},
			"serverInfo":     map[string]any{"name": "troubleshooting-mcp", "version": "0.1.0"},
		})
	case "notifications/initialized":
		return nil
	case "tools/list":
		return s.respond(req.ID, map[string]any{"tools": s.toolDefinitions()})
	case "tools/call":
		return s.handleToolCall(req)
	default:
		return s.respondError(req.ID, -32601, "method not found: "+req.Method)
	}
}

func (s *Server) handleToolCall(req Request) *Response {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return s.respondError(req.ID, -32602, "invalid params")
	}

	result, err := s.dispatch(params.Name, params.Arguments)
	if err != nil {
		return s.respond(req.ID, map[string]any{
			"content": []map[string]any{{"type": "text", "text": fmt.Sprintf("error: %v", err)}},
			"isError": true,
		})
	}
	text, _ := json.MarshalIndent(result, "", "  ")
	return s.respond(req.ID, map[string]any{
		"content": []map[string]any{{"type": "text", "text": string(text)}},
	})
}

func (s *Server) dispatch(name string, args json.RawMessage) (any, error) {
	switch name {
	case "session_start":
		var p struct {
			Title        string            `json:"title"`
			Service      string            `json:"service"`
			Environment  string            `json:"environment"`
			IncidentHint string            `json:"incident_hint"`
			Labels       map[string]string `json:"labels"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, err
		}
		return s.svc.StartSession(p.Title, p.Service, p.Environment, p.IncidentHint, p.Labels)

	case "session_get_state":
		var p struct{ SessionID string `json:"session_id"` }
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, err
		}
		return s.svc.GetState(p.SessionID)

	case "session_add_finding":
		var p struct {
			SessionID  string         `json:"session_id"`
			Kind       string         `json:"kind"`
			Summary    string         `json:"summary"`
			Details    string         `json:"details"`
			Importance string         `json:"importance"`
			Tags       []string       `json:"tags"`
			Evidence   []flexEvidence `json:"evidence"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, err
		}
		var evidence []domain.Evidence
		for _, fe := range p.Evidence {
			evidence = append(evidence, domain.Evidence(fe))
		}
		return s.svc.AddFinding(p.SessionID, p.Kind, p.Summary, p.Details, p.Importance, p.Tags, evidence)

	case "session_add_hypothesis":
		var p struct {
			SessionID  string       `json:"session_id"`
			Statement  string       `json:"statement"`
			Impact     string       `json:"impact"`
			Confidence *flexFloat64 `json:"confidence"`
			NextChecks []string     `json:"next_checks"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, err
		}
		var conf *float64
		if p.Confidence != nil {
			v := float64(*p.Confidence)
			conf = &v
		}
		return s.svc.AddHypothesis(p.SessionID, p.Statement, p.Impact, conf, p.NextChecks)

	case "session_update_hypothesis":
		var p struct {
			ID         string                   `json:"id"`
			Status     *domain.HypothesisStatus `json:"status"`
			Confidence *flexFloat64             `json:"confidence"`
			Support    []string                 `json:"supporting_finding_ids"`
			Contradict []string                 `json:"contradicting_finding_ids"`
			NextChecks []string                 `json:"next_checks"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, err
		}
		normalizeHypothesisStatus(p.Status)
		var conf *float64
		if p.Confidence != nil {
			v := float64(*p.Confidence)
			conf = &v
		}
		return s.svc.UpdateHypothesis(p.ID, p.Status, conf, p.Support, p.Contradict, p.NextChecks)

	case "session_rank_hypotheses":
		var p struct{ SessionID string `json:"session_id"` }
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, err
		}
		return s.svc.RankHypotheses(p.SessionID)

	case "session_recommend_next_step":
		var p struct {
			SessionID string  `json:"session_id"`
			Count     flexInt `json:"count"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, err
		}
		count := int(p.Count)
		if count <= 0 {
			count = 3
		}
		return s.svc.RecommendNextSteps(p.SessionID, count)

	case "session_generate_summary":
		var p struct {
			SessionID string `json:"session_id"`
			Mode      string `json:"mode"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, err
		}
		normalizeSummaryMode(&p.Mode)
		return s.svc.GenerateSummary(p.SessionID, p.Mode)

	case "session_get_timeline":
		var p struct{ SessionID string `json:"session_id"` }
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, err
		}
		return s.svc.GetTimeline(p.SessionID)

	case "session_close":
		var p struct {
			SessionID string               `json:"session_id"`
			Status    domain.SessionStatus `json:"status"`
			Outcome   string               `json:"outcome"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, err
		}
		normalizeSessionStatus(&p.Status)
		return s.svc.CloseSession(p.SessionID, p.Status, p.Outcome)

	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

func (s *Server) respond(id any, result any) *Response {
	return &Response{JSONRPC: "2.0", ID: id, Result: result}
}

func (s *Server) respondError(id any, code int, msg string) *Response {
	return &Response{JSONRPC: "2.0", ID: id, Error: &Error{Code: code, Message: msg}}
}

func (s *Server) toolDefinitions() []toolDef {
	return []toolDef{
		{Name: "session_start", Description: "Start a new troubleshooting investigation session. Returns the session object with its ID.", InputSchema: map[string]any{
			"type": "object", "required": []string{"title", "service", "environment"},
			"properties": map[string]any{
				"title":         map[string]any{"type": "string", "description": "Short description of the problem being investigated"},
				"service":       map[string]any{"type": "string", "description": "Name of the service or component under investigation"},
				"environment":   map[string]any{"type": "string", "description": "Environment: prod, staging, dev, etc."},
				"incident_hint": map[string]any{"type": "string", "description": "Incident ID or reference if known (e.g. INC-1234)"},
				"labels":        map[string]any{"type": "object", "description": "Optional key-value labels", "additionalProperties": map[string]any{"type": "string"}},
			},
		}},
		{Name: "session_get_state", Description: "Get full session state including findings, hypotheses, and recent timeline events.", InputSchema: map[string]any{
			"type": "object", "required": []string{"session_id"},
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string", "description": "Session ID returned by session_start"},
			},
		}},
		{Name: "session_add_finding", Description: "Record a structured observation or piece of evidence discovered during investigation. Use 'summary' for the one-line description, 'kind' for the category, and 'importance' for severity.", InputSchema: map[string]any{
			"type": "object", "required": []string{"session_id", "kind", "summary"},
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string", "description": "Session ID"},
				"kind":       map[string]any{"type": "string", "description": "Type of finding", "enum": []string{"observation", "error", "anomaly", "configuration", "change"}},
				"summary":    map[string]any{"type": "string", "description": "One-line summary of what was found (required)"},
				"details":    map[string]any{"type": "string", "description": "Longer explanation, raw output, or additional context"},
				"importance": map[string]any{"type": "string", "description": "Severity level", "enum": []string{"critical", "high", "medium", "low"}},
				"tags":       map[string]any{"type": "array", "description": "Categorization tags", "items": map[string]any{"type": "string"}},
				"evidence": map[string]any{"type": "array", "description": "Evidence references. Each item is either a structured object or a plain string description.", "items": map[string]any{
					"oneOf": []map[string]any{
						{
							"type": "object",
							"properties": map[string]any{
								"type":    map[string]any{"type": "string", "description": "Evidence type", "enum": []string{"log", "shell", "sql", "file", "url", "trace", "metric", "k8s"}},
								"pointer": map[string]any{"type": "string", "description": "Location: file path, URL, command, query, etc."},
								"snippet": map[string]any{"type": "string", "description": "Relevant excerpt from the evidence source"},
							},
							"required": []string{"type", "pointer"},
						},
						{
							"type":        "string",
							"description": "Plain text evidence description (converted to type 'observation')",
						},
					},
				}},
			},
		}},
		{Name: "session_add_hypothesis", Description: "Record a candidate explanation for the problem. Always include next_checks so the hypothesis can be validated.", InputSchema: map[string]any{
			"type": "object", "required": []string{"session_id", "statement"},
			"properties": map[string]any{
				"session_id":  map[string]any{"type": "string", "description": "Session ID"},
				"statement":   map[string]any{"type": "string", "description": "Testable claim about the root cause"},
				"impact":      map[string]any{"type": "string", "description": "Expected severity if true", "enum": []string{"critical", "high", "medium", "low"}},
				"confidence":  map[string]any{"type": "number", "description": "Estimated likelihood, 0.0 to 1.0"},
				"next_checks": map[string]any{"type": "array", "description": "Concrete steps to validate or refute this hypothesis", "items": map[string]any{"type": "string"}},
			},
		}},
		{Name: "session_update_hypothesis", Description: "Update a hypothesis with new status, confidence, linked findings, or additional checks.", InputSchema: map[string]any{
			"type": "object", "required": []string{"id"},
			"properties": map[string]any{
				"id":                        map[string]any{"type": "string", "description": "Hypothesis ID"},
				"status":                    map[string]any{"type": "string", "description": "New status: open, supported, contradicted, confirmed, or rejected", "enum": []string{"open", "supported", "contradicted", "confirmed", "rejected"}},
				"confidence":                map[string]any{"type": "number", "description": "Updated confidence, 0.0 to 1.0"},
				"supporting_finding_ids":    map[string]any{"type": "array", "description": "Finding IDs that support this hypothesis", "items": map[string]any{"type": "string"}},
				"contradicting_finding_ids": map[string]any{"type": "array", "description": "Finding IDs that contradict this hypothesis", "items": map[string]any{"type": "string"}},
				"next_checks":               map[string]any{"type": "array", "description": "Additional steps to investigate", "items": map[string]any{"type": "string"}},
			},
		}},
		{Name: "session_rank_hypotheses", Description: "Return hypotheses ranked by status priority (supported > open > contradicted > confirmed > rejected) then by confidence descending.", InputSchema: map[string]any{
			"type": "object", "required": []string{"session_id"},
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string", "description": "Session ID"},
			},
		}},
		{Name: "session_recommend_next_step", Description: "Get recommended next investigative actions derived from pending hypothesis checks.", InputSchema: map[string]any{
			"type": "object", "required": []string{"session_id"},
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string", "description": "Session ID"},
				"count":      map[string]any{"type": "integer", "description": "Maximum number of recommendations (default 3)"},
			},
		}},
		{Name: "session_generate_summary", Description: "Generate a markdown summary. Handoff mode includes next steps; postmortem-draft mode includes timeline.", InputSchema: map[string]any{
			"type": "object", "required": []string{"session_id"},
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string", "description": "Session ID"},
				"mode":       map[string]any{"type": "string", "description": "Summary mode", "enum": []string{"handoff", "postmortem-draft"}},
			},
		}},
		{Name: "session_get_timeline", Description: "Get chronological list of session events (session_started, finding_added, hypothesis_added, etc.).", InputSchema: map[string]any{
			"type": "object", "required": []string{"session_id"},
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string", "description": "Session ID"},
			},
		}},
		{Name: "session_close", Description: "Close the investigation session with a final status and outcome description.", InputSchema: map[string]any{
			"type": "object", "required": []string{"session_id", "status"},
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string", "description": "Session ID"},
				"status":     map[string]any{"type": "string", "description": "Final status", "enum": []string{"resolved", "mitigated", "abandoned", "needs-followup"}},
				"outcome":    map[string]any{"type": "string", "description": "What was done to resolve or mitigate the issue"},
			},
		}},
	}
}
