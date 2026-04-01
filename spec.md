# Spec: Local Troubleshooting Session Manager

## Summary

Build an MVP for a local troubleshooting session manager used during AI-assisted investigations in terminal-centric workflows.

The system must allow an engineer or AI agent to:
- start an investigation session
- record findings and evidence
- create and update hypotheses
- inspect current state and timeline
- rank current hypotheses
- recommend next investigative steps
- generate markdown summaries for handoff or postmortem drafting

The same capabilities must be available through:
- a local CLI
- a local MCP stdio server

The system is local-first and must not require a central service.

---

## Problem Statement

During troubleshooting, important state is often lost across:
- terminal sessions
- AI conversations
- ad hoc shell commands
- partial notes
- interrupted investigations

As a result:
- hypotheses are forgotten or repeated
- supporting and contradicting evidence is not linked clearly
- handoff is weak
- postmortem preparation becomes manual reconstruction

A lightweight local session engine can preserve the state of an investigation without requiring a full centralized troubleshooting platform.

---

## Goals

### Primary goals

1. Provide a structured local representation of an investigation session.
2. Make that representation accessible both to humans and AI tools.
3. Preserve findings, hypotheses, evidence references, and timeline events across restarts.
4. Generate concise grounded summaries for handoff and postmortem drafting.
5. Keep the implementation local-first and simple to run.

### Secondary goals

1. Make the design suitable for future integration into richer troubleshooting workflows.
2. Enable AI-assisted ranking and recommendation later without changing the core data model.
3. Support gradual migration from ad hoc notes toward structured investigation state.

---

## Non-goals

The MVP does not attempt to provide:

- centralized collaboration across multiple users
- a hosted or shared backend
- authentication or access control
- direct log/metrics/traces collection integrations
- autonomous troubleshooting
- automatic root cause determination
- automatic remediation
- mutation of production systems
- workflow orchestration beyond session-state management

---

## Users

### Primary user
A troubleshooting engineer working locally in terminal-based workflows who wants to preserve and organize the state of an investigation.

### Secondary user
An AI agent operating via Codex CLI or Claude Code that needs a typed, structured place to store and retrieve investigation state.

---

## User Stories

### Session lifecycle

- As a troubleshooting engineer, I want to start a local investigation session for a service and environment so that I can persist the context of my work.
- As a troubleshooting engineer, I want to reopen or inspect an active session so that I can resume work after interruption.
- As a troubleshooting engineer, I want to close a session with an outcome so that the final state is explicit.

### Findings and evidence

- As a troubleshooting engineer, I want to add a finding with a short summary, detailed notes, and evidence references so that useful observations are not lost.
- As a troubleshooting engineer, I want evidence references to point to logs, shell commands, SQL output, traces, metrics, or files so that later review is grounded.
- As a troubleshooting engineer, I want to list and filter findings so that I can inspect the most relevant evidence quickly.

### Hypotheses

- As a troubleshooting engineer, I want to create hypotheses during investigation so that possible explanations are explicit.
- As a troubleshooting engineer, I want to mark hypotheses as supported, contradicted, confirmed, or rejected so that my investigation has visible progress.
- As a troubleshooting engineer, I want to link findings to hypotheses so that evidence is traceable.

### Recommendations and summaries

- As a troubleshooting engineer, I want the system to rank current hypotheses so that I can focus on the most promising explanations.
- As a troubleshooting engineer, I want the system to recommend a small number of next steps with reasons so that I can move the investigation forward.
- As a troubleshooting engineer, I want a markdown summary for handoff or postmortem notes so that I can communicate what is known efficiently.

### AI integration

- As an AI agent, I want to perform the same session operations through MCP tools so that I can participate in structured troubleshooting flows without inventing my own state store.
- As an AI agent, I want typed structured inputs and outputs so that tool usage is reliable and reproducible.

---

## Functional Requirements

### FR-1 Session creation
The system must support starting a new session with:
- title
- service
- environment
- incident hint
- optional labels

Output must include:
- session id
- created timestamp
- initial status

### FR-2 Session retrieval
The system must support retrieving current session state by session id.

Returned state must include:
- session metadata
- current status
- findings summary
- hypotheses summary
- recent timeline events

### FR-3 Finding creation
The system must support adding a finding to a session.

A finding must support:
- finding kind
- short summary
- optional detailed notes
- optional importance
- optional tags
- zero or more evidence references

### FR-4 Evidence references
Each finding may include structured evidence references.

An evidence reference must support:
- type
- pointer
- optional snippet
- collected timestamp if provided

Supported evidence types in MVP may include:
- log
- shell
- sql
- file
- url
- trace
- metric
- k8s

### FR-5 Hypothesis creation
The system must support adding a hypothesis to a session.

A hypothesis must support:
- statement
- optional confidence
- optional impact
- optional next checks

