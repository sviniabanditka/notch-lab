# Terminal: the Daemon, Sessions, the Emulator, the terminal Bench

## Parts

| Part | Process | Package | Role |
|---|---|---|---|
| Daemon | `notch-labd` | `cmd/notch-labd`, `internal/daemon` | owns every Session and its ring buffer, detects Agents |
| Daemon client | app | `internal/daemon` | connects to the Daemon, attaches Sessions, receives events |
| Emulator | app | `internal/emulator` | turns a Session's bytes into cells |
| terminal Bench | app | `internal/bench/terminal` | paints cells, takes input, Tabs, selection, links, search |

The Daemon has no Emulator in v1. The Daemon stores bytes. The app renders. Consequence: an MCP Client that reads a Screen reads it from the app (`13-mcp.md`), not from the Daemon. A Session with no attached app has no Screen until an app attaches.

## The Daemon

### Lifetime

R-TERM-001. The Daemon runs as a launchd agent with label `dev.sviniabanditka.notch-lab.daemon`. The app writes the plist to `~/Library/LaunchAgents/dev.sviniabanditka.notch-lab.daemon.plist` the first time the app needs a Session. The plist sets `KeepAlive` to true and `RunAtLoad` to true. The plist's `ProgramArguments` point to the `notch-labd` binary inside the app bundle.

R-TERM-002. When the app moves on disk, the plist's path is stale. The app rewrites the plist and runs `launchctl bootout` then `launchctl bootstrap` when the binary path in the plist differs from the app's own.

R-TERM-003. The Daemon listens on the Unix socket `$NOTCH_LAB_HOME/daemon.sock`. The socket file has mode 0600. The Daemon reads the peer's uid with `getpeereid` and closes a connection from another uid at once, before it reads a byte.

R-TERM-004. When the socket is absent or refuses the connection, the app runs `launchctl kickstart -k gui/<uid>/dev.sviniabanditka.notch-lab.daemon`. When launchd does not know the label, the app bootstraps the plist first. The app retries the connection every 200 ms for 5 s, then shows the Toast "The terminal daemon did not start" with a "Show log" action.

R-TERM-005. When `NOTCH_LAB_HOME` is set (`tools/dev`), the app does not use launchd. The app starts `notch-labd` as a child process with the same environment, so a development run never touches the user's launchd agent.

R-TERM-006. The Daemon writes its log to `$NOTCH_LAB_HOME/daemon.log`, with rotation at 10 MB and 3 files kept.

R-TERM-007. `[later]` The Daemon exits after 10 minutes with zero Sessions and zero connected clients. In v1 the Daemon stays.

R-TERM-008. The Daemon holds a lock file `$NOTCH_LAB_HOME/daemon.lock` with `flock`. A second Daemon that cannot take the lock exits with code 3.

### Handshake

R-TERM-010. The first frame on a connection is `hello` from the client: `{"t":"hello","protocol":1,"client":"notch-lab 0.1.0"}`. The Daemon answers `{"t":"hello","protocol":1,"daemon":"notch-labd 0.1.0","pid":123}`.

R-TERM-011. When the protocol numbers differ, the Daemon answers `{"t":"error","code":"protocol","message":"..."}` and closes. The app then shows the Toast "The terminal daemon is from another version" with a "Restart daemon" action, which runs `launchctl kickstart -k`.

Acceptance: start a Daemon built with protocol 1, connect a client sending protocol 2, the client receives `error protocol` and the socket closes within 100 ms.

### Protocol

R-TERM-020. A frame is a 4-byte big-endian length followed by that many bytes of JSON. A frame is at most 16 MiB. A frame with `"t":"session.data"` carries bytes in the field `b` as base64. `[later]` msgpack frames when base64 cost shows in a profile.

R-TERM-021. Every request from the client carries `"id"`, an integer unique on the connection. The Daemon answers with the same `id` and either `"ok":{...}` or `"error":{"code":"...","message":"..."}`. Events from the Daemon have no `id`.

R-TERM-022. Requests:

