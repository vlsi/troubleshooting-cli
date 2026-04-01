package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

func TestCLIAddHypothesis(t *testing.T) {
	db := testDB(t)
	out, _ := runCLI(t, db, "session", "start",
		"--title", "test", "--service", "svc", "--env", "dev")
	var sess domain.Session
	json.Unmarshal(out, &sess)

	out, err := runCLI(t, db, "session", "add-hypothesis",
		"--session", sess.ID,
		"--statement", "DB connection pool exhausted",
		"--impact", "high",
		"--confidence", "0.7",
		"--next-checks", "check pool metrics,review connection limits")
	if err != nil {
		t.Fatalf("add-hypothesis failed: %v", err)
	}
	var h domain.Hypothesis
	if err := json.Unmarshal(out, &h); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if h.ID == "" {
		t.Error("hypothesis ID must not be empty")
	}
	if h.Statement != "DB connection pool exhausted" {
		t.Errorf("statement mismatch: %q", h.Statement)
	}
	if h.Status != domain.HypothesisOpen {
		t.Errorf("expected open status, got %q", h.Status)
	}
	if h.Confidence == nil || *h.Confidence != 0.7 {
		t.Errorf("confidence mismatch: %v", h.Confidence)
	}
	if h.Impact != "high" {
		t.Errorf("impact mismatch: %q", h.Impact)
	}
	if len(h.NextChecks) != 2 {
		t.Errorf("expected 2 next checks, got %d", len(h.NextChecks))
	}
}

func TestCLIAddHypothesisNoConfidence(t *testing.T) {
	db := testDB(t)
	out, _ := runCLI(t, db, "session", "start",
		"--title", "test", "--service", "svc", "--env", "dev")
	var sess domain.Session
	json.Unmarshal(out, &sess)

	out, err := runCLI(t, db, "session", "add-hypothesis",
		"--session", sess.ID,
		"--statement", "memory leak")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var h domain.Hypothesis
	json.Unmarshal(out, &h)
	if h.Confidence != nil {
		t.Errorf("expected nil confidence when not provided, got %v", *h.Confidence)
	}
}

