package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vlsi/troubleshooting-cli/internal/app"
	"github.com/vlsi/troubleshooting-cli/internal/domain"
	"github.com/vlsi/troubleshooting-cli/internal/storage"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := storage.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	counter := 0
	svc := app.NewService(store, func() string {
		counter++
		return fmt.Sprintf("test-id-%d", counter)
	})
	return &Server{svc: svc}
}

func call(t *testing.T, srv *Server, method string, id any, params any) Response {
	t.Helper()
	paramsBytes, _ := json.Marshal(params)
	req := Request{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  paramsBytes,
	}
	line, _ := json.Marshal(req)
	in := bytes.NewReader(append(line, '\n'))
	out := new(bytes.Buffer)
	srv.Run(in, out)

	var resp Response
	if out.Len() == 0 {
		return resp
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response JSON: %v\n%s", err, out.String())
	}
	return resp
}

func toolCall(t *testing.T, srv *Server, toolName string, args any) Response {
	t.Helper()
	return call(t, srv, "tools/call", 1, map[string]any{
		"name":      toolName,
		"arguments": args,
	})
}

func extractContent(t *testing.T, resp Response) string {
	t.Helper()
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("result is not a map: %T", resp.Result)
	}
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatal("no content in result")
	}
	first := content[0].(map[string]any)
	return first["text"].(string)
}

func TestMCPInitialize(t *testing.T) {
	srv := testServer(t)
	resp := call(t, srv, "initialize", 1, nil)
	if resp.Error != nil {
		t.Fatalf("initialize error: %s", resp.Error.Message)
	}
	result := resp.Result.(map[string]any)
	info := result["serverInfo"].(map[string]any)
	if info["name"] != "troubleshooting-mcp" {
		t.Errorf("unexpected server name: %v", info["name"])
	}
}

func TestMCPToolsList(t *testing.T) {
	srv := testServer(t)
	resp := call(t, srv, "tools/list", 1, nil)
	if resp.Error != nil {
		t.Fatalf("tools/list error: %s", resp.Error.Message)
	}
	result := resp.Result.(map[string]any)
	tools := result["tools"].([]any)
	names := make(map[string]bool)
	for _, tool := range tools {
		name := tool.(map[string]any)["name"].(string)
		names[name] = true
	}
	for _, expected := range []string{"session_start", "session_get_state", "session_add_finding"} {
		if !names[expected] {
			t.Errorf("missing tool: %s", expected)
		}
	}
}

func TestMCPSessionStart(t *testing.T) {
	srv := testServer(t)
	resp := toolCall(t, srv, "session_start", map[string]any{
		"title":         "pod crash",
		"service":       "api-gateway",
		"environment":   "production",
		"incident_hint": "INC-99",
	})
	if resp.Error != nil {
		t.Fatalf("error: %s", resp.Error.Message)
	}
	text := extractContent(t, resp)
	var sess domain.Session
	if err := json.Unmarshal([]byte(text), &sess); err != nil {
		t.Fatalf("invalid session JSON: %v", err)
	}
	if sess.ID == "" {
		t.Error("session ID must not be empty")
	}
	if sess.Title != "pod crash" {
		t.Errorf("title mismatch: %q", sess.Title)
	}
	if sess.Status != domain.SessionOpen {
		t.Errorf("status mismatch: %q", sess.Status)
	}
}

func TestMCPGetState(t *testing.T) {
	srv := testServer(t)
	// Start session
	resp := toolCall(t, srv, "session_start", map[string]any{
		"title": "test", "service": "svc", "environment": "dev",
	})
	var sess domain.Session
	json.Unmarshal([]byte(extractContent(t, resp)), &sess)

	// Get state
	resp = toolCall(t, srv, "session_get_state", map[string]any{
		"session_id": sess.ID,
	})
	if resp.Error != nil {
		t.Fatalf("error: %s", resp.Error.Message)
	}
	text := extractContent(t, resp)
	var state domain.SessionState
	if err := json.Unmarshal([]byte(text), &state); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if state.Session.ID != sess.ID {
		t.Error("session ID mismatch")
	}
	if len(state.Timeline) == 0 {
		t.Error("expected timeline events")
	}
}

func TestMCPGetStateNotFound(t *testing.T) {
	srv := testServer(t)
	resp := toolCall(t, srv, "session_get_state", map[string]any{
		"session_id": "nonexistent",
	})
	result := resp.Result.(map[string]any)
	if isErr, ok := result["isError"]; !ok || isErr != true {
		t.Error("expected isError=true for nonexistent session")
	}
}

