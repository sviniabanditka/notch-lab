# notch-lab overview

## What it is

notch-lab is a native macOS app for software development. One window holds all the tools a developer opens during a day. Coding agents control the same tools through MCP.

Tools, each a Bench type:

| Type | What a Tab is | What the Panel is |
|---|---|---|
| terminal | one Session | none |
| browser | one Page | bookmarks and history |
| editor | one File | the Explorer |
| files | one Folder view | favorites |
| notes | one Note | the list of Notes |
| http | one Request | Collections and Environments |
| db | one Query | Connections and the schema tree |
| ssh | one SSH session | Hosts |
| pull-requests | one Pull request | the list by Review scope |
| agents | one Chat | the list of Agents |

## Facts that decide the design

| Fact | Consequence |
|---|---|
| mygo 0.4.5 draws native UI on the GPU in pure Go and embeds a system WKWebView in a native window | The whole app is Go. Browser Pages are real WebKit views inside the Layout. |
| mygo has non-activating flyouts, window levels, frames, activation policies and display queries, but no screen-anchored panel, no panel that takes the keyboard only on click, no notch rectangle and no global pointer | The app uses a fork of mygo with four window patches (`14-mygo-fork.md`) and, from M4, the WebKit patches. No upstream pull requests. |
| macOS 15.4 has `WKWebExtension`, the engine Safari runs Extensions on | Browser Benches load Chrome and Firefox Extensions. Minimum macOS is 15.4. |
| Search (Office Commun, MIT) has a complete Extension layer on WKWebExtension: Store install, CRX check, Shims, popups | The app ports that layer to Go. The Shim JavaScript is reused as is. |
| herdr-go (MIT) has Go packages for pull requests, file search, file preview, diff view, settings, history, unread, transcripts | The app copies those packages. The herdr protocol is not used. |
| A terminal in the app's process dies with the app | The Daemon owns Sessions. The app is a client of the Daemon. |

## Platform

- macOS 15.4 or later.
- Apple Silicon and Intel.
- Go 1.27 or later, no cgo. ObjC calls go through purego, as mygo does.
- Dependencies in `go.mod` only. No Homebrew packages at run time, except `gh` for pull requests and `ssh` for SSH sessions.

## Processes

| Process | Role | Starts when |
|---|---|---|
| `notch-lab` (app) | the window, the notch panel, every Bench, the MCP socket | the user opens the app |
| `notch-labd` (Daemon) | every Session; agent detection | the app first needs a Session; launchd keeps it |
| `notch-lab --mcp` (Bridge) | stdio MCP for one Client | an agent client starts it |

The app and the Daemon talk over a Unix socket in `~/Library/Application Support/notch-lab/daemon.sock`. The Bridge and the app talk over `~/Library/Application Support/notch-lab/mcp.sock`. Both sockets have mode 0600 and the app checks the peer's uid.

## Repository layout

```
cmd/notch-lab/        the app: main, bundle, menu, windows
cmd/notch-labd/       the Daemon
internal/model/       Lab, Bench, Tab, Layout, State, the state database
internal/shell/       rail, sidebar, Lab menu, Switcher, History, Unread, Toast
internal/layout/      the Split tree and its view
internal/bench/<type>/ one package per Bench type
internal/daemon/      the Daemon's protocol and its client
internal/emulator/    cells from bytes
internal/extensions/  WKWebExtension port: install, Store, Shims, popups
internal/mcp/         the MCP server and the Bridge
internal/settings/    settings.json, the Settings window, the Keychain
internal/<copied>/    packages copied from herdr-go: pullrequests, filesearch, filepreview, diffview, history, unread, transcript, notify
tools/dev             runs the app against an isolated state folder
tools/verify/         the verify skill: synthetic input, screenshots
docs/spec/            this specification
docs/adr/             decisions
```

Every `internal/` package exposes its API in `api.go` and keeps everything else unexported.

## Performance

| Measure | Limit |
|---|---|
| Time from launch to first frame | 1 s |
| Memory at idle with no browser Tab, main window 1200 x 800 pt on a Retina display | 120 MB (measured 107 MB at M1; the window's GPU buffers are about 45 MB of it) |
| Memory with one browser Tab | 190 MB |
| Terminal frame rate while a program writes at full speed | 60 fps |
| Key press to glyph on screen in a terminal Tab | 16 ms |
| Switcher result for a query over 10 000 items | 50 ms |
| State database write after a change | 100 ms, batched |

## Not in v1

- Plugins and a public API for them.
- Sync between Macs.
- Windows and Linux.
- Language servers (LSP) in the editor.
- More than one main window.
- Automatic updates.
- Browser Extensions that need blocking `webRequest`.
- SFTP and port forwarding in the ssh Bench.
- WebSocket, GraphQL and gRPC in the http Bench.
- Redis, MongoDB and ClickHouse in the db Bench.

## Documents

| Document | Content |
|---|---|
| `00-writing.md` | the writing rules |
| `01-model.md` | Lab, Bench, Tab, Layout, State |
| `02-shell.md` | windows, notch panel, rail, sidebar, Switcher, shortcuts, Settings |
| `03-terminal.md` | the Daemon, Sessions, the Emulator, the terminal Bench |
| `04-browser.md` | Pages, Profiles, Extensions, the Store, the Probe |
| `05-editor.md` | Buffers, the Explorer, the Preview, git |
| `06-files.md` | the files Bench |
| `07-notes.md` | the notes Bench |
| `08-http.md` | the http Bench |
| `09-db.md` | the db Bench |
| `10-ssh.md` | the ssh Bench |
| `11-pull-requests.md` | the pull-requests Bench |
| `12-agents.md` | agent detection, the agents Bench, Tasks |
| `13-mcp.md` | the MCP server, the Bridge, Access levels, tools |
| `14-mygo-fork.md` | what the fork of mygo adds |
| `15-build-test.md` | build, bundle, tests, the verify skill, logs |
| `../milestones.md` | the order of work |
