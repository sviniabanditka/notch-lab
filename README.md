# notch-lab

A native macOS development environment under the notch and in a full window: terminals, a browser with Extensions, an editor, files, notes, an HTTP client, a database manager, SSH hosts, pull requests and coding agents. Agents control all of it through MCP.

Status: specification. No code yet.

## Read first

- `CONTEXT.md` — the vocabulary. Use these words everywhere.
- `docs/spec/00-overview.md` — what the app is, the facts that decided the design, the repository layout.
- `docs/spec/01-model.md` to `15-build-test.md` — one document per part, numbered requirements.
- `docs/adr/` — the decisions and why.
- `docs/milestones.md` — the order of work.

All documents are written in ASD-STE100 Simplified Technical English (`docs/spec/00-writing.md`).

## Platform

macOS 15.4 or later, Apple Silicon and Intel. Go, no cgo, on a fork of mygo. MIT license.
