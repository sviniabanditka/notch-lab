# 0003. Sessions live in a Daemon

Date: 2026-10-12. Status: accepted.

## Context

A terminal in the app's process dies when the app quits or crashes. The options were herdr as a backend (as herdr-go does), tmux in control mode, or a Daemon of our own.

- herdr: ready, detects agents, but owns the workspace, tab, pane and split model. That model conflicts with Lab, Bench, Tab and the Bench-owned Layout. The client is pinned to one herdr version.
- tmux: persistence and scrollback for free, but a second emulator, a Homebrew dependency and size quirks.
- Own Daemon: a PTY owner with a ring buffer and a socket. About 800 lines. Agent detection lives there too.

## Decision

`notch-labd` is a launchd agent. It owns every Session and its raw-byte ring buffer. The app attaches and runs the Emulator. Sessions continue across app restarts, not across logout. `03-terminal.md` has the protocol.

## Consequences

- The app needs an Emulator of its own. `03-terminal.md` names the candidate package and the required sequences.
- Remote shells (ssh Bench) are Sessions too, so they survive app restarts.
- A Daemon version change needs the app and the Daemon to match: the handshake refuses a mismatch and the app restarts the Daemon.