### FR-6 Hypothesis update
The system must support updating a hypothesis with:
- status
- confidence
- supporting finding ids
- contradicting finding ids
- additional next checks

Supported statuses:
- open
- supported
- contradicted
- confirmed
- rejected

### FR-7 Hypothesis ranking
The system must support returning hypotheses ranked for the current session.

The response must include at minimum:
- hypothesis id
- statement
- status
- confidence if known
- impact if known

If ranking logic provides reasoning, the response should include it.

### FR-8 Next-step recommendation
The system must support returning a limited number of recommended next steps.

Each recommendation must include:
- action
- why it is useful now
- intended investigative goal

### FR-9 Timeline retrieval
The system must support retrieving a timeline of session events.

The timeline should include:
- timestamp
- event kind
- event payload summary

### FR-10 Summary generation
The system must support generating a markdown summary for a session.

The summary must support at least:
- handoff mode
- postmortem-draft mode

The summary should include:
- session context
- key findings
- current or final hypotheses
- recommended or executed next steps if available
- timeline excerpts when requested

### FR-11 Session closing
The system must support closing a session with an explicit final status.

Supported final statuses may include:
- resolved
- mitigated
- abandoned
- needs-followup

### FR-12 CLI interface
All MVP operations must be available through a local CLI.

### FR-13 MCP interface
All MVP operations must be available through a local MCP stdio server.

### FR-14 Shared core behavior
CLI and MCP must use the same underlying core logic and storage model.

### FR-15 Persistence
All stored sessions, findings, hypotheses, and timeline events must survive process restart.

---

## Data Model Expectations

### Session
A session contains:
- identity
- metadata
- state
- associated findings
- associated hypotheses
- associated timeline

### Finding
A finding is a structured observation captured during the investigation.

Expected attributes:
- id
- session id
- timestamp
- kind
- summary
- details
- importance
- tags
- evidence refs

### Hypothesis
A hypothesis is a candidate explanation for the observed problem.

Expected attributes:
- id
- session id
- statement
- status
- confidence
- impact
- supporting finding ids
- contradicting finding ids
- next checks

### Timeline event
A timeline event records an important moment in the investigation.

Expected kinds include:
- session_started
- finding_added
- hypothesis_added
- hypothesis_updated
- summary_generated
- session_closed
- external_event

---

## Interface Sketch

### MCP tools expected in MVP

- `session_start`
- `session_get_state`
- `session_add_finding`
- `session_add_hypothesis`
- `session_update_hypothesis`
- `session_rank_hypotheses`
- `session_recommend_next_step`
- `session_generate_summary`
- `session_get_timeline`
- `session_close`

### CLI commands expected in MVP

- `session start`
- `session get-state`
- `session add-finding`
- `session add-hypothesis`
- `session update-hypothesis`
- `session rank-hypotheses`
- `session recommend-next-step`
- `session generate-summary`
- `session get-timeline`
- `session close`

Naming may change slightly during implementation, but parity between CLI and MCP is required.

---

## Storage Requirements

The MVP must use local persistent storage.

The storage implementation must:
- require no separately managed server
- survive process restarts
- support querying by session id
- support links between hypotheses and findings
- support ordered timeline retrieval

A single local database or local structured files are acceptable.
A single-file embedded database is preferred.

---

## UX Expectations

### CLI UX
The CLI should be:
- scriptable
- human-usable
- explicit
- unsurprising

Structured outputs should be available where useful.
Human-readable outputs should be available by default for summaries and inspection.

### MCP UX
The MCP interface should:
- expose typed tool inputs
- return structured outputs
- remain easy for terminal agents to use
- avoid hidden side effects

---

## Quality and Behavior Expectations

1. The system should prefer explicit structured fields over opaque blobs.
2. The system should preserve timestamps for all meaningful events.
3. The system should not require an LLM for core behavior.
4. Recommendations and rankings should remain explainable.
5. The system should be safe to use in read-only troubleshooting workflows.

---

## Success Criteria

The MVP is successful if all of the following are true:

1. A developer can start a session locally, add findings and hypotheses, close the process, reopen it, and continue work without losing state.
2. CLI and MCP can both manipulate the same stored sessions.
3. Generated summaries are sufficient for basic handoff between engineers.
4. Findings can be linked to hypotheses in a queryable way.
5. The tool is simple enough to adopt without standing up a backend or provisioning service credentials.

---

## Open Questions for Later Iterations

These are explicitly postponed beyond MVP:

- Should ranking be purely heuristic or optionally LLM-assisted?
- Should evidence snippets be stored inline or separately for large artifacts?
- Should sessions support export/import between machines?
- Should postmortem generation become a richer structured output?
- Should future versions support collaboration or shared stores?
- Should future versions integrate directly with logs, metrics, traces, or tickets?