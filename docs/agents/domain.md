# Domain docs

FileGot uses a single-context domain-documentation layout.

Before exploring domain behavior, read:

- `CONTEXT.md` at the repository root, when present
- Relevant decisions under `docs/adr/`

These files are created lazily when terminology or durable decisions need to
be recorded. Do not create empty documentation preemptively.

Use terminology defined in `CONTEXT.md` consistently. Surface any proposal
that contradicts an existing ADR instead of silently overriding it.
