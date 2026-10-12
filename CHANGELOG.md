# Changelog

## 0.1.0 — unreleased

- M0: fork of mygo with the window patches, app bundle, `tools/dev`, verify skill, empty main window.
- M1: Shell with Labs, Benches, Tabs, flexible splits, Switcher, Settings, State in SQLite.
- M2: terminal Bench on the `notch-labd` Daemon with the Ghostty emulator; agent detection.
- M3: native editor, Explorer, Go to File, Preview, git status and diff, files Bench.
- M4: browser Bench on WebKit with Profiles, the Probe and default browser; Chrome Extensions through WKWebExtension with install from the Chrome Web Store ("Add to notch-lab"), CRX3 check, Search's Shim, action popups and Settings › Extensions.
- M5: notch panel, a second Shell on the same State in a flyout under the notch: hover, click, Alt+Space, tray and Window menu; S/M/L, Move to window; a Tab the panel shows draws there only. Extensions now load with the first browser Web view (idle memory back to 114 MB).
- M6: MCP. `notch-lab --mcp` is a stdio Bridge to the app's socket; Access levels per domain, Audit log, Pause and Stop, Settings › Agents with "Connect to Claude Code"; tools for Labs, terminals, the browser (snapshot with refs, trusted clicks and keys, eval, console, network, screenshots) and the editor.
