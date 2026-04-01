# AGENTS.md

## Project instructions

This project follows the requirements in:
- `constitution.md`
- `spec.md`

Treat both files as mandatory project guidance.

## Implementation rules

- Build a local-first troubleshooting session manager.
- Keep one shared core and expose it through CLI and MCP stdio.
- Do not add remote services, auth, multi-user sync, or destructive operations.
- Prefer a minimal vertical slice first.
- Start by generating:
    - project layout
    - domain models
    - storage layer
    - CLI scaffolding
    - MCP scaffolding
    - first working commands

## Delivery style

When implementing:
1. restate the scope from `spec.md`
2. propose the smallest workable vertical slice
3. create files and code
4. explain what was created and what remains