---
applyTo: "**"
---

## Skill trigger: `troubleshooting-session`

### Automatic session lifecycle

Whenever you begin investigating ANY k8s/PostgreSQL issue, you MUST:

1. **Start a session** — call `mcp__tscli__session_start` immediately when troubleshooting begins. Do not wait for the user to ask.
2. **Record findings** — call `mcp__tscli__session_add_finding` each time you discover a relevant fact (e.g., pod status, error message, metric value). Record findings as you go, not in bulk at the end.
3. **Record hypotheses** — call `mcp__tscli__session_add_hypothesis` as soon as you form a theory about the root cause. Update with `mcp__tscli__session_update_hypothesis` as evidence confirms or refutes it.
4. **Rank hypotheses** — call `mcp__tscli__session_rank_hypotheses` after gathering enough evidence to compare competing theories.
5. **Get next step** — call `mcp__tscli__session_recommend_next_step` when you are unsure what to investigate next.
6. **Close the session** — call `mcp__tscli__session_close` when the root cause is identified and verified, or when the user ends the investigation.

### Key rules

- Do NOT skip session_start — every troubleshooting conversation must have a session.
- Record findings and hypotheses inline as they emerge, not after the fact.
- A handoff summary (`mcp__tscli__session_generate_summary`) should be generated when the user asks for a summary or when closing a long session.