func TestMCPAddFinding(t *testing.T) {
	srv := testServer(t)
	// Start session
	resp := toolCall(t, srv, "session_start", map[string]any{
		"title": "test", "service": "svc", "environment": "dev",
	})
	var sess domain.Session
	json.Unmarshal([]byte(extractContent(t, resp)), &sess)

	// Add finding
	resp = toolCall(t, srv, "session_add_finding", map[string]any{
		"session_id": sess.ID,
		"kind":       "observation",
		"summary":    "high latency on /health",
		"details":    "p99 at 1.8s",
		"importance": "high",
		"tags":       []string{"latency", "health"},
		"evidence": []map[string]any{
			{"type": "log", "pointer": "/var/log/app.log", "snippet": "timeout after 2s"},
		},
	})
	if resp.Error != nil {
		t.Fatalf("error: %s", resp.Error.Message)
	}
	text := extractContent(t, resp)
	var finding domain.Finding
	if err := json.Unmarshal([]byte(text), &finding); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if finding.ID == "" {
		t.Error("finding ID must not be empty")
	}
	if finding.Summary != "high latency on /health" {
		t.Errorf("summary mismatch: %q", finding.Summary)
	}
	if finding.Importance != "high" {
		t.Errorf("importance mismatch: %q", finding.Importance)
	}
	if len(finding.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(finding.Tags))
	}
	if len(finding.EvidenceRefs) != 1 {
		t.Fatalf("expected 1 evidence ref, got %d", len(finding.EvidenceRefs))
	}
	if finding.EvidenceRefs[0].Snippet != "timeout after 2s" {
		t.Errorf("evidence snippet mismatch: %q", finding.EvidenceRefs[0].Snippet)
	}
}

func TestMCPAddFindingBadSession(t *testing.T) {
	srv := testServer(t)
	resp := toolCall(t, srv, "session_add_finding", map[string]any{
		"session_id": "nonexistent",
		"kind":       "observation",
		"summary":    "something",
	})
	result := resp.Result.(map[string]any)
	if isErr, ok := result["isError"]; !ok || isErr != true {
		t.Error("expected isError=true for bad session")
	}
}

func TestMCPAddHypothesis(t *testing.T) {
	srv := testServer(t)
	resp := toolCall(t, srv, "session_start", map[string]any{
		"title": "test", "service": "svc", "environment": "dev",
	})
	var sess domain.Session
	json.Unmarshal([]byte(extractContent(t, resp)), &sess)

	resp = toolCall(t, srv, "session_add_hypothesis", map[string]any{
		"session_id":  sess.ID,
		"statement":   "DB pool exhausted",
		"impact":      "high",
		"confidence":  0.75,
		"next_checks": []string{"check pool metrics", "review limits"},
	})
	if resp.Error != nil {
		t.Fatalf("error: %s", resp.Error.Message)
	}
	var h domain.Hypothesis
	json.Unmarshal([]byte(extractContent(t, resp)), &h)
	if h.ID == "" {
		t.Error("hypothesis ID must not be empty")
	}
	if h.Statement != "DB pool exhausted" {
		t.Errorf("statement mismatch: %q", h.Statement)
	}
	if h.Status != domain.HypothesisOpen {
		t.Errorf("expected open, got %q", h.Status)
	}
	if h.Confidence == nil || *h.Confidence != 0.75 {
		t.Error("confidence mismatch")
	}
	if len(h.NextChecks) != 2 {
		t.Errorf("expected 2 next checks, got %d", len(h.NextChecks))
	}
}

func TestMCPAddHypothesisBadSession(t *testing.T) {
	srv := testServer(t)
	resp := toolCall(t, srv, "session_add_hypothesis", map[string]any{
		"session_id": "nonexistent",
		"statement":  "something",
	})
	result := resp.Result.(map[string]any)
	if isErr, ok := result["isError"]; !ok || isErr != true {
		t.Error("expected isError=true")
	}
}