| `t` | Params | Answer | Notes |
|---|---|---|---|
| `session.create` | `cwd`, `argv` (list, optional), `env` (map, optional), `cols`, `rows`, `bench`, `tab` | `{"id":"<session>"}` | empty `argv` means the login shell |
| `session.list` | — | `{"sessions":[{id, cwd, argv, cols, rows, bench, tab, pid, exited, code, title, agent}]}` | |
| `session.attach` | `id`, `since` (byte offset, optional) | `{"offset":n}` then `session.data` events | replays the ring buffer from `since`, or all of it |
| `session.detach` | `id` | `{}` | stops `session.data` events for that Session on this connection |
| `session.write` | `id`, `b` (base64) | `{}` | writes to the PTY |
| `session.resize` | `id`, `cols`, `rows` | `{}` | `TIOCSWINSZ`, then `SIGWINCH` |
| `session.signal` | `id`, `signal` (`int`, `term`, `kill`, `hup`) | `{}` | sent to the process group |
| `session.close` | `id` | `{}` | `SIGHUP`, 2 s, `SIGKILL`, frees the buffer |
| `session.cwd` | `id` | `{"cwd":"..."}` | see R-TERM-030 |
| `session.processes` | `id` | `{"processes":[{pid, ppid, name, args}]}` | the foreground process group's tree |
| `session.screen` | `id` | error `unsupported` | `[later]`, see Parts |

R-TERM-023. Events:

| `t` | Fields | When |
|---|---|---|
| `session.data` | `id`, `b`, `offset` | bytes left the PTY, after attach; `offset` is the ring buffer position of the first byte |
| `session.exited` | `id`, `code`, `signal` | the child exited; the Session stays until `session.close` |
| `session.title` | `id`, `text` | OSC 0 or OSC 2 seen in the byte stream |
| `session.cwd_changed` | `id`, `cwd` | OSC 7 seen, or the poll of R-TERM-030 saw a change |
| `agent.changed` | `id`, `agent` (`null` or `{kind, pid, config_dir}`) | see `12-agents.md` |

R-TERM-024. The Daemon scans the byte stream for OSC 0, 2 and 7 only. The Daemon does not interpret other sequences.

R-TERM-025. A connection can attach any number of Sessions. A Session can have any number of attached connections. All receive the same `session.data`.

R-TERM-026. A `session.write` of more than 64 KiB is refused with `error too_large`. The app splits a paste into 64 KiB writes.

R-TERM-027. When the PTY blocks on write for more than 5 s, the Daemon answers `error stalled` to that `session.write` and keeps the Session.

### Sessions

R-TERM-030. The Daemon finds a Session's current folder from the foreground process group leader: `tcgetpgrp` on the PTY, then `proc_pidinfo(pid, PROC_PIDVNODEPATHINFO)`. The Daemon polls this every 1 s per Session and sends `session.cwd_changed` on change. OSC 7 updates it at once.

R-TERM-031. The Daemon opens the PTY with `github.com/creack/pty`. The child gets a new session (`setsid`) and the PTY as its controlling terminal.

R-TERM-032. The login shell is the user's shell from `dscl . -read /Users/<user> UserShell`, else `$SHELL`, else `/bin/zsh`. The Daemon runs it as `<shell> -l` with `argv[0]` set to `-<basename>`.

R-TERM-033. The child's environment is the Daemon's environment plus: `TERM=xterm-256color`, `COLORTERM=truecolor`, `NOTCH_LAB=1`, `NOTCH_LAB_SESSION=<id>`, `TERM_PROGRAM=notch-lab`, `TERM_PROGRAM_VERSION=<version>`, plus the `env` map of the request. `LANG` and `LC_*` come from the Daemon's environment; when `LANG` is unset the Daemon sets `LANG=en_US.UTF-8`.

R-TERM-034. The Daemon's own environment comes from launchd, which has no shell variables. Before the first Session the Daemon runs `<login shell> -l -c env` once, 5 s limit, and takes `PATH` and `HOME` from it. A shell that does not answer in time leaves `PATH` at `/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:/opt/homebrew/bin`.

R-TERM-035. The ring buffer of a Session holds the last 8 MiB of PTY output. The size is a Daemon setting `buffer_bytes` between 1 MiB and 64 MiB, written by the app to `$NOTCH_LAB_HOME/daemon.json`. The buffer keeps a total byte count so `offset` grows without limit.

