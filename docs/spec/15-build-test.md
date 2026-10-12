# Build, test, release

## Build

R-BUILD-001. The build command is:

```
go run github.com/egoist/mygo/cmd/mygo build -skip-dmg
```

`go.mod` has `replace github.com/egoist/mygo => github.com/sviniabanditka/mygo <version>`, so the command runs the fork. The fork is `14-mygo-fork.md`.

R-BUILD-002. `mygo.json`:

```json
{
  "name": "notch-lab",
  "identifier": "dev.sviniabanditka.notch-lab",
  "version": "0.1.0",
  "main": "./cmd/notch-lab",
  "out": "build",
  "icon": "tools/icon/icon.png",
  "macos": {
    "minimumSystemVersion": "15.4",
    "infoPlist": {
      "CFBundleURLTypes": [{"CFBundleURLName": "Web", "CFBundleURLSchemes": ["http", "https"], "LSHandlerRank": "Default"}],
      "CFBundleDocumentTypes": [{"CFBundleTypeName": "HTML", "LSItemContentTypes": ["public.html"], "CFBundleTypeRole": "Viewer"}],
      "NSUserNotificationsUsageDescription": "notch-lab tells you when an agent needs you.",
      "NSCameraUsageDescription": "A web page asked for the camera.",
      "NSMicrophoneUsageDescription": "A web page asked for the microphone.",
      "NSLocationUsageDescription": "A web page asked for your location."
    }
  }
}
```

`version` equals the `VERSION` file; `tools/dev` refuses to build when they differ. The Daemon (M2) is built by `tools/dev` and `tools/verify/nl` into `resources/darwin-<arch>/bin/notch-labd` before `mygo build`, which copies it to `Contents/Resources/bin/notch-labd` and signs it with the app. The app starts the Daemon from that path.

R-BUILD-003. The build signs the bundle ad hoc. The bundle opens on the build Mac without a warning. Notarization with a Developer ID. `[later]` DMG with `mygo build` (no `-skip-dmg`). `[later]`

R-BUILD-004. Go 1.27 or later. `go.mod` sets `toolchain` and the build uses `GOTOOLCHAIN=auto`. No cgo: `CGO_ENABLED=0` is set in `tools/dev` and in CI.

R-BUILD-005. `go run ./cmd/notch-lab` without the bundle runs the app with no notifications and no URL handling. Everything else works.

## Dev run

R-BUILD-010. `tools/dev` is a shell script. It:

1. sets `NOTCH_LAB_HOME=/tmp/notch-lab-dev` and `CGO_ENABLED=0`;
2. runs the build;
3. starts `build/darwin-*/notch-lab.app/Contents/Resources/bin/notch-labd` under that home when the bundle has one and none runs there;
4. opens the app bundle with `open -n`;
5. tails `$NOTCH_LAB_HOME/app.log`.

`tools/dev down` quits the app and stops the dev Daemon. It never touches `~/Library/Application Support/notch-lab`.

R-BUILD-011. Two dev runs at once serialize with `mkdir $NOTCH_LAB_HOME/dev.lock`. `tools/dev down` removes the lock.

## Logs

R-BUILD-020. The app writes `$NOTCH_LAB_HOME/app.log`. The Daemon writes `$NOTCH_LAB_HOME/daemon.log`. The Bridge writes stderr.

R-BUILD-021. Log levels: error, warn, info, debug. Default: info. Settings › Advanced sets the level for the app and the Daemon.

R-BUILD-022. A log file rotates at 10 MiB. The app keeps 3 old files: `app.log.1`, `app.log.2`, `app.log.3`.

R-BUILD-023. A log line is `<RFC3339 time> <level> <package>: <message>`. No line holds a token, a password, typed terminal input, a request body or a page's text.

R-BUILD-024. `NOTCH_LAB_FRAME_STATS=1` makes the app log frame times every second: count, max, p95.

## Crashes

R-BUILD-030. A panic on the main goroutine writes `$NOTCH_LAB_HOME/crash-<RFC3339 time>.log` with the stack and the last 200 log lines, then the app writes State and restarts the window once. A second panic within 60 s lets the app exit with code 3.

R-BUILD-031. A panic on another goroutine is recovered, written to the log with its stack, and shown as a Toast "Something went wrong; see app.log".

R-BUILD-032. The Daemon on a panic writes `crash-daemon-<time>.log` and exits. launchd restarts it. Sessions are lost; the app shows "Terminal sessions were lost" once.

## Tests

R-BUILD-040. `go test ./...` passes before every merge. Tests run without a window, without the Daemon and without network.

R-BUILD-041. Unit tests per package, at least:

| Package | Tests |
|---|---|
| `internal/model` | migrations from each `user_version` to the current; Layout split, close, focus moves, zoom, ratio limits; State write batching |
| `internal/emulator` | a table of escape sequences and the cells they produce: SGR colors, cursor moves, scroll regions, alt screen, wrap, wide characters, OSC title |
| `internal/daemon` | protocol framing; ring buffer replay; reconnect |
| `internal/extensions` | CRX3 header parse and signature check against a fixture; id from public key; Store URL to id; manifest rewrite for the Shim |
| `internal/bench/http` | `.http` parser round trip; curl import; variable substitution; secret masking |
| `internal/bench/ssh` | `~/.ssh/config` parser: Host patterns, Include, ProxyJump |
| `internal/shell` | Switcher fuzzy scoring and ordering; History back and forward |
| `internal/pullrequests` | as ported from herdr-go |
| `internal/mcp` | as `13-mcp.md` R-MCP-111 |

