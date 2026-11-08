# Domain Docs

How the engineering skills should consume this repo's domain documentation when exploring the codebase.

## Before exploring, read these

- **The `## Ubiquitous language` table in `CLAUDE.md`**: this repo's glossary. There is no separate `GLOSSARY.md`; don't create one. New or changed terms go into that table, before any code uses them.
- **`docs/adr/`**: read ADRs that touch the area you're about to work in.

If `docs/adr/` doesn't exist, **proceed silently**. Don't flag its absence; don't suggest creating it upfront. The `/domain-modeling` skill creates it lazily when a decision actually gets resolved.

## File structure

Single-context repo:

```
/
├── CLAUDE.md          ← Ubiquitous language table (the glossary)
├── docs/adr/
│   └── 0001-<decision>.md
├── cmd/weave/
└── internal/session/{domain,app,infra}/
```

## Use the glossary's vocabulary

When your output names a domain concept (in an issue title, a refactor proposal, a hypothesis, a test name), use the term as defined in the Ubiquitous language table. Use only those words: the config format and CLI are released, so when code and user-facing words disagree, the code is renamed. Respect the "not" notes (e.g. "kill", not "stop").

If the concept you need isn't in the table yet, that's a signal: either you're inventing language the project doesn't use (reconsider) or there's a real gap (note it for `/domain-modeling`, which adds the entry to the table).

## Flag ADR conflicts

If your output contradicts an existing ADR, surface it explicitly rather than silently overriding:

> _Contradicts ADR-0007 (…), but worth reopening because…_
