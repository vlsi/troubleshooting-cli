package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"

	"github.com/vlsi/troubleshooting-cli/internal/app"
	"github.com/vlsi/troubleshooting-cli/internal/domain"
)

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
			SessionID  string            `json:"session_id"`
			Kind       string            `json:"kind"`
			Summary    string            `json:"summary"`
			Details    string            `json:"details"`
			Importance string            `json:"importance"`
			Tags       []string          `json:"tags"`
			Evidence   []domain.Evidence `json:"evidence"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, err
		}
		return s.svc.AddFinding(p.SessionID, p.Kind, p.Summary, p.Details, p.Importance, p.Tags, p.Evidence)

	case "session_add_hypothesis":
		var p struct {
			SessionID  string   `json:"session_id"`
			Statement  string   `json:"statement"`
			Impact     string   `json:"impact"`
			Confidence *float64 `json:"confidence"`
			NextChecks []string `json:"next_checks"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, err
		}
		return s.svc.AddHypothesis(p.SessionID, p.Statement, p.Impact, p.Confidence, p.NextChecks)

	case "session_update_hypothesis":
		var p struct {
			ID         string                   `json:"id"`
			Status     *domain.HypothesisStatus `json:"status"`
			Confidence *float64                 `json:"confidence"`
			Support    []string                 `json:"supporting_finding_ids"`
			Contradict []string                 `json:"contradicting_finding_ids"`
			NextChecks []string                 `json:"next_checks"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, err
		}
		return s.svc.UpdateHypothesis(p.ID, p.Status, p.Confidence, p.Support, p.Contradict, p.NextChecks)

	case "session_rank_hypotheses":
		var p struct{ SessionID string `json:"session_id"` }
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, err
		}
		return s.svc.RankHypotheses(p.SessionID)

	case "session_recommend_next_step":
		var p struct {
			SessionID string `json:"session_id"`
			Count     int    `json:"count"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, err
		}
		if p.Count <= 0 {
			p.Count = 3
		}
		return s.svc.RecommendNextSteps(p.SessionID, p.Count)

	case "session_generate_summary":
		var p struct {
			SessionID string `json:"session_id"`
			Mode      string `json:"mode"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, err
		}
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
		{Name: "session_start", Description: "Start a new investigation session", InputSchema: map[string]any{
			"type": "object", "required": []string{"title", "service", "environment"},
			"properties": map[string]any{
				"title":         map[string]any{"type": "string"},
				"service":       map[string]any{"type": "string"},
				"environment":   map[string]any{"type": "string"},
				"incident_hint": map[string]any{"type": "string"},
				"labels":        map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
			},
		}},
		{Name: "session_get_state", Description: "Get full session state", InputSchema: map[string]any{
			"type": "object", "required": []string{"session_id"},
			"properties": map[string]any{"session_id": map[string]any{"type": "string"}},
		}},
		{Name: "session_add_finding", Description: "Add a finding to a session", InputSchema: map[string]any{
			"type": "object", "required": []string{"session_id", "kind", "summary"},
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"kind":       map[string]any{"type": "string"},
				"summary":    map[string]any{"type": "string"},
				"details":    map[string]any{"type": "string"},
				"importance": map[string]any{"type": "string"},
				"tags":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"evidence":   map[string]any{"type": "array"},
			},
		}},
		{Name: "session_add_hypothesis", Description: "Add a hypothesis to a session", InputSchema: map[string]any{
			"type": "object", "required": []string{"session_id", "statement"},
			"properties": map[string]any{
				"session_id":  map[string]any{"type": "string"},
				"statement":   map[string]any{"type": "string"},
				"impact":      map[string]any{"type": "string"},
				"confidence":  map[string]any{"type": "number"},
				"next_checks": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
		}},
		{Name: "session_update_hypothesis", Description: "Update a hypothesis", InputSchema: map[string]any{
			"type": "object", "required": []string{"id"},
			"properties": map[string]any{
				"id":                        map[string]any{"type": "string"},
				"status":                    map[string]any{"type": "string"},
				"confidence":                map[string]any{"type": "number"},
				"supporting_finding_ids":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"contradicting_finding_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"next_checks":               map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
		}},
		{Name: "session_rank_hypotheses", Description: "Rank hypotheses for a session", InputSchema: map[string]any{
			"type": "object", "required": []string{"session_id"},
			"properties": map[string]any{"session_id": map[string]any{"type": "string"}},
		}},
		{Name: "session_recommend_next_step", Description: "Get recommended next investigative steps", InputSchema: map[string]any{
			"type": "object", "required": []string{"session_id"},
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"count":      map[string]any{"type": "integer"},
			},
		}},
		{Name: "session_generate_summary", Description: "Generate a markdown summary for handoff or postmortem", InputSchema: map[string]any{
			"type": "object", "required": []string{"session_id"},
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"mode":       map[string]any{"type": "string", "enum": []string{"handoff", "postmortem-draft"}},
			},
		}},
		{Name: "session_get_timeline", Description: "Get session timeline events", InputSchema: map[string]any{
			"type": "object", "required": []string{"session_id"},
			"properties": map[string]any{"session_id": map[string]any{"type": "string"}},
		}},
		{Name: "session_close", Description: "Close a session with a final status", InputSchema: map[string]any{
			"type": "object", "required": []string{"session_id", "status"},
			"properties": map[string]any{
				"session_id": map[string]any{"type": "string"},
				"status":     map[string]any{"type": "string", "enum": []string{"resolved", "mitigated", "abandoned", "needs-followup"}},
				"outcome":    map[string]any{"type": "string"},
			},
		}},
	}
}
