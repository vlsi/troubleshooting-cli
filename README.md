# troubleshooting-cli

Local-first troubleshooting session manager for AI-assisted investigations.

Preserves findings, hypotheses, evidence, and timeline across terminal sessions, tool switches, and handoffs. Works from the command line and as an MCP server for AI agents.

## Install

```sh
go install github.com/vlsi/troubleshooting-cli/cmd/tscli@latest
```

Single binary, no external dependencies. Data stored in `~/.troubleshooting/sessions.db` (SQLite).

## CLI usage

```sh
# Start an investigation
tscli session start --title "high p99 latency" --service api-gateway --env prod

# Record findings
tscli session add-finding --session <ID> --kind observation \
  --summary "p99 at 2.3s since 14:30 UTC" --importance high

# Formulate hypotheses
tscli session add-hypothesis --session <ID> \
  --statement "connection pool exhausted" --confidence 0.7 \
  --next-checks "check pool metrics,review connection limits"

# Update hypothesis with evidence
tscli session update-hypothesis --id <HYP_ID> \
  --status supported --support <FINDING_ID>

# See what to do next
tscli session rank-hypotheses --session <ID>
tscli session recommend-next-step --session <ID>

# Get session state or timeline
tscli session get-state --id <ID>
tscli session get-timeline --session <ID>

# Generate a summary for handoff or postmortem
tscli session generate-summary --session <ID> --mode handoff
tscli session generate-summary --session <ID> --mode postmortem-draft

# Close the investigation
tscli session close --session <ID> --status resolved --outcome "fixed pool config"
```

All commands output JSON (except `generate-summary` which outputs markdown).

Use `--db /path/to/db` to override the default database location.

## MCP server

The same binary serves as an MCP stdio server for AI agent integration:

```sh
tscli mcp
```

### Configuration

Add to your MCP client config (e.g. `.mcp.json` or `claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "troubleshooting": {
      "command": "tscli",
      "args": ["mcp"]
    }
  }
}
```

To use a custom database path:

```json
{
  "mcpServers": {
    "troubleshooting": {
      "command": "tscli",
      "args": ["--db", "/path/to/sessions.db", "mcp"]
    }
  }
}
```

### MCP tools

| Tool | Description |
|------|-------------|
| `session_start` | Start a new investigation session |
| `session_get_state` | Get full session state |
| `session_add_finding` | Add a finding with evidence refs |
| `session_add_hypothesis` | Add a hypothesis with next checks |
| `session_update_hypothesis` | Update status, confidence, linked findings |
| `session_rank_hypotheses` | Rank hypotheses by status and confidence |
| `session_recommend_next_step` | Get recommended next actions |
| `session_generate_summary` | Generate handoff or postmortem markdown |
| `session_get_timeline` | Get chronological session events |
| `session_close` | Close session with outcome |

### Agent workflow guidance

See [SKILL.md](skills/troubleshooting-session/SKILL.md) for instructions on when and how an AI agent should use the troubleshooting session tools. Copy into your project's `CLAUDE.md` or equivalent prompt file.

## Standalone MCP binary

A separate `tsmcp` entrypoint is also available if you prefer a dedicated binary:

```sh
go install github.com/vlsi/troubleshooting-cli/cmd/tsmcp@latest
tsmcp  # reads stdin, writes stdout
```

This is functionally identical to `tscli mcp`.

## Development

```sh
go test ./...        # run all tests
go build ./cmd/tscli # build CLI
```