func TestMCPUpdateHypothesis(t *testing.T) {
	srv := testServer(t)
	resp := toolCall(t, srv, "session_start", map[string]any{
		"title": "test", "service": "svc", "environment": "dev",
	})
	var sess domain.Session
	json.Unmarshal([]byte(extractContent(t, resp)), &sess)

	// Add finding + hypothesis
	resp = toolCall(t, srv, "session_add_finding", map[string]any{
		"session_id": sess.ID, "kind": "observation", "summary": "high latency",
	})
	var f domain.Finding
	json.Unmarshal([]byte(extractContent(t, resp)), &f)

	resp = toolCall(t, srv, "session_add_hypothesis", map[string]any{
		"session_id": sess.ID, "statement": "pool issue",
	})
	var h domain.Hypothesis
	json.Unmarshal([]byte(extractContent(t, resp)), &h)

	// Update
	supported := "supported"
	resp = toolCall(t, srv, "session_update_hypothesis", map[string]any{
		"id":                     h.ID,
		"status":                 supported,
		"confidence":             0.9,
		"supporting_finding_ids": []string{f.ID},
		"next_checks":            []string{"verify fix"},
	})
	if resp.Error != nil {
		t.Fatalf("error: %s", resp.Error.Message)
	}
	var updated domain.Hypothesis
	json.Unmarshal([]byte(extractContent(t, resp)), &updated)
	if updated.Status != domain.HypothesisSupported {
		t.Errorf("expected supported, got %q", updated.Status)
	}
	if updated.Confidence == nil || *updated.Confidence != 0.9 {
		t.Error("confidence not updated")
	}
	if len(updated.SupportingFindingIDs) != 1 || updated.SupportingFindingIDs[0] != f.ID {
		t.Errorf("supporting findings mismatch: %v", updated.SupportingFindingIDs)
	}
	if len(updated.NextChecks) != 1 {
		t.Errorf("expected 1 next check, got %d", len(updated.NextChecks))
	}
}

func TestMCPUpdateHypothesisContradicting(t *testing.T) {
	srv := testServer(t)
	resp := toolCall(t, srv, "session_start", map[string]any{
		"title": "test", "service": "svc", "environment": "dev",
	})
	var sess domain.Session
	json.Unmarshal([]byte(extractContent(t, resp)), &sess)

	resp = toolCall(t, srv, "session_add_hypothesis", map[string]any{
		"session_id": sess.ID, "statement": "goroutine leak",
	})
	var h domain.Hypothesis
	json.Unmarshal([]byte(extractContent(t, resp)), &h)

	resp = toolCall(t, srv, "session_update_hypothesis", map[string]any{
		"id":                        h.ID,
		"status":                    "contradicted",
		"contradicting_finding_ids": []string{"f1", "f2"},
	})
	var updated domain.Hypothesis
	json.Unmarshal([]byte(extractContent(t, resp)), &updated)
	if updated.Status != domain.HypothesisContradicted {
		t.Errorf("expected contradicted, got %q", updated.Status)
	}
	if len(updated.ContradictingFindingIDs) != 2 {
		t.Errorf("expected 2 contradicting, got %d", len(updated.ContradictingFindingIDs))
	}
}

func TestMCPUpdateHypothesisNotFound(t *testing.T) {
	srv := testServer(t)
	resp := toolCall(t, srv, "session_update_hypothesis", map[string]any{
		"id":     "nonexistent",
		"status": "supported",
	})
	result := resp.Result.(map[string]any)
	if isErr, ok := result["isError"]; !ok || isErr != true {
		t.Error("expected isError=true")
	}
}

func TestMCPRankHypotheses(t *testing.T) {
	srv := testServer(t)
	resp := toolCall(t, srv, "session_start", map[string]any{
		"title": "test", "service": "svc", "environment": "dev",
	})
	var sess domain.Session
	json.Unmarshal([]byte(extractContent(t, resp)), &sess)

	// Add 3 hypotheses
	resp = toolCall(t, srv, "session_add_hypothesis", map[string]any{
		"session_id": sess.ID, "statement": "low conf open", "confidence": 0.2,
	})
	var hLow domain.Hypothesis
	json.Unmarshal([]byte(extractContent(t, resp)), &hLow)

	resp = toolCall(t, srv, "session_add_hypothesis", map[string]any{
		"session_id": sess.ID, "statement": "high conf open", "confidence": 0.9,
	})
	var hHigh domain.Hypothesis
	json.Unmarshal([]byte(extractContent(t, resp)), &hHigh)

	resp = toolCall(t, srv, "session_add_hypothesis", map[string]any{
		"session_id": sess.ID, "statement": "supported", "confidence": 0.5,
	})
	var hSup domain.Hypothesis
	json.Unmarshal([]byte(extractContent(t, resp)), &hSup)

	// Mark one supported
	toolCall(t, srv, "session_update_hypothesis", map[string]any{
		"id": hSup.ID, "status": "supported",
	})

	// Rank
	resp = toolCall(t, srv, "session_rank_hypotheses", map[string]any{
		"session_id": sess.ID,
	})
	if resp.Error != nil {
		t.Fatalf("error: %s", resp.Error.Message)
	}
	var ranked []domain.Hypothesis
	json.Unmarshal([]byte(extractContent(t, resp)), &ranked)
	if len(ranked) != 3 {
		t.Fatalf("expected 3, got %d", len(ranked))
	}
	// supported first, then open by confidence desc
	if ranked[0].ID != hSup.ID {
		t.Errorf("rank 0: expected supported, got %q (status=%s)", ranked[0].Statement, ranked[0].Status)
	}
	if ranked[1].ID != hHigh.ID {
		t.Errorf("rank 1: expected high conf open, got %q", ranked[1].Statement)
	}
	if ranked[2].ID != hLow.ID {
		t.Errorf("rank 2: expected low conf open, got %q", ranked[2].Statement)
	}
}

