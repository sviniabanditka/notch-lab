---
name: verify
description: Run and drive notch-lab under its dev home — launch, type, click, screenshot, measure memory. Use to verify a change live, before a commit, or on "проверь вживую".
---

# Verify notch-lab live

`tools/verify/nl` is the handle; every command uses the dev home `/tmp/notch-lab-dev` and leaves the user's own `~/Library/Application Support/notch-lab` alone. Swift helpers compile into `$NOTCH_LAB_HOME/bin` on first use.

1. `nl up` — builds the bundle, starts the dev Daemon when the bundle has one, opens the app. Done when it prints the app pid.
2. Drive the change at the surface it reaches:
   - Typing: `nl text "…"`, `nl key <keycode> [cmd shift ctrl alt]` (d=2, w=13, k=40, e=14, b=11, t=17, p=35, n=45, arrows 123–126, Enter 36, Escape 53).
   - Mouse: `nl click|rclick|cclick|dclick|move X Y`, `nl scroll X Y N`, `nl drag X1 Y1 X2 Y2` — points relative to the main window's top-left, in screen points.
   - MCP (from M6): `nl mcp <tool> '<json>'`.
3. `nl shot <name>` captures the app window to `/tmp/notch-lab-dev/shots/<name>.png`; read it. `nl mem [secs]` samples RSS and CPU of the app and the Daemon.
4. `nl down` — quits the app and the dev Daemon.

Input goes to the app's pid only (`bg.swift`): the user's cursor and front app stay put, and the window may sit behind other windows or on another Space. `nl text` and `nl key` without Cmd need the key window (mygo drops typed keys otherwise), so they bring the app to the front for the send and hand the front back. The Mac's Cyrillic layout types Cyrillic through `osascript keystroke`; `nl text` posts Unicode directly. Cmd+letter goes through menu accelerators. Context menus are their own window: `nl shot` misses them. `NL_FOREGROUND=1 nl …` restores global events.

`tools/verify/smoke` is the check CI runs: up, shot, down.
