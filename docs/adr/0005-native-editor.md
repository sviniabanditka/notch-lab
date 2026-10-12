# 0005. The editor is native

Date: 2026-10-12. Status: accepted.

## Context

The editor Bench could be CodeMirror 6 in a web view, which gives many features for free, or a native editor drawn by mygo `ui`, which gives one consistent interface and no web engine in memory for editing.

## Decision

The editor is native: a piece-table Buffer, chroma highlighting, multiple cursors, find and replace. Language servers are `[later]`. The same editor serves the notes Bench, the http Bench's bodies and the db Bench's SQL.

## Consequences

- Features arrive one by one. `05-editor.md` lists v1.
- No web view is needed for editing, so an editor Bench costs no WebKit memory.