func TestMCPRankHypothesesEmpty(t *testing.T) {
	srv := testServer(t)
	resp := toolCall(t, srv, "session_start", map[string]any{
		"title": "test", "service": "svc", "environment": "dev",
	})
	var sess domain.Session
	json.Unmarshal([]byte(extractContent(t, resp)), &sess)

	resp = toolCall(t, srv, "session_rank_hypotheses", map[string]any{
		"session_id": sess.ID,
	})
	var ranked []domain.Hypothesis
	json.Unmarshal([]byte(extractContent(t, resp)), &ranked)
	if len(ranked) != 0 {
		t.Errorf("expected 0, got %d", len(ranked))
	}
}

func TestMCPHypothesisFullFlow(t *testing.T) {
	srv := testServer(t)

	// Start session
	resp := toolCall(t, srv, "session_start", map[string]any{
		"title": "OOM investigation", "service": "worker", "environment": "prod",
	})
	var sess domain.Session
	json.Unmarshal([]byte(extractContent(t, resp)), &sess)

	// Add findings
	resp = toolCall(t, srv, "session_add_finding", map[string]any{
		"session_id": sess.ID, "kind": "observation",
		"summary": "RSS growing", "importance": "high",
	})
	var f1 domain.Finding
	json.Unmarshal([]byte(extractContent(t, resp)), &f1)

	resp = toolCall(t, srv, "session_add_finding", map[string]any{
		"session_id": sess.ID, "kind": "observation",
		"summary": "goroutine count stable",
	})
	var f2 domain.Finding
	json.Unmarshal([]byte(extractContent(t, resp)), &f2)

	// Add hypotheses
	resp = toolCall(t, srv, "session_add_hypothesis", map[string]any{
		"session_id": sess.ID, "statement": "memory leak in handler",
		"confidence": 0.8, "next_checks": []string{"heap profile"},
	})
	var h1 domain.Hypothesis
	json.Unmarshal([]byte(extractContent(t, resp)), &h1)

	resp = toolCall(t, srv, "session_add_hypothesis", map[string]any{
		"session_id": sess.ID, "statement": "goroutine leak",
		"confidence": 0.3,
	})
	var h2 domain.Hypothesis
	json.Unmarshal([]byte(extractContent(t, resp)), &h2)

	// Update hypotheses with evidence links
	toolCall(t, srv, "session_update_hypothesis", map[string]any{
		"id": h1.ID, "status": "supported",
		"supporting_finding_ids": []string{f1.ID},
	})
	toolCall(t, srv, "session_update_hypothesis", map[string]any{
		"id": h2.ID, "status": "contradicted",
		"contradicting_finding_ids": []string{f2.ID},
	})

	// Rank: supported should be first
	resp = toolCall(t, srv, "session_rank_hypotheses", map[string]any{
		"session_id": sess.ID,
	})
	var ranked []domain.Hypothesis
	json.Unmarshal([]byte(extractContent(t, resp)), &ranked)
	if len(ranked) != 2 {
		t.Fatalf("expected 2, got %d", len(ranked))
	}
	if ranked[0].ID != h1.ID {
		t.Error("expected supported hypothesis first")
	}
	if len(ranked[0].SupportingFindingIDs) != 1 {
		t.Error("expected supporting finding linked")
	}
	if ranked[1].ID != h2.ID {
		t.Error("expected contradicted hypothesis second")
	}
	if len(ranked[1].ContradictingFindingIDs) != 1 {
		t.Error("expected contradicting finding linked")
	}

	// Verify get-state has everything
	resp = toolCall(t, srv, "session_get_state", map[string]any{"session_id": sess.ID})
	var state domain.SessionState
	json.Unmarshal([]byte(extractContent(t, resp)), &state)
	if len(state.Findings) != 2 {
		t.Errorf("expected 2 findings, got %d", len(state.Findings))
	}
	if len(state.Hypotheses) != 2 {
		t.Errorf("expected 2 hypotheses, got %d", len(state.Hypotheses))
	}
	// session_started + 2 findings + 2 hypotheses + 2 updates = 7
	if len(state.Timeline) < 7 {
		t.Errorf("expected >=7 timeline events, got %d", len(state.Timeline))
	}
}

