# 0007. The Emulator is Ghostty's, and the Daemon keeps a Snapshot

Date: 2026-10-12. Status: accepted. Changes ADR 0003 and `03-terminal.md` R-TERM-020 … R-TERM-055.

## Context

ADR 0003 planned a Daemon that stores raw output in an 8 MiB ring buffer and an Emulator of our own or `charmbracelet/x/vt` in the app. mygo 0.4.5 ships `plugins/terminal`: the terminal of Ghostty (libghostty-vt, loaded with purego, put into the bundle by `mygo build`) with painting, selection, input methods, links, bracketed paste, mouse reports, the Kitty keyboard protocol and a Scrollback that reflows. It takes a `Conn` in place of a program, and it has `Feed` and `Snapshot`, made for a server that keeps a session's screen for the windows that attach to it.

A raw ring buffer replays from a cut in the middle of a sequence, replays old device queries (which the app would answer into the shell), and holds 8 MiB per Session.

## Decision

- The app shows a Tab with `terminal.New(Options{Conn: <Session>})` of the plugin. No Emulator package of our own.
- The Daemon feeds every Session's output to a headless plugin terminal, in the same critical section that fans it out to attached clients. `session.attach` answers with that terminal's `Snapshot()` (Screen, Scrollback, modes, cursor), then live bytes. The headless terminal answers device queries only while no client is attached.
- The Daemon reads an Agent's status from the headless terminal's text, so status and Unread work while no app is attached.
- The Daemon sends one event, `session.state` (folder, title, foreground process, shell or not, exit, Agent), in place of `session.title`, `session.cwd_changed`, `session.exited` and `agent.changed`.

## Consequences

- The ring buffer setting `daemon.buffer_bytes` sets the headless terminal's Scrollback in bytes.
- The Daemon links libghostty-vt too: it finds it at `Contents/Resources/libghostty-vt.dylib` from `Contents/Resources/bin/notch-labd`.
- R-TERM-041 … R-TERM-055 are Ghostty's behaviour; the tests are Ghostty's. Search in the Scrollback (R-TERM-110 … R-TERM-113) waits for a search API in the plugin. `[later]`
- A Session does not survive a Daemon restart (as before).
