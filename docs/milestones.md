# Milestones

Each milestone ends with a build the user can run every day. The order is fixed. Work inside a milestone can run in parallel.

| # | Name | Delivers | Depends on |
|---|---|---|---|
| M0 | Fork and skeleton (done 2026-10-12) | fork of mygo with the window patches (`14-mygo-fork.md`), app bundle, `tools/dev`, verify skill, empty main window | — |
| M1 | Model and Shell (done 2026-10-12) | Labs, Benches, Tabs, Layout with Splits, State, rail, sidebar, Lab menu, Switcher, History, shortcuts, Settings window, theme | M0 |
| M2 | Terminal (done 2026-10-12) | the Daemon, Sessions, the Emulator, terminal Bench, agent detection, Unread | M1 |
| M3 | Editor and files (done 2026-10-12) | editor Bench with Explorer and Preview, Go to File, files Bench, git status and diff | M1 |
| M4 | Browser (done 2026-10-12) | browser Bench, Profiles, Extensions, Store install, Probe, default browser | M1 |
| M5 | Notch panel | notch panel on the fork's window patches, same State as the main window | M1 |
| M6 | MCP | the server, the Bridge, Access levels, Audit log, tools for lab, terminal, browser, editor, notes | M2, M3, M4 |
| M7 | HTTP | http Bench, Collections, Environments, Keychain secrets, MCP tools | M3 (editor for bodies) |
| M8 | Database | db Bench, Connections, Queries, Results, Tunnels, MCP tools | M7 |
| M9 | SSH | ssh Bench, Hosts, `~/.ssh/config` import, MCP tools | M2 |
| M10 | Notes | notes Bench, search, MCP tools | M3 |
| M11 | Pull requests and agents | pull-requests Bench, agents Bench, Chat, Tasks, MCP tools | M2, M4 |

## Done means

- Every requirement of the milestone's documents without `[later]` passes its acceptance test.
- `go test ./...` passes.
- The verify skill's smoke run passes.
- `docs/` describes what was built. A requirement that changed during the work is updated in its document.