func TestCLIAddHypothesisBadSession(t *testing.T) {
	db := testDB(t)
	_, err := runCLI(t, db, "session", "add-hypothesis",
		"--session", "nonexistent",
		"--statement", "something")
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestCLIUpdateHypothesis(t *testing.T) {
	db := testDB(t)
	out, _ := runCLI(t, db, "session", "start",
		"--title", "test", "--service", "svc", "--env", "dev")
	var sess domain.Session
	json.Unmarshal(out, &sess)

	// Add a finding to link
	out, _ = runCLI(t, db, "session", "add-finding",
		"--session", sess.ID, "--summary", "high latency")
	var finding domain.Finding
	json.Unmarshal(out, &finding)

	// Add hypothesis
	out, _ = runCLI(t, db, "session", "add-hypothesis",
		"--session", sess.ID, "--statement", "pool exhaustion")
	var h domain.Hypothesis
	json.Unmarshal(out, &h)

	// Update: change status, add confidence, link finding
	out, err := runCLI(t, db, "session", "update-hypothesis",
		"--id", h.ID,
		"--status", "supported",
		"--confidence", "0.85",
		"--support", finding.ID,
		"--next-checks", "check pool size")
	if err != nil {
		t.Fatalf("update-hypothesis failed: %v", err)
	}
	var updated domain.Hypothesis
	if err := json.Unmarshal(out, &updated); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if updated.Status != domain.HypothesisSupported {
		t.Errorf("expected supported, got %q", updated.Status)
	}
	if updated.Confidence == nil || *updated.Confidence != 0.85 {
		t.Error("confidence not updated")
	}
	if len(updated.SupportingFindingIDs) != 1 || updated.SupportingFindingIDs[0] != finding.ID {
		t.Errorf("supporting findings mismatch: %v", updated.SupportingFindingIDs)
	}
	if len(updated.NextChecks) != 1 {
		t.Errorf("expected 1 next check, got %d", len(updated.NextChecks))
	}
}

func TestCLIUpdateHypothesisContradicting(t *testing.T) {
	db := testDB(t)
	out, _ := runCLI(t, db, "session", "start",
		"--title", "test", "--service", "svc", "--env", "dev")
	var sess domain.Session
	json.Unmarshal(out, &sess)

	out, _ = runCLI(t, db, "session", "add-hypothesis",
		"--session", sess.ID, "--statement", "test hyp")
	var h domain.Hypothesis
	json.Unmarshal(out, &h)

	out, err := runCLI(t, db, "session", "update-hypothesis",
		"--id", h.ID,
		"--status", "contradicted",
		"--contradict", "f1,f2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	json.Unmarshal(out, &h)
	if h.Status != domain.HypothesisContradicted {
		t.Errorf("expected contradicted, got %q", h.Status)
	}
	if len(h.ContradictingFindingIDs) != 2 {
		t.Errorf("expected 2 contradicting findings, got %d", len(h.ContradictingFindingIDs))
	}
}

func TestCLIUpdateHypothesisNotFound(t *testing.T) {
	db := testDB(t)
	_, err := runCLI(t, db, "session", "update-hypothesis",
		"--id", "nonexistent", "--status", "supported")
	if err == nil {
		t.Error("expected error for nonexistent hypothesis")
	}
}

func TestCLIRankHypotheses(t *testing.T) {
	db := testDB(t)
	out, _ := runCLI(t, db, "session", "start",
		"--title", "test", "--service", "svc", "--env", "dev")
	var sess domain.Session
	json.Unmarshal(out, &sess)

	// Add hypotheses with different confidences
	out, _ = runCLI(t, db, "session", "add-hypothesis",
		"--session", sess.ID, "--statement", "low confidence", "--confidence", "0.2")
	var hLow domain.Hypothesis
	json.Unmarshal(out, &hLow)

	out, _ = runCLI(t, db, "session", "add-hypothesis",
		"--session", sess.ID, "--statement", "high confidence", "--confidence", "0.9")
	var hHigh domain.Hypothesis
	json.Unmarshal(out, &hHigh)

	out, _ = runCLI(t, db, "session", "add-hypothesis",
		"--session", sess.ID, "--statement", "supported one", "--confidence", "0.6")
	var hSup domain.Hypothesis
	json.Unmarshal(out, &hSup)

	// Mark one as supported (rank priority 0, above open which is 1)
	runCLI(t, db, "session", "update-hypothesis",
		"--id", hSup.ID, "--status", "supported")

	// Rank
	out, err := runCLI(t, db, "session", "rank-hypotheses", "--session", sess.ID)
	if err != nil {
		t.Fatalf("rank failed: %v", err)
	}
	var ranked []domain.Hypothesis
	if err := json.Unmarshal(out, &ranked); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(ranked) != 3 {
		t.Fatalf("expected 3 hypotheses, got %d", len(ranked))
	}
	// Supported first, then open by descending confidence
	if ranked[0].ID != hSup.ID {
		t.Errorf("rank 0: expected supported, got %q (status=%s)", ranked[0].Statement, ranked[0].Status)
	}
	if ranked[1].ID != hHigh.ID {
		t.Errorf("rank 1: expected high confidence open, got %q", ranked[1].Statement)
	}
	if ranked[2].ID != hLow.ID {
		t.Errorf("rank 2: expected low confidence open, got %q", ranked[2].Statement)
	}
}

func TestCLIRankHypothesesEmpty(t *testing.T) {
	db := testDB(t)
	out, _ := runCLI(t, db, "session", "start",
		"--title", "test", "--service", "svc", "--env", "dev")
	var sess domain.Session
	json.Unmarshal(out, &sess)

	out, err := runCLI(t, db, "session", "rank-hypotheses", "--session", sess.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty array or null is fine
	var ranked []domain.Hypothesis
	json.Unmarshal(out, &ranked)
	if len(ranked) != 0 {
		t.Errorf("expected 0 hypotheses, got %d", len(ranked))
	}
}

func TestCLIHypothesisFullFlow(t *testing.T) {
	db := testDB(t)

	// Start session, add findings, add hypotheses, update, rank
	out, _ := runCLI(t, db, "session", "start",
		"--title", "OOM investigation", "--service", "worker", "--env", "prod")
	var sess domain.Session
	json.Unmarshal(out, &sess)

	// Add findings
	out, _ = runCLI(t, db, "session", "add-finding",
		"--session", sess.ID, "--summary", "RSS growing over time", "--importance", "high")
	var f1 domain.Finding
	json.Unmarshal(out, &f1)

	out, _ = runCLI(t, db, "session", "add-finding",
		"--session", sess.ID, "--summary", "goroutine count stable")
	var f2 domain.Finding
	json.Unmarshal(out, &f2)

	// Add hypotheses
	out, _ = runCLI(t, db, "session", "add-hypothesis",
		"--session", sess.ID, "--statement", "memory leak in handler",
		"--confidence", "0.8", "--next-checks", "heap profile")
	var h1 domain.Hypothesis
	json.Unmarshal(out, &h1)

	out, _ = runCLI(t, db, "session", "add-hypothesis",
		"--session", sess.ID, "--statement", "goroutine leak",
		"--confidence", "0.3")
	var h2 domain.Hypothesis
	json.Unmarshal(out, &h2)

	// Update: support h1 with f1, contradict h2 with f2
	runCLI(t, db, "session", "update-hypothesis",
		"--id", h1.ID, "--status", "supported", "--support", f1.ID)
	runCLI(t, db, "session", "update-hypothesis",
		"--id", h2.ID, "--status", "contradicted", "--contradict", f2.ID)

	// Rank: supported > contradicted
	out, err := runCLI(t, db, "session", "rank-hypotheses", "--session", sess.ID)
	if err != nil {
		t.Fatalf("rank: %v", err)
	}
	var ranked []domain.Hypothesis
	json.Unmarshal(out, &ranked)
	if len(ranked) != 2 {
		t.Fatalf("expected 2, got %d", len(ranked))
	}
	if ranked[0].ID != h1.ID {
		t.Error("expected supported hypothesis first")
	}
	if ranked[1].ID != h2.ID {
		t.Error("expected contradicted hypothesis second")
	}

	// Verify state includes everything
	out, _ = runCLI(t, db, "session", "get-state", "--id", sess.ID)
	var state domain.SessionState
	json.Unmarshal(out, &state)
	if len(state.Findings) != 2 {
		t.Errorf("expected 2 findings, got %d", len(state.Findings))
	}
	if len(state.Hypotheses) != 2 {
		t.Errorf("expected 2 hypotheses, got %d", len(state.Hypotheses))
	}
}

func TestCLIRecommendNextStep(t *testing.T) {
	db := testDB(t)
	out, _ := runCLI(t, db, "session", "start",
		"--title", "test", "--service", "svc", "--env", "dev")
	var sess domain.Session
	json.Unmarshal(out, &sess)

	// Add a finding so "gather findings" recommendation doesn't appear
	runCLI(t, db, "session", "add-finding",
		"--session", sess.ID, "--summary", "baseline observation")

	// Add hypothesis with next checks
	runCLI(t, db, "session", "add-hypothesis",
		"--session", sess.ID,
		"--statement", "pool exhaustion",
		"--next-checks", "check pool metrics,review connection limits")

	out, err := runCLI(t, db, "session", "recommend-next-step", "--session", sess.ID)
	if err != nil {
		t.Fatalf("recommend failed: %v", err)
	}
	var recs []domain.Recommendation
	if err := json.Unmarshal(out, &recs); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(recs) != 2 {
		t.Fatalf("expected 2 recommendations, got %d", len(recs))
	}
	if recs[0].Action != "check pool metrics" {
		t.Errorf("unexpected action: %q", recs[0].Action)
	}
	if recs[0].Reason == "" {
		t.Error("reason must not be empty")
	}
	if recs[0].Goal == "" {
		t.Error("goal must not be empty")
	}
}

func TestCLIRecommendNextStepLimit(t *testing.T) {
	db := testDB(t)
	out, _ := runCLI(t, db, "session", "start",
		"--title", "test", "--service", "svc", "--env", "dev")
	var sess domain.Session
	json.Unmarshal(out, &sess)

	runCLI(t, db, "session", "add-hypothesis",
		"--session", sess.ID, "--statement", "h1",
		"--next-checks", "a,b,c,d,e")

	out, err := runCLI(t, db, "session", "recommend-next-step",
		"--session", sess.ID, "-n", "2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var recs []domain.Recommendation
	json.Unmarshal(out, &recs)
	if len(recs) != 2 {
		t.Errorf("expected 2 recommendations with -n 2, got %d", len(recs))
	}
}

func TestCLIRecommendNoFindings(t *testing.T) {
	db := testDB(t)
	out, _ := runCLI(t, db, "session", "start",
		"--title", "test", "--service", "svc", "--env", "dev")
	var sess domain.Session
	json.Unmarshal(out, &sess)

	out, err := runCLI(t, db, "session", "recommend-next-step", "--session", sess.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var recs []domain.Recommendation
	json.Unmarshal(out, &recs)
	if len(recs) == 0 {
		t.Fatal("expected at least one recommendation")
	}
	// Should suggest gathering findings
	found := false
	for _, r := range recs {
		if strings.Contains(r.Action, "findings") || strings.Contains(r.Action, "logs") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected recommendation to gather findings, got %v", recs)
	}
}

func TestCLIGenerateSummaryHandoff(t *testing.T) {
	db := testDB(t)
	out, _ := runCLI(t, db, "session", "start",
		"--title", "latency spike", "--service", "api-gw", "--env", "prod",
		"--incident", "INC-99")
	var sess domain.Session
	json.Unmarshal(out, &sess)

	// Add finding with evidence
	runCLI(t, db, "session", "add-finding",
		"--session", sess.ID,
		"--kind", "observation",
		"--summary", "p99 at 2.3s",
		"--details", "Started at 14:30 UTC",
		"--importance", "high",
		"--evidence", `[{"type":"log","pointer":"/var/log/api.log","snippet":"timeout"}]`)

	// Add hypothesis with next check
	runCLI(t, db, "session", "add-hypothesis",
		"--session", sess.ID,
		"--statement", "upstream DB slow",
		"--confidence", "0.7",
		"--next-checks", "check DB query latency")

	// Generate handoff summary — outputs raw text, not JSON
	out, err := runCLI(t, db, "session", "generate-summary",
		"--session", sess.ID, "--mode", "handoff")
	if err != nil {
		t.Fatalf("generate-summary failed: %v", err)
	}
	md := string(out)
	if !strings.Contains(md, "# Handoff: latency spike") {
		t.Error("expected handoff header")
	}
	if !strings.Contains(md, "INC-99") {
		t.Error("expected incident hint")
	}
	if !strings.Contains(md, "p99 at 2.3s") {
		t.Error("expected finding")
	}
	if !strings.Contains(md, "Started at 14:30") {
		t.Error("expected finding details")
	}
	if !strings.Contains(md, "/var/log/api.log") {
		t.Error("expected evidence pointer")
	}
	if !strings.Contains(md, "upstream DB slow") {
		t.Error("expected hypothesis")
	}
	if !strings.Contains(md, "70%") {
		t.Error("expected confidence")
	}
	if !strings.Contains(md, "Recommended Next Steps") {
		t.Error("expected next steps in handoff")
	}
	if !strings.Contains(md, "check DB query latency") {
		t.Error("expected next check in recommendations")
	}
}

func TestCLIGenerateSummaryPostmortem(t *testing.T) {
	db := testDB(t)
	out, _ := runCLI(t, db, "session", "start",
		"--title", "outage", "--service", "payments", "--env", "prod")
	var sess domain.Session
	json.Unmarshal(out, &sess)

	runCLI(t, db, "session", "add-finding",
		"--session", sess.ID, "--summary", "circuit breaker tripped")
	runCLI(t, db, "session", "add-hypothesis",
		"--session", sess.ID, "--statement", "downstream timeout")

	// Close session
	runCLI(t, db, "session", "close",
		"--session", sess.ID, "--status", "resolved", "--outcome", "increased timeout")

	out, err := runCLI(t, db, "session", "generate-summary",
		"--session", sess.ID, "--mode", "postmortem-draft")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	md := string(out)
	if !strings.Contains(md, "# Postmortem Draft: outage") {
		t.Error("expected postmortem header")
	}
	if !strings.Contains(md, "resolved") {
		t.Error("expected resolved status")
	}
	if !strings.Contains(md, "increased timeout") {
		t.Error("expected outcome")
	}
	if !strings.Contains(md, "## Timeline") {
		t.Error("expected timeline in postmortem")
	}
	if !strings.Contains(md, "session_started") {
		t.Error("expected session_started event in timeline")
	}
	if strings.Contains(md, "Recommended Next Steps") {
		t.Error("postmortem should not include next steps")
	}
}

func TestCLIGenerateSummaryDefaultMode(t *testing.T) {
	db := testDB(t)
	out, _ := runCLI(t, db, "session", "start",
		"--title", "test", "--service", "svc", "--env", "dev")
	var sess domain.Session
	json.Unmarshal(out, &sess)

	// Default mode (no --mode flag)
	out, err := runCLI(t, db, "session", "generate-summary",
		"--session", sess.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	md := string(out)
	if !strings.Contains(md, "# Handoff:") {
		t.Error("expected default mode to produce handoff summary")
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
