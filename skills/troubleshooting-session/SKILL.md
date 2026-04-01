---
name: troubleshooting-session
description: Manage structured troubleshooting sessions using the tscli MCP server — track findings, hypotheses, evidence, and generate handoff summaries
---

# Troubleshooting Session Skill

Use the troubleshooting MCP server to manage structured investigation sessions.
This keeps findings, hypotheses, and evidence organized across restarts, handoffs, and tool switches.

## When to use

Start a troubleshooting session when the user is investigating a problem — service outage, latency spike, error rate increase, failing deployment, data inconsistency, or similar operational issue.

Indicators:
- "why is X down / slow / failing"
- "let's investigate", "let's debug", "let's troubleshoot"
- "something is wrong with service Y"
- user is checking logs, metrics, traces, or running diagnostic commands
- user mentions an incident number or page

Do NOT start a session for routine development, code review, feature work, or questions unrelated to an active investigation.

## Investigation workflow

### 1. Start a session

Call `session_start` as soon as you recognize an investigation is beginning.

```
session_start(title, service, environment, incident_hint?)
```

- `title`: concise description of the problem ("high p99 latency on /api/v1/orders")
- `service`: the service or component under investigation
- `environment`: prod, staging, dev, etc.
- `incident_hint`: incident ID if known (e.g. "INC-1234")

Keep the session ID for all subsequent calls.

### 2. Record findings as you discover them

Every time you observe something relevant — a log line, a metric, a command output, a configuration value — record it as a finding. Do this immediately, not in batches.

```
session_add_finding(session_id, kind, summary, details?, importance?, tags?, evidence?)
```

- `kind`: what type of observation — `observation`, `error`, `anomaly`, `configuration`, `change`
- `summary`: one-line description of what was found
- `details`: longer explanation, raw output, or context
- `importance`: `critical`, `high`, `medium`, `low`
- `evidence`: structured references to the source material:
  ```json
  [{"type": "log", "pointer": "/var/log/app.log:423", "snippet": "OOM killed process 1234"}]
  ```
  Evidence types: `log`, `shell`, `sql`, `file`, `url`, `trace`, `metric`, `k8s`

Record findings even when they seem to rule something out — contradicting evidence is valuable.

### 3. Formulate hypotheses

When you have enough findings to form a theory, or when the user suggests a possible cause, record it as a hypothesis.

```
session_add_hypothesis(session_id, statement, impact?, confidence?, next_checks?)
```

- `statement`: a testable claim ("connection pool is exhausted due to leaked connections")
- `confidence`: 0.0 to 1.0 — your current estimate of how likely this is
- `impact`: how severe if true — `critical`, `high`, `medium`, `low`
- `next_checks`: what to investigate next to validate or refute this hypothesis

Always include `next_checks`. A hypothesis without next checks is a dead end.

### 4. Update hypotheses as evidence arrives

As you gather more findings, link them to hypotheses and update status.

```
session_update_hypothesis(id, status?, confidence?, supporting_finding_ids?, contradicting_finding_ids?, next_checks?)
```

Statuses:
- `open` — not yet tested
- `supported` — evidence points toward this being correct
- `contradicted` — evidence points against this
- `confirmed` — validated as the root cause
- `rejected` — definitively ruled out

Link finding IDs to make the evidence chain explicit. Update confidence as new information arrives.

### 5. Use ranking and recommendations to guide focus

When deciding what to do next, ask the system:

```
session_rank_hypotheses(session_id)     — ordered by status priority and confidence
session_recommend_next_step(session_id) — actionable next steps with reasoning
```

Ranking order: supported > open > contradicted > confirmed > rejected.
Within the same status, higher confidence ranks first.

Use recommendations to stay on track rather than pursuing tangential threads.

### 6. Generate summaries for handoff or review

```
session_generate_summary(session_id, mode)
```

- `handoff` — for passing the investigation to another engineer: includes context, findings with evidence, ranked hypotheses, and recommended next steps
- `postmortem-draft` — for after resolution: includes context, findings, hypotheses, outcome, and full timeline

Generate a handoff summary:
- when the user asks to hand off or share status
- when the user is stepping away or ending a shift
- when switching to a different investigation thread

Generate a postmortem draft:
- after the session is closed
- when the user asks for a postmortem or incident review

### 7. Close the session

When the investigation reaches a conclusion:

```
session_close(session_id, status, outcome?)
```

Final statuses:
- `resolved` — root cause found and fixed
- `mitigated` — impact reduced but root cause may remain
- `abandoned` — investigation stopped without resolution
- `needs-followup` — partially resolved, requires further work

Always include an `outcome` describing what was done.

## Behavioral guidelines

**Record early and often.** Don't wait until you have a complete picture. Partial findings are better than lost observations. If you ran a command and the output was relevant, record it immediately.

**Be specific in evidence.** Include file paths, line numbers, command outputs, metric values. The goal is that someone reading the finding later can retrace your steps.

**Keep hypotheses testable.** "Something is wrong" is not a hypothesis. "The connection pool is exhausted because the cleanup goroutine panics on nil pointer" is.

**Update, don't duplicate.** When you learn more about an existing hypothesis, update it with `session_update_hypothesis`. Don't create a new hypothesis that says the same thing with minor changes.

**Check state before acting.** If resuming work or unsure of current state, call `session_get_state` to see where the investigation stands. Call `session_get_timeline` for a chronological view.

**Don't skip the boring findings.** "CPU usage is normal" is a finding. "No recent deploys" is a finding. Negative results narrow the search space and support or contradict hypotheses.

**Use the right evidence type.** This makes findings filterable and traceable:
- `log` — log file entries, journald output
- `shell` — command output (kubectl, curl, etc.)
- `sql` — query results
- `metric` — Prometheus, Datadog, Grafana values
- `trace` — distributed trace IDs or spans
- `k8s` — pod status, events, describe output
- `file` — config files, source code
- `url` — dashboards, runbooks, documentation links