R-TERM-036. On `session.attach` the Daemon sends the buffer from `since` (or from its start) in `session.data` frames of at most 1 MiB, then live bytes. The client rebuilds Screen and Scrollback by feeding all of it to the Emulator in order. A `since` older than the buffer's start is answered from the buffer's start, and the answer's `offset` says so.

Acceptance: run `seq 1 100000` in a Session, quit the app, open the app, the Tab shows the same last Screen and Scrollback up to the Scrollback limit within 1 s.

R-TERM-037. A Session whose child exited keeps its buffer and its `cwd`. `session.list` reports `exited:true` and `code`.

R-TERM-038. The Daemon closes every Session and exits on `SIGTERM`. Children get `SIGHUP`. `[later]` Session survival across a Daemon restart.

R-TERM-039. The Daemon limits Sessions to 256. `session.create` beyond that answers `error limit`.

## The Emulator

R-TERM-040. The app uses `github.com/charmbracelet/x/vt` when it meets every requirement R-TERM-041 to R-TERM-052. Else the app has its own package `internal/emulator` that meets them. A test file per requirement decides; the tests run against the chosen implementation.

R-TERM-041. Colors: 16 ANSI, 256-color (`38;5;n`), truecolor (`38;2;r;g;b`), default foreground and background, inverse.

R-TERM-042. Attributes: bold, dim, italic, underline (single, double, curly), strikethrough, blink flag, invisible, inverse.

R-TERM-043. Alternate screen (`?1049`, `?47`, `?1047`) with its own cursor and no Scrollback.

R-TERM-044. Mouse modes 1000, 1002, 1003 and SGR encoding 1006. The Emulator reports which mode is on, so the Bench knows when to forward the mouse.

R-TERM-045. Bracketed paste (`?2004`): the Emulator reports the mode; the Bench wraps a paste in `ESC[200~` and `ESC[201~`.

R-TERM-046. OSC 8 hyperlinks: a cell carries its link id and the link's URL.

R-TERM-047. OSC 52 clipboard write: the Emulator emits an event with the bytes; the Bench writes the clipboard only when the setting `terminal.osc52` is on. Default off. OSC 52 read is never answered.

R-TERM-048. Cursor shapes with DECSCUSR 0 to 6, cursor visibility (`?25`), cursor position save and restore.

R-TERM-049. Wide characters take 2 cells. Combining marks join the cell before them. Zero-width joiner sequences and emoji presentation selectors stay in one cell of width 2. Width comes from `golang.org/x/text/width` plus the East Asian Wide table; the ambiguous width is 1.

R-TERM-050. Scroll regions (DECSTBM), insert and delete lines and characters, erase in display and in line, tabs and tab stops, origin mode, auto-wrap and the pending-wrap state at the last column.

R-TERM-051. Resize reflows soft-wrapped lines. A line the program wrapped by writing past the last column is one logical line. A resize re-wraps logical lines to the new width. Hard line ends stay. The cursor stays on the same logical position. The alternate screen does not reflow; it is cut or padded.

R-TERM-052. Scrollback holds the last N logical lines, N from the Bench setting `scrollback` (default 10 000, between 1 000 and 100 000). A Scrollback line keeps its cells and its OSC 8 links.

R-TERM-053. The Emulator exposes: a dirty set of rows since the last read, the cursor, the modes, the title, the Screen as rows of cells, Scrollback by line index, a `Write([]byte)` that takes any split point in a sequence.

R-TERM-054. `Write` of 1 MiB of mixed text and SGR takes under 50 ms on an M1.

R-TERM-055. The Emulator answers device queries the Bench sends back to the PTY: `CSI 6 n` (cursor position), `CSI c` (`?62;22c`), `CSI 0 c`, DA2 `>0;0;0c`, `CSI 5 n` (`0n`), XTVERSION, DECRQM for the modes above. The Bench writes the answers to the Session within the same frame.

## The terminal Bench

### Tabs

R-TERM-060. A terminal Tab shows one Session. The Tab's type data is `{"session":"<id>","font_size":n}`.

R-TERM-061. Cmd+T creates a Session with the login shell in the Bench's effective root folder, and a Tab for it, in the focused Leaf. When the effective root folder does not exist, the shell starts in the home folder and the Tab shows the Toast "Folder not found: <path>".

