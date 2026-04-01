# Constitution

## Purpose

This project provides a local-first troubleshooting session engine for AI-assisted investigations.
It stores investigation state in a structured form so that engineers and AI tools can track findings, hypotheses, evidence, timelines, and summaries during troubleshooting.

The project is intended to be used locally on an engineer's machine and integrated with AI tooling through a CLI and an MCP server.

---

## Core Principles

### 1. Local-first execution

The system must work fully on a developer or support engineer machine without requiring a central backend, Kubernetes deployment, or hosted state store.

Rationale:
- local credentials and kube/db access already exist on engineer machines
- deployment and auth complexity must not block adoption
- troubleshooting sessions should remain usable offline or in partially disconnected environments

Implications:
- the primary storage is local persistent storage
- the primary runtime is a local process
- network services are optional future extensions, not MVP requirements

---

### 2. One shared core, multiple frontends

All business logic for sessions, findings, hypotheses, summaries, and timelines must live in a single shared core.

CLI and MCP must be thin adapters over the same domain model and application services.

Rationale:
- behavior must stay consistent between human-driven and agent-driven usage
- duplication between CLI and MCP leads to drift
- testing must focus on a single core

Implications:
- no separate logic implementations for CLI and MCP
- all important operations must be exposed through reusable application services
- tests should primarily target the shared core

---

### 3. Structured state over free-form notes

Investigation state must be stored as explicit structured entities rather than only as unstructured markdown or free-form text.

Required first-class concepts include:
- session
- finding
- hypothesis
- evidence reference
- timeline event
- summary

Rationale:
- structured state enables ranking, linking, filtering, and reproducible summaries
- AI tools benefit from typed data more than large free-form notes
- postmortem generation and handoff need traceable evidence chains

Implications:
- free-form text is allowed as annotation, but not as the only representation of state
- all important events must be queryable as structured data
- relationships between findings and hypotheses must be explicit

---

### 4. Safe by default

The project must not require or encourage destructive actions.

The session engine itself stores and organizes troubleshooting state.
It does not mutate production systems.
Any integrations added later must default to read-only behavior.

Rationale:
- troubleshooting support should reduce operational risk
- local AI-assisted workflows must not silently perform mutating actions
- safety defaults must be explicit from the start

Implications:
- no write actions against production systems in MVP
- recommendations may suggest actions, but should not execute them
- destructive or privileged actions are out of scope unless explicitly added later with separate controls

---

### 5. Explainability over magic

When the system ranks hypotheses, recommends next steps, or generates summaries, it must preserve enough structure to explain why those outputs were produced.

Rationale:
- engineers must be able to trust and review the investigation flow
- hidden scoring or opaque state transitions reduce confidence
- handoff and postmortem usage require traceable reasoning

Implications:
- ranking outputs should include status, confidence, and supporting evidence where possible
- next-step recommendations should include a rationale and intended goal
- summaries should be grounded in stored findings and timeline events

---

### 6. Deterministic core before LLM-assisted behavior

The core functionality of session storage, retrieval, linking, filtering, timeline generation, and summary assembly must remain usable without requiring an LLM.

LLM-assisted features are optional enhancements, not foundational dependencies.

Rationale:
- the project must remain robust, testable, and portable
- the core domain should not disappear if model access changes
- LLM usage should improve workflow, not define it

Implications:
- CRUD-like session behavior must be deterministic
- summary generation should have a deterministic baseline mode
- recommendation and ranking logic may have heuristic or LLM-assisted variants, but must preserve structured outputs

---

### 7. Portable distribution

The project should be easy to install and run across major operating systems with minimal runtime friction.

Rationale:
- adoption depends on low setup cost
- the tool should work well in local shell-centric workflows
- a small portable runtime is preferable to a complex deployment model

Implications:
- prefer a single executable or similarly simple distribution model
- avoid unnecessary infrastructure dependencies
- prefer stdio-friendly integration for MCP

---

### 8. MVP boundaries are strict

The first version must remain intentionally narrow.

The MVP does not include:
- remote multi-user collaboration
- centralized backend services
- authentication and authorization layers
- automatic remediation
- direct destructive integrations
- broad incident-management platform features
- always-on orchestration services

Rationale:
- the project exists to validate structured investigation state first
- early over-expansion would delay useful feedback
- the simplest viable session engine should be delivered first

Implications:
- every new feature must justify why it belongs in MVP
- nice-to-have platform features must be postponed
- scope expansion should happen only after the local session model proves useful

---

## Architectural Direction

The expected architecture is:

- shared domain/application core
- local persistent storage
- thin CLI interface
- thin MCP stdio server interface

This architecture is preferred unless a change clearly improves simplicity without violating the principles above.

---

## Quality Expectations

The project should prioritize:
- correctness of stored investigation state
- clarity of structured data
- portability
- testability
- low operational complexity
- usability in local shell-based troubleshooting workflows

The project should avoid:
- hidden behavior
- strong coupling to one AI vendor
- infrastructure-heavy deployment assumptions
- premature autonomous features