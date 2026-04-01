package main

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

func testServer(t *testing.T) *mcpServer {
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
	return &mcpServer{svc: svc}
}

func call(t *testing.T, srv *mcpServer, method string, id any, params any) jsonrpcResponse {
	t.Helper()
	paramsBytes, _ := json.Marshal(params)
	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  paramsBytes,
	}
	line, _ := json.Marshal(req)
	in := bytes.NewReader(append(line, '\n'))
	out := new(bytes.Buffer)
	srv.run(in, out)

	var resp jsonrpcResponse
	if out.Len() == 0 {
		return resp
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response JSON: %v\n%s", err, out.String())
	}
	return resp
}

func toolCall(t *testing.T, srv *mcpServer, toolName string, args any) jsonrpcResponse {
	t.Helper()
	return call(t, srv, "tools/call", 1, map[string]any{
		"name":      toolName,
		"arguments": args,
	})
}

func extractContent(t *testing.T, resp jsonrpcResponse) string {
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