R-TERM-062. At app start the Bench re-attaches each Tab's Session by id. A Session the Daemon no longer knows gets a new login shell in the Tab's last known folder, and the Tab shows the line "Session was lost; started a new shell" in the Bench's message style above the Screen for 5 s.

R-TERM-063. A Session whose child exited shows the Screen as it was, then one line "Process exited with code N. Press Enter to restart." Enter creates a new Session with the same `cwd` and `argv` and keeps the Tab. Any other key does nothing.

R-TERM-064. The Tab's title is, in order: the custom title; the OSC title when the setting `terminal.osc_title` is on (default on); the foreground process name when the process is not the login shell; the last path element of the folder.

R-TERM-065. The sidebar shows next to the title: the Agent status icon when an Agent runs there (`12-agents.md`), else nothing.

R-TERM-066. The Tab's context menu has: Restart (close the Session, new Session with the same `cwd` and `argv`), Duplicate (new Tab, new Session, same `cwd`), Copy folder path, Rename, Close.

R-TERM-067. Closing a Tab closes its Session in the Daemon. When the Session has a foreground process other than the shell, the app asks "Close <process>?" with Close and Cancel. The setting `terminal.confirm_close` turns the question off.

R-TERM-068. "Open in terminal" from another Bench (editor, files, pull-requests) creates a Tab in the first terminal Bench of the same Lab, or creates a terminal Bench named "terminal" when the Lab has none, with the Session's `cwd` set to the given folder, and goes to it.

### Painting

R-TERM-070. The Bench paints cells as herdr-go's `paneview` does: one draw call per run of cells with equal style, the glyph cache of mygo, the background first, then text, then the cursor, then the selection as an overlay.

R-TERM-071. The font is the setting `terminal.font` (family, size). Default: the first installed of `JetBrains Mono`, `SF Mono`, `Menlo`, size 13. Ligatures are off by default; the setting `terminal.ligatures` turns them on. Cell width is the advance of `M`; cell height is ascent plus descent plus the line gap, rounded up.

R-TERM-072. Cmd+Plus and Cmd+Minus change the font size of the Bench by 1 pt between 8 and 32. Cmd+0 restores the setting's size. The size is stored in the Bench settings, not per Tab.

R-TERM-073. The Leaf's cell grid is `floor(width / cellWidth)` by `floor(height / cellHeight)`. The Bench resizes the Session when the grid changes, at most once per 50 ms during a drag, and once at the end of the drag.

R-TERM-074. Attributes paint as: bold with the bold face, dim at 60 % foreground alpha, italic with the italic face, underline 1 pt below the baseline, double underline 2 lines, curly underline a sine of 1 pt amplitude, strikethrough through the x-height middle, inverse swaps foreground and background, invisible paints background only.

R-TERM-075. The cursor paints as block, underline or bar per DECSCUSR, in the theme's cursor color. A block cursor paints its cell's text in the background color. When the Leaf has no Focus the cursor paints as a hollow block. The setting `terminal.cursor_blink` (default off) blinks the cursor with a 500 ms period; typing restarts the period in the visible phase.