func TestMCPRecommendNextStep(t *testing.T) {
	srv := testServer(t)
	resp := toolCall(t, srv, "session_start", map[string]any{
		"title": "test", "service": "svc", "environment": "dev",
	})
	var sess domain.Session
	json.Unmarshal([]byte(extractContent(t, resp)), &sess)

	// Add a finding so "gather findings" recommendation doesn't appear
	toolCall(t, srv, "session_add_finding", map[string]any{
		"session_id": sess.ID, "kind": "observation", "summary": "baseline",
	})

	toolCall(t, srv, "session_add_hypothesis", map[string]any{
		"session_id":  sess.ID,
		"statement":   "pool exhaustion",
		"next_checks": []string{"check pool metrics", "review limits"},
	})

	resp = toolCall(t, srv, "session_recommend_next_step", map[string]any{
		"session_id": sess.ID,
	})
	if resp.Error != nil {
		t.Fatalf("error: %s", resp.Error.Message)
	}
	var recs []domain.Recommendation
	json.Unmarshal([]byte(extractContent(t, resp)), &recs)
	if len(recs) != 2 {
		t.Fatalf("expected 2 recommendations, got %d", len(recs))
	}
	if recs[0].Action != "check pool metrics" {
		t.Errorf("unexpected action: %q", recs[0].Action)
	}
	if recs[0].Reason == "" || recs[0].Goal == "" {
		t.Error("reason and goal must not be empty")
	}
}

func TestMCPRecommendNextStepWithLimit(t *testing.T) {
	srv := testServer(t)
	resp := toolCall(t, srv, "session_start", map[string]any{
		"title": "test", "service": "svc", "environment": "dev",
	})
	var sess domain.Session
	json.Unmarshal([]byte(extractContent(t, resp)), &sess)

	toolCall(t, srv, "session_add_hypothesis", map[string]any{
		"session_id":  sess.ID,
		"statement":   "h1",
		"next_checks": []string{"a", "b", "c", "d"},
	})

	resp = toolCall(t, srv, "session_recommend_next_step", map[string]any{
		"session_id": sess.ID,
		"count":      2,
	})
	var recs []domain.Recommendation
	json.Unmarshal([]byte(extractContent(t, resp)), &recs)
	if len(recs) != 2 {
		t.Errorf("expected 2 with count=2, got %d", len(recs))
	}
}