R-BUILD-042. `ui.Tester` headless tests build the Shell with a fake model: the rail shows one icon per Bench; the sidebar shows the Tabs of the selected Bench; Cmd+B hides the sidebar; the Switcher finds a Tab by a fuzzy query.

R-BUILD-043. Every test writes under `t.TempDir()` and sets `NOTCH_LAB_HOME` to it.

## Verify skill

R-BUILD-050. `tools/verify/nl` is the handle for a live run, ported from herdr-go `hg`. Commands:

| Command | Effect |
|---|---|
| `nl up` | `tools/dev` plus wait for the window; prints the app pid |
| `nl down` | `tools/dev down` |
| `nl shot <name>` | captures the main window only to `$NOTCH_LAB_HOME/shots/<name>.png` |
| `nl text "<text>"` | posts Unicode text to the app's pid |
| `nl key <keycode> [cmd\|shift\|ctrl\|alt...]` | posts a key with modifiers |
| `nl click\|rclick\|dclick\|move X Y` | mouse at window-relative points |
| `nl drag X1 Y1 X2 Y2` | a drag |
| `nl scroll X Y N` | N wheel steps |
| `nl mem [secs]` | samples RSS and CPU of the app and the Daemon |
| `nl mcp <tool> <json>` | one tool call through the Bridge against the dev home |

R-BUILD-051. Input goes to the app's pid only. The user's cursor and front app stay. Text and keys without Cmd bring the app to the front for the send and hand the front back. `NL_FOREGROUND=1` restores global events.

R-BUILD-052. The Mac's Cyrillic layout types Cyrillic through `osascript keystroke`. `nl text` posts Unicode directly. Cmd+letter shortcuts go through menu accelerators, because mygo reports them as key 0.

R-BUILD-053. `nl shot` captures the window only. A context menu is a window of its own and is absent from the shot.

R-BUILD-054. The smoke run `tools/verify/smoke` does: `nl up`; `nl shot smoke`; `nl down`. Exit 0 on success. M1 adds: create a Lab and a terminal Bench. M6 adds: `nl mcp terminal.create`; `nl mcp terminal.write '{"text":"echo ok"}'`; `nl mcp terminal.key Enter`; read the Screen and expect `ok`.

## CI

R-BUILD-060. `.github/workflows/ci.yml` runs on `macos-15` on push and pull request. The fork of mygo is private: the repository secret `MYGO_FORK_TOKEN` (a token that reads `sviniabanditka/mygo`) goes into a git `insteadOf` rule before the first `go` command. Steps:

1. `go vet ./...`
2. `go test ./...`
3. `go run github.com/egoist/mygo/cmd/mygo build -skip-dmg`
4. `tools/verify/smoke` with `NOTCH_LAB_HOME=$RUNNER_TEMP/nl`

The build artifact `notch-lab.app.zip` uploads for 14 days.

R-BUILD-061. CI caches `~/go/pkg/mod` and `~/.cache/go-build` keyed by `go.sum`.

## Benchmarks

R-BUILD-070. Each limit of the performance table in `00-overview.md` has a command:

| Measure | Command |
|---|---|
| launch to first frame | `nl up` prints the time from `open` to the first `frame` log line |
| memory at idle, no browser Tab | `nl mem 10` after `nl up` with one terminal Tab |
| memory with one browser Tab | `nl mem 10` after opening `about:blank` in a browser Tab |
| terminal frame rate | `NOTCH_LAB_FRAME_STATS=1`, run `yes` for 5 s, read p95 from the log |
| key to glyph | `go test -bench Emulator ./internal/emulator` for the parse part; `nl` cannot measure the screen |
| Switcher over 10 000 items | `go test -bench Switcher ./internal/shell` |
| State write | `go test -bench Write ./internal/model` |

R-BUILD-071. A benchmark result above its limit blocks a milestone's "done".

## Versions and releases

R-BUILD-080. The version is semver in the `VERSION` file. The build reads it into `mygo.json`. About and `notch-lab --version` show it with the git commit.

R-BUILD-081. Release checklist:

1. `VERSION` bumped; `CHANGELOG.md` has the version with the date.
2. CI green on `main`.
3. `tools/verify/smoke` passes on a clean `NOTCH_LAB_HOME`.
4. The benchmarks are within limits.
5. A git tag `v<version>`.
6. A GitHub Release with `notch-lab.app.zip` and the changelog section.

R-BUILD-082. Automatic updates. `[later]`

## Licenses

R-BUILD-090. The repository is MIT. `LICENSE` holds the text. The repositories (`notch-lab`, the fork of mygo) are private.

R-BUILD-091. `NOTICE` lists: herdr-go (MIT; packages copied), Search by Office Commun (MIT; the Shim JavaScript and the Extension design), the mygo fork (MIT), and every Go dependency with its license, generated with `go-licenses report ./...` into `NOTICE` at each release.

## Repository CLAUDE.md

R-BUILD-100. `CLAUDE.md` at the repository root says, in short:

- Vocabulary: `CONTEXT.md`. Use its words in code and prose.
- Specification: `docs/spec/`. A change in behaviour updates its requirement.
- Decisions: `docs/adr/`.
- Run the app only through `tools/dev` and `tools/verify/nl`. Every run uses an isolated `NOTCH_LAB_HOME`. Never touch `~/Library/Application Support/notch-lab`.
- `go test ./...` before a commit. Documents in ASD-STE100 (`docs/spec/00-writing.md`).
- mygo is the fork in `go.mod`; upgrade on purpose, never by `go get -u`.