R-TERM-076. The Bench redraws only dirty rows. Output arriving within one frame is coalesced: the Bench feeds the Emulator as bytes arrive, and paints at most once per display refresh (60 fps, or the display's rate).

R-TERM-077. The theme gives the 16 ANSI colors, the default foreground and background, the cursor color, the selection color and the search highlight color. The Bench uses the Lab's color nowhere in the grid.

Acceptance: `cat` a 10 MB text file; the Bench holds 60 fps and the CPU of the app stays under 100 % of one core; `yes` for 10 s then Ctrl+C stops the output within 100 ms.

R-TERM-078. The Scrollback shows when the user scrolls up. A scroll position is a line offset from the bottom. New output while scrolled keeps the view in place; the Bench shows a "↓ N new lines" pill at the bottom right; click or Cmd+End returns to the bottom. Typing a key returns to the bottom.

R-TERM-079. The mouse wheel scrolls 3 lines per notch. A trackpad scrolls by its precise delta in pixels divided by the cell height. Shift+wheel scrolls 1 page. When the program reports mouse (R-TERM-044) and the Screen is the alternate screen, the wheel goes to the program as button 4 and 5 events.

### Keyboard

R-TERM-080. Keys go to the Session as xterm sequences: arrows (`CSI A`..`D`, with modifiers `CSI 1;m A`), Home, End, Page Up, Page Down, Insert, Delete, F1 to F12, Tab, Shift+Tab (`CSI Z`), Enter (`CR`), Backspace (`DEL`), Escape. The application cursor mode (`?1`) switches arrows to `SS3`.

R-TERM-081. Ctrl+letter sends the control byte. Ctrl+Space sends NUL. Ctrl+[ sends ESC.

R-TERM-082. Alt+key sends ESC then the key when the setting `terminal.alt_is_meta` is on (default on). Off, Alt+key types the macOS character.

R-TERM-083. Cmd+key never reaches the Session. Cmd shortcuts go to the app menu. Cmd+Shift+K clears the Scrollback and the Screen (sends nothing to the Session).

R-TERM-084. IME composition shows inline at the cursor with an underline, in the theme's composition style. The committed text goes to the Session as UTF-8. A composition in progress blocks other key handling until it ends.

R-TERM-085. Cmd+V pastes the clipboard text. In bracketed paste mode the text goes between `ESC[200~` and `ESC[201~`. Line ends in the text become `CR`. A paste of more than 1 000 lines, or a paste that contains a newline while the Screen is not in the alternate screen and the foreground process is the shell, asks "Paste N lines?" with Paste and Cancel. The setting `terminal.confirm_paste` turns the question off.

R-TERM-086. When the mouse mode is on, clicks, drags and wheel go to the Session as SGR mouse reports, except Shift+click and Shift+drag, which the Bench keeps for selection.

### Selection

R-TERM-090. A drag selects cells from the press to the pointer. A double-click selects the word around the point; a word is a run of characters not in the setting `terminal.word_separators` (default `` ,│`|:"'()[]{}<>​ ``) plus whitespace. A triple-click selects the logical line. Shift+click extends the selection.

R-TERM-091. The selection stays while output arrives. The selection moves with the Scrollback as lines scroll. The selection clears on a click without drag, on Escape when no search bar shows, and when the Tab leaves the Leaf.

R-TERM-092. Cmd+C copies the selection as text: cells joined per row, trailing spaces of each row removed, soft-wrapped rows joined without a line end, hard rows joined with `\n`. With no selection, Cmd+C sends ETX (Ctrl+C) to the Session. The setting `terminal.copy_on_select` (default off) copies at the end of a drag.

R-TERM-093. Alt+drag selects a rectangle of cells. Copy joins each row with `\n`.

R-TERM-094. The context menu has: Copy, Paste, Select All, Clear Scrollback, Open Link (when on a link), Copy Link (when on a link), Search Scrollback.

### Links

R-TERM-100. The Bench finds links in the rows on screen on every paint, cached per row by the row's content hash:

| Kind | Rule |
|---|---|
| OSC 8 | the cell's link, as the program gave it |
| URL | `https?://`, `ftp://`, `file://`, `mailto:` up to whitespace, a closing bracket not opened in the match, or a trailing `.`, `,`, `;`, `:`, `)`, `'`, `"` |
| path | a run of `[A-Za-z0-9._~/@+-]` with at least one `/` or a known file extension, optionally followed by `:line`, `:line:col`, `(line,col)` or `, line N` |

R-TERM-101. Cmd+hover underlines the link and shows its target in the status bar. Cmd+click opens the link.

R-TERM-102. A URL opens in a browser Tab of the first browser Bench of the same Lab. When the Lab has no browser Bench, the app creates one named "browser". `mailto:` goes to `open`. The setting `terminal.links_external` (default off) sends every URL to the system browser instead.

R-TERM-103. A path resolves, in order: as given when absolute; against the Session's current folder (R-TERM-030); against the Bench's effective root folder; with `~` expanded. The first that exists wins. A path that resolves nowhere is not a link.

R-TERM-104. A path to a file opens in an editor Tab of the first editor Bench of the same Lab (created when missing), at the line and column when given. A path to a folder re-roots that editor Bench's Explorer to the folder, or, when Shift is held, opens a files Tab for it.

R-TERM-105. Link detection on a row takes under 1 ms for a 300-cell row. Rows are scanned only when on screen.

### Search

R-TERM-110. Cmd+F shows the search bar at the top of the focused Leaf with a text field, the count "n of m", previous and next buttons, and a close button. Escape closes it and clears highlights.

R-TERM-111. The search is case-insensitive unless the query has an upper-case letter. The search covers the Screen and the Scrollback as logical lines. The Bench runs the search 100 ms after the last key and highlights every match on screen; the current match has the stronger highlight.

R-TERM-112. Enter goes to the next match toward the top; Shift+Enter toward the bottom. The Bench scrolls so the current match is in the middle third of the Leaf. The search wraps around and the count shows "wrapped" for 1 s.

R-TERM-113. The search over 100 000 lines answers within 200 ms.

### Bell and status

R-TERM-120. BEL flashes the Leaf's background to the theme's bell color for 100 ms. The setting `terminal.bell_sound` (default off) plays the system alert. A BEL in a Tab off screen marks the Tab Unread when the setting `terminal.bell_unread` is on (default off).

R-TERM-121. The status bar of a focused terminal Leaf shows: the Session's current folder shortened with `~`, the foreground process name, the Agent status when an Agent runs, the grid size during a resize for 1 s.

### Settings

| Key | Type | Default | Range |
|---|---|---|---|
| `terminal.font` | `{family, size}` | first installed of JetBrains Mono, SF Mono, Menlo; 13 | size 8 to 32 |
| `terminal.ligatures` | bool | false | |
| `terminal.scrollback` | int | 10 000 | 1 000 to 100 000 |
| `terminal.cursor_blink` | bool | false | |
| `terminal.cursor_shape` | `block`, `bar`, `underline` | `block` | the default when the program sets none |
| `terminal.alt_is_meta` | bool | true | |
| `terminal.osc52` | bool | false | |
| `terminal.osc_title` | bool | true | |
| `terminal.copy_on_select` | bool | false | |
| `terminal.confirm_close` | bool | true | |
| `terminal.confirm_paste` | bool | true | |
| `terminal.bell_sound` | bool | false | |
| `terminal.bell_unread` | bool | false | |
| `terminal.links_external` | bool | false | |
| `terminal.word_separators` | string | see R-TERM-090 | |
| `terminal.shell` | string | empty = the login shell | an absolute path |
| `daemon.buffer_bytes` | int | 8 MiB | 1 MiB to 64 MiB |

Bench settings override app settings for `font`, `scrollback` and `shell`.

## Security

R-TERM-130. The Daemon accepts connections from the same uid only (R-TERM-003). The Daemon never listens on TCP.

R-TERM-131. OSC 52 writes the clipboard only when the setting is on. OSC 52 reads are never answered.

R-TERM-132. Escape sequences in a Tab's title are stripped before the title shows anywhere.

R-TERM-133. A paste never runs before the user sees it in the Session: the Bench strips `ESC` bytes from pasted text when the Session is not in bracketed paste mode.

R-TERM-134. The Daemon's `env` parameter cannot set `DYLD_*` or `LD_*` variables; the Daemon drops them.

## Performance

| Measure | Limit |
|---|---|
| key press to glyph on screen | 16 ms |
| output at 64 KB/s | 60 fps, no dropped frames |
| `yes` for 10 s | input stays responsive; Ctrl+C stops within 100 ms |
| attach with an 8 MiB buffer | Screen shown within 1 s |
| resize drag | grid updates at most every 50 ms, no visible tearing |
| 50 Sessions open across Benches | app memory under 200 MB without browser Tabs |

## Tests

- `internal/daemon`: protocol framing, handshake, uid check with a fake peer, ring buffer wrap and offset, attach replay from `since`.
- `internal/emulator`: one test file per R-TERM-041 to R-TERM-055, with the vttest cases that apply, run against the chosen implementation.
- `internal/bench/terminal`: link detection table, selection to text, paste rules, title order, search wrap, with `ui.Tester`.
- Verify skill: `hg` runs `seq`, `vim`, `htop`, a Claude Code prompt, screenshots compared by hand.