func TestMCPRecommendNoFindings(t *testing.T) {
	srv := testServer(t)
	resp := toolCall(t, srv, "session_start", map[string]any{
		"title": "test", "service": "svc", "environment": "dev",
	})
	var sess domain.Session
	json.Unmarshal([]byte(extractContent(t, resp)), &sess)

	resp = toolCall(t, srv, "session_recommend_next_step", map[string]any{
		"session_id": sess.ID,
	})
	var recs []domain.Recommendation
	json.Unmarshal([]byte(extractContent(t, resp)), &recs)
	if len(recs) == 0 {
		t.Fatal("expected at least one recommendation")
	}
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

func TestMCPRecommendSkipsTerminal(t *testing.T) {
	srv := testServer(t)
	resp := toolCall(t, srv, "session_start", map[string]any{
		"title": "test", "service": "svc", "environment": "dev",
	})
	var sess domain.Session
	json.Unmarshal([]byte(extractContent(t, resp)), &sess)

	// Add confirmed hypothesis with checks — checks should not appear
	resp = toolCall(t, srv, "session_add_hypothesis", map[string]any{
		"session_id":  sess.ID,
		"statement":   "confirmed",
		"next_checks": []string{"should not appear"},
	})
	var h domain.Hypothesis
	json.Unmarshal([]byte(extractContent(t, resp)), &h)
	toolCall(t, srv, "session_update_hypothesis", map[string]any{
		"id": h.ID, "status": "confirmed",
	})

	// Add open hypothesis with checks — these should appear
	toolCall(t, srv, "session_add_hypothesis", map[string]any{
		"session_id":  sess.ID,
		"statement":   "open one",
		"next_checks": []string{"should appear"},
	})

	resp = toolCall(t, srv, "session_recommend_next_step", map[string]any{
		"session_id": sess.ID,
	})
	var recs []domain.Recommendation
	json.Unmarshal([]byte(extractContent(t, resp)), &recs)
	for _, r := range recs {
		if r.Action == "should not appear" {
			t.Error("should not recommend checks from confirmed hypothesis")
		}
	}
	if len(recs) == 0 || recs[0].Action != "should appear" {
		t.Error("expected check from open hypothesis")
	}
}

func TestMCPGenerateSummaryHandoff(t *testing.T) {
	srv := testServer(t)
	resp := toolCall(t, srv, "session_start", map[string]any{
		"title": "latency spike", "service": "api-gw", "environment": "prod",
		"incident_hint": "INC-99",
	})
	var sess domain.Session
	json.Unmarshal([]byte(extractContent(t, resp)), &sess)

	// Add finding with evidence
	toolCall(t, srv, "session_add_finding", map[string]any{
		"session_id": sess.ID, "kind": "observation",
		"summary": "p99 at 2.3s", "details": "Started at 14:30 UTC",
		"importance": "high",
		"evidence": []map[string]any{
			{"type": "log", "pointer": "/var/log/api.log", "snippet": "timeout"},
		},
	})

	// Add hypothesis
	toolCall(t, srv, "session_add_hypothesis", map[string]any{
		"session_id": sess.ID, "statement": "upstream DB slow",
		"confidence": 0.7, "next_checks": []string{"check DB latency"},
	})

	resp = toolCall(t, srv, "session_generate_summary", map[string]any{
		"session_id": sess.ID, "mode": "handoff",
	})
	if resp.Error != nil {
		t.Fatalf("error: %s", resp.Error.Message)
	}
	// Summary is returned as a string in content text
	md := extractContent(t, resp)
	// MCP returns the summary as a JSON string, unwrap it
	var mdStr string
	if err := json.Unmarshal([]byte(md), &mdStr); err != nil {
		mdStr = md // fallback: might be bare text
	}
	if !strings.Contains(mdStr, "# Handoff: latency spike") {
		t.Error("expected handoff header")
	}
	if !strings.Contains(mdStr, "INC-99") {
		t.Error("expected incident hint")
	}
	if !strings.Contains(mdStr, "p99 at 2.3s") {
		t.Error("expected finding")
	}
	if !strings.Contains(mdStr, "/var/log/api.log") {
		t.Error("expected evidence pointer")
	}
	if !strings.Contains(mdStr, "upstream DB slow") {
		t.Error("expected hypothesis")
	}
	if !strings.Contains(mdStr, "Recommended Next Steps") {
		t.Error("expected next steps in handoff")
	}
	if !strings.Contains(mdStr, "check DB latency") {
		t.Error("expected next check in recommendations")
	}
}

func TestMCPGenerateSummaryPostmortem(t *testing.T) {
	srv := testServer(t)
	resp := toolCall(t, srv, "session_start", map[string]any{
		"title": "outage", "service": "payments", "environment": "prod",
	})
	var sess domain.Session
	json.Unmarshal([]byte(extractContent(t, resp)), &sess)

	toolCall(t, srv, "session_add_finding", map[string]any{
		"session_id": sess.ID, "kind": "error", "summary": "circuit breaker tripped",
	})
	toolCall(t, srv, "session_add_hypothesis", map[string]any{
		"session_id": sess.ID, "statement": "downstream timeout",
	})
	toolCall(t, srv, "session_close", map[string]any{
		"session_id": sess.ID, "status": "resolved", "outcome": "increased timeout",
	})

	resp = toolCall(t, srv, "session_generate_summary", map[string]any{
		"session_id": sess.ID, "mode": "postmortem-draft",
	})
	md := extractContent(t, resp)
	var mdStr string
	if err := json.Unmarshal([]byte(md), &mdStr); err != nil {
		mdStr = md
	}
	if !strings.Contains(mdStr, "# Postmortem Draft: outage") {
		t.Error("expected postmortem header")
	}
	if !strings.Contains(mdStr, "resolved") {
		t.Error("expected resolved status")
	}
	if !strings.Contains(mdStr, "increased timeout") {
		t.Error("expected outcome")
	}
	if !strings.Contains(mdStr, "## Timeline") {
		t.Error("expected timeline")
	}
	if strings.Contains(mdStr, "Recommended Next Steps") {
		t.Error("postmortem should not include next steps")
	}
}

func TestMCPGenerateSummaryBadSession(t *testing.T) {
	srv := testServer(t)
	resp := toolCall(t, srv, "session_generate_summary", map[string]any{
		"session_id": "nonexistent", "mode": "handoff",
	})
	result := resp.Result.(map[string]any)
	if isErr, ok := result["isError"]; !ok || isErr != true {
		t.Error("expected isError=true")
	}
}

func TestMCPRecommendBadSession(t *testing.T) {
	srv := testServer(t)
	resp := toolCall(t, srv, "session_recommend_next_step", map[string]any{
		"session_id": "nonexistent",
	})
	result := resp.Result.(map[string]any)
	if isErr, ok := result["isError"]; !ok || isErr != true {
		t.Error("expected isError=true")
	}
}

func TestMCPGetTimeline(t *testing.T) {
	srv := testServer(t)
	resp := toolCall(t, srv, "session_start", map[string]any{
		"title": "test", "service": "svc", "environment": "dev",
	})
	var sess domain.Session
	json.Unmarshal([]byte(extractContent(t, resp)), &sess)

	toolCall(t, srv, "session_add_finding", map[string]any{
		"session_id": sess.ID, "kind": "observation", "summary": "finding A",
	})
	toolCall(t, srv, "session_add_hypothesis", map[string]any{
		"session_id": sess.ID, "statement": "hypothesis B",
	})

	resp = toolCall(t, srv, "session_get_timeline", map[string]any{
		"session_id": sess.ID,
	})
	if resp.Error != nil {
		t.Fatalf("error: %s", resp.Error.Message)
	}
	var events []domain.TimelineEvent
	json.Unmarshal([]byte(extractContent(t, resp)), &events)
	if len(events) < 3 {
		t.Fatalf("expected at least 3 events, got %d", len(events))
	}
	if events[0].Kind != "session_started" {
		t.Errorf("first: expected session_started, got %s", events[0].Kind)
	}
	if events[1].Kind != "finding_added" {
		t.Errorf("second: expected finding_added, got %s", events[1].Kind)
	}
	if events[2].Kind != "hypothesis_added" {
		t.Errorf("third: expected hypothesis_added, got %s", events[2].Kind)
	}
	for _, e := range events {
		if e.ID == "" || e.SessionID == "" || e.Summary == "" {
			t.Error("event fields must not be empty")
		}
	}
}

func TestMCPGetTimelineNotFound(t *testing.T) {
	srv := testServer(t)
	resp := toolCall(t, srv, "session_get_timeline", map[string]any{
		"session_id": "nonexistent",
	})
	result := resp.Result.(map[string]any)
	if isErr, ok := result["isError"]; !ok || isErr != true {
		t.Error("expected isError=true")
	}
}

func TestMCPClose(t *testing.T) {
	srv := testServer(t)
	resp := toolCall(t, srv, "session_start", map[string]any{
		"title": "test", "service": "svc", "environment": "dev",
	})
	var sess domain.Session
	json.Unmarshal([]byte(extractContent(t, resp)), &sess)

	resp = toolCall(t, srv, "session_close", map[string]any{
		"session_id": sess.ID,
		"status":     "resolved",
		"outcome":    "fixed the config",
	})
	if resp.Error != nil {
		t.Fatalf("error: %s", resp.Error.Message)
	}
	var closed domain.Session
	json.Unmarshal([]byte(extractContent(t, resp)), &closed)
	if closed.Status != domain.SessionResolved {
		t.Errorf("expected resolved, got %q", closed.Status)
	}
	if closed.ClosedAt == nil {
		t.Error("closed_at must be set")
	}
	if closed.Outcome != "fixed the config" {
		t.Errorf("outcome mismatch: %q", closed.Outcome)
	}
}

func TestMCPCloseAllStatuses(t *testing.T) {
	srv := testServer(t)
	for _, status := range []string{"resolved", "mitigated", "abandoned", "needs-followup"} {
		t.Run(status, func(t *testing.T) {
			resp := toolCall(t, srv, "session_start", map[string]any{
				"title": "test-" + status, "service": "svc", "environment": "dev",
			})
			var sess domain.Session
			json.Unmarshal([]byte(extractContent(t, resp)), &sess)

			resp = toolCall(t, srv, "session_close", map[string]any{
				"session_id": sess.ID, "status": status,
			})
			var closed domain.Session
			json.Unmarshal([]byte(extractContent(t, resp)), &closed)
			if string(closed.Status) != status {
				t.Errorf("expected %s, got %s", status, closed.Status)
			}
		})
	}
}

func TestMCPCloseAlreadyClosed(t *testing.T) {
	srv := testServer(t)
	resp := toolCall(t, srv, "session_start", map[string]any{
		"title": "test", "service": "svc", "environment": "dev",
	})
	var sess domain.Session
	json.Unmarshal([]byte(extractContent(t, resp)), &sess)

	toolCall(t, srv, "session_close", map[string]any{
		"session_id": sess.ID, "status": "resolved",
	})

	resp = toolCall(t, srv, "session_close", map[string]any{
		"session_id": sess.ID, "status": "mitigated",
	})
	result := resp.Result.(map[string]any)
	if isErr, ok := result["isError"]; !ok || isErr != true {
		t.Error("expected isError=true when closing already-closed session")
	}
}

func TestMCPCloseNotFound(t *testing.T) {
	srv := testServer(t)
	resp := toolCall(t, srv, "session_close", map[string]any{
		"session_id": "nonexistent", "status": "resolved",
	})
	result := resp.Result.(map[string]any)
	if isErr, ok := result["isError"]; !ok || isErr != true {
		t.Error("expected isError=true")
	}
}

func TestMCPCloseTimelineAndState(t *testing.T) {
	srv := testServer(t)
	resp := toolCall(t, srv, "session_start", map[string]any{
		"title": "investigation", "service": "api", "environment": "prod",
	})
	var sess domain.Session
	json.Unmarshal([]byte(extractContent(t, resp)), &sess)

	toolCall(t, srv, "session_add_finding", map[string]any{
		"session_id": sess.ID, "kind": "observation", "summary": "found it",
	})
	toolCall(t, srv, "session_close", map[string]any{
		"session_id": sess.ID, "status": "resolved", "outcome": "deployed fix",
	})

	// Verify timeline
	resp = toolCall(t, srv, "session_get_timeline", map[string]any{"session_id": sess.ID})
	var events []domain.TimelineEvent
	json.Unmarshal([]byte(extractContent(t, resp)), &events)
	lastEvent := events[len(events)-1]
	if lastEvent.Kind != "session_closed" {
		t.Errorf("last event: expected session_closed, got %s", lastEvent.Kind)
	}
	if !strings.Contains(lastEvent.Summary, "resolved") {
		t.Errorf("expected resolved in summary, got %q", lastEvent.Summary)
	}

	// Verify state
	resp = toolCall(t, srv, "session_get_state", map[string]any{"session_id": sess.ID})
	var state domain.SessionState
	json.Unmarshal([]byte(extractContent(t, resp)), &state)
	if state.Session.Status != domain.SessionResolved {
		t.Errorf("expected resolved, got %s", state.Session.Status)
	}
	if state.Session.Outcome != "deployed fix" {
		t.Errorf("outcome mismatch: %q", state.Session.Outcome)
	}
}

func TestMCPFullSliceFlow(t *testing.T) {
	srv := testServer(t)

	// 1. Start
	resp := toolCall(t, srv, "session_start", map[string]any{
		"title": "connection failures", "service": "order-svc", "environment": "staging",
	})
	var sess domain.Session
	json.Unmarshal([]byte(extractContent(t, resp)), &sess)

	// 2. Add two findings
	toolCall(t, srv, "session_add_finding", map[string]any{
		"session_id": sess.ID, "kind": "error",
		"summary": "pool exhausted", "importance": "critical",
	})
	toolCall(t, srv, "session_add_finding", map[string]any{
		"session_id": sess.ID, "kind": "observation",
		"summary": "CPU normal", "importance": "low",
	})

	// 3. Get state and verify
	resp = toolCall(t, srv, "session_get_state", map[string]any{"session_id": sess.ID})
	text := extractContent(t, resp)
	var state domain.SessionState
	json.Unmarshal([]byte(text), &state)

	if len(state.Findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(state.Findings))
	}
	if state.Findings[0].Summary != "pool exhausted" {
		t.Errorf("first finding mismatch: %q", state.Findings[0].Summary)
	}
	if len(state.Timeline) < 3 {
		t.Errorf("expected >=3 timeline events, got %d", len(state.Timeline))
	}
}

func TestMCPUnknownMethod(t *testing.T) {
	srv := testServer(t)
	resp := call(t, srv, "bogus/method", 1, nil)
	if resp.Error == nil {
		t.Error("expected error for unknown method")
	}
	if !strings.Contains(resp.Error.Message, "method not found") {
		t.Errorf("unexpected error: %s", resp.Error.Message)
	}
}

func TestMCPUnknownTool(t *testing.T) {
	srv := testServer(t)
	resp := toolCall(t, srv, "nonexistent_tool", map[string]any{})
	result := resp.Result.(map[string]any)
	if isErr, ok := result["isError"]; !ok || isErr != true {
		t.Error("expected isError=true for unknown tool")
	}
}
