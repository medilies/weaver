# Domain Docs

How the engineering skills should consume this repo's domain documentation.

## Before exploring, read these

- **The `## Ubiquitous language` table in `CLAUDE.md`**: this repo's glossary, and the rules above it. There is no separate `GLOSSARY.md`; don't create one.
- **`docs/adr/`**: read ADRs that touch the area you're about to work in.

## Use the glossary's vocabulary

When your output names a domain concept (in an issue title, a refactor proposal, a hypothesis, a test name), use the term as defined in the table. Respect the "not" notes (e.g. "kill", not "stop").

If the concept you need isn't in the table yet, either you're inventing language the project doesn't use (reconsider) or there's a real gap (note it for `/domain-modeling`, which adds the entry to the table).

## Flag ADR conflicts

If your output contradicts an existing ADR, surface it explicitly rather than silently overriding:

> _Contradicts ADR-0007 (…), but worth reopening because…_
