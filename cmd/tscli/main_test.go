package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/vlsi/troubleshooting-cli/internal/domain"
)

func testDB(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "test.db")
}

func runCLI(t *testing.T, db string, args ...string) ([]byte, error) {
	t.Helper()
	cmd := rootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(new(bytes.Buffer))
	// Redirect stdout for printJSON which writes to os.Stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	cmd.SetArgs(append([]string{"--db", db}, args...))
	err := cmd.Execute()
	w.Close()
	os.Stdout = old
	out := new(bytes.Buffer)
	out.ReadFrom(r)
	return out.Bytes(), err
}

func TestCLISessionStart(t *testing.T) {
	db := testDB(t)
	out, err := runCLI(t, db, "session", "start",
		"--title", "pod crash loop",
		"--service", "api-gateway",
		"--env", "production",
		"--incident", "INC-42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var sess domain.Session
	if err := json.Unmarshal(out, &sess); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out)
	}
	if sess.ID == "" {
		t.Error("session ID must not be empty")
	}
	if sess.Title != "pod crash loop" {
		t.Errorf("expected title 'pod crash loop', got %q", sess.Title)
	}
	if sess.Service != "api-gateway" {
		t.Errorf("expected service 'api-gateway', got %q", sess.Service)
	}
	if sess.Environment != "production" {
		t.Errorf("expected env 'production', got %q", sess.Environment)
	}
	if sess.IncidentHint != "INC-42" {
		t.Errorf("expected incident hint 'INC-42', got %q", sess.IncidentHint)
	}
	if sess.Status != domain.SessionOpen {
		t.Errorf("expected status 'open', got %q", sess.Status)
	}
}

func TestCLISessionStartMissingRequired(t *testing.T) {
	db := testDB(t)
	_, err := runCLI(t, db, "session", "start", "--title", "test")
	if err == nil {
		t.Error("expected error for missing required flags")
	}
}

func TestCLIGetState(t *testing.T) {
	db := testDB(t)
	// Start a session first
	out, err := runCLI(t, db, "session", "start",
		"--title", "test", "--service", "svc", "--env", "dev")
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}
	var sess domain.Session
	json.Unmarshal(out, &sess)

	// Get state
	out, err = runCLI(t, db, "session", "get-state", "--id", sess.ID)
	if err != nil {
		t.Fatalf("get-state failed: %v", err)
	}
	var state domain.SessionState
	if err := json.Unmarshal(out, &state); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if state.Session.ID != sess.ID {
		t.Errorf("session ID mismatch: %s vs %s", state.Session.ID, sess.ID)
	}
	if state.Session.Title != "test" {
		t.Errorf("title mismatch: %q", state.Session.Title)
	}
	if len(state.Timeline) == 0 {
		t.Error("expected at least one timeline event (session_started)")
	}
}

func TestCLIGetStateNotFound(t *testing.T) {
	db := testDB(t)
	_, err := runCLI(t, db, "session", "get-state", "--id", "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestCLIAddFinding(t *testing.T) {
	db := testDB(t)
	// Start a session
	out, _ := runCLI(t, db, "session", "start",
		"--title", "test", "--service", "svc", "--env", "dev")
	var sess domain.Session
	json.Unmarshal(out, &sess)

	// Add a finding
	out, err := runCLI(t, db, "session", "add-finding",
		"--session", sess.ID,
		"--kind", "observation",
		"--summary", "high p99 latency on /api/v1/users",
		"--details", "Latency spiked to 2.3s at 14:30 UTC",
		"--importance", "high",
		"--tags", "latency,api")
	if err != nil {
		t.Fatalf("add-finding failed: %v", err)
	}
	var finding domain.Finding
	if err := json.Unmarshal(out, &finding); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if finding.ID == "" {
		t.Error("finding ID must not be empty")
	}
	if finding.SessionID != sess.ID {
		t.Error("session ID mismatch")
	}
	if finding.Kind != "observation" {
		t.Errorf("expected kind 'observation', got %q", finding.Kind)
	}
	if finding.Summary != "high p99 latency on /api/v1/users" {
		t.Errorf("summary mismatch: %q", finding.Summary)
	}
	if finding.Importance != "high" {
		t.Errorf("importance mismatch: %q", finding.Importance)
	}
	if len(finding.Tags) != 2 || finding.Tags[0] != "latency" {
		t.Errorf("tags mismatch: %v", finding.Tags)
	}
}

func TestCLIAddFindingWithEvidence(t *testing.T) {
	db := testDB(t)
	out, _ := runCLI(t, db, "session", "start",
		"--title", "test", "--service", "svc", "--env", "dev")
	var sess domain.Session
	json.Unmarshal(out, &sess)

	evidenceJSON := `[{"type":"log","pointer":"/var/log/app.log","snippet":"OOM killed"}]`
	out, err := runCLI(t, db, "session", "add-finding",
		"--session", sess.ID,
		"--summary", "OOM kill observed",
		"--evidence", evidenceJSON)
	if err != nil {
		t.Fatalf("add-finding with evidence failed: %v", err)
	}
	var finding domain.Finding
	json.Unmarshal(out, &finding)
	if len(finding.EvidenceRefs) != 1 {
		t.Fatalf("expected 1 evidence ref, got %d", len(finding.EvidenceRefs))
	}
	if finding.EvidenceRefs[0].Type != domain.EvidenceLog {
		t.Errorf("expected log evidence, got %q", finding.EvidenceRefs[0].Type)
	}
	if finding.EvidenceRefs[0].Snippet != "OOM killed" {
		t.Errorf("snippet mismatch: %q", finding.EvidenceRefs[0].Snippet)
	}
}

func TestCLIAddFindingBadSession(t *testing.T) {
	db := testDB(t)
	_, err := runCLI(t, db, "session", "add-finding",
		"--session", "nonexistent",
		"--summary", "something")
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestCLIFullSliceFlow(t *testing.T) {
	db := testDB(t)

	// 1. Start session
	out, err := runCLI(t, db, "session", "start",
		"--title", "DB connection failures",
		"--service", "order-service",
		"--env", "staging")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	var sess domain.Session
	json.Unmarshal(out, &sess)

	// 2. Add findings
	out, err = runCLI(t, db, "session", "add-finding",
		"--session", sess.ID,
		"--kind", "error",
		"--summary", "Connection pool exhausted",
		"--importance", "critical")
	if err != nil {
		t.Fatalf("add finding 1: %v", err)
	}

	out, err = runCLI(t, db, "session", "add-finding",
		"--session", sess.ID,
		"--kind", "observation",
		"--summary", "CPU usage normal",
		"--importance", "low")
	if err != nil {
		t.Fatalf("add finding 2: %v", err)
	}

	// 3. Verify state shows both findings
	out, err = runCLI(t, db, "session", "get-state", "--id", sess.ID)
	if err != nil {
		t.Fatalf("get-state: %v", err)
	}
	var state domain.SessionState
	json.Unmarshal(out, &state)

	if state.Session.Status != domain.SessionOpen {
		t.Errorf("expected open status, got %q", state.Session.Status)
	}
	if len(state.Findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(state.Findings))
	}
	// session_started + 2 finding_added = 3 timeline events
	if len(state.Timeline) < 3 {
		t.Errorf("expected at least 3 timeline events, got %d", len(state.Timeline))
	}
}
