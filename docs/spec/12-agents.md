> ADR 0007: the Daemon reads the Agent's status from its headless terminal and sends it in `session.state`. Claude Code names its process after its version (`2.1.295`), so detection also reads `argv[0]`.

# Agents: detection, status, the Transcript, the agents Bench, Tasks

## Parts

| Part | Where | Role |
|---|---|---|
| detection | Daemon | finds an Agent in a Session's process tree, reports `agent.changed` |
| status | app, `internal/bench/terminal` | derives Agent status from the Screen and the Transcript |
| Transcript follower | app, `internal/transcript` (copied from herdr-go) | reads the Agent's JSONL and builds the Conversation |
| agents Bench | app, `internal/bench/agents` | the list of Agents, the Chat, Tasks |
| Unread | app, `internal/unread` (copied from herdr-go) | marks Tabs whose Agent needs the user |

## Detection

R-AGENT-001. The Daemon polls each Session's process tree every 1 s: the foreground process group leader and its descendants, by `proc_listchildpids` and `proc_pidpath`.

R-AGENT-002. The Daemon matches the executable's base name against this table. The first match in the tree, nearest the Session's shell, wins.

| Base name | Kind | Config folder |
|---|---|---|
| `claude` | `claude` (Claude Code) | `$CLAUDE_CONFIG_DIR` of the process, else `~/.claude` |
| `codex` | `codex` (Codex) | `$CODEX_HOME` of the process, else `~/.codex` |
| `[later]` `gemini`, `cursor-agent`, `aider` | | |

R-AGENT-003. The Daemon reads the process's environment with `KERN_PROCARGS2` to find the config folder. When the read fails the Daemon reports the default folder.

R-AGENT-004. The Daemon sends `agent.changed{id, agent:{kind, pid, config_dir}}` when an Agent appears or its pid changes, and `agent.changed{id, agent:null}` when the Agent is gone. One event per change, not per poll.

R-AGENT-005. A Session with an Agent inside a nested shell or `tmux` is detected the same way: the match walks the whole tree.

Acceptance: run `claude` in a Session; within 2 s the app receives `agent.changed` with kind `claude`; press Ctrl+C twice; within 2 s the app receives `agent:null`.

## Agent status

R-AGENT-010. An Agent has one status: `idle`, `working`, `blocked`, `done`. The app derives the status; the Daemon reports presence only.

R-AGENT-011. The app derives the status from the Screen after each paint, with these rules in order; the first that matches wins:

| Rule | Status |
|---|---|
| the last 12 rows contain "Do you want to proceed?", "Do you want to", "Tab to amend", "Esc to cancel" together with numbered options, or "(y/n)" at a line end | `blocked` |
| a row in the last 12 contains a spinner glyph (`⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏`, `·✢✳✶✻✽`) or the words "Thinking", "Working", "Running", followed by an ellipsis or a timer | `working` |
| the composer prompt (`>` or `❯` at the start of the last non-empty row) shows and the previous status was `working` | `done` |
| the composer prompt shows | `idle` |
| none | the previous status |

R-AGENT-012. When the Transcript is readable, the Transcript overrides the Screen for `working` and `done`: a Turn that is open is `working`; a Turn that ended within the last 2 s is `done`. `blocked` comes from the Screen only.

R-AGENT-013. When the Screen matches no rule for 30 s and the Transcript is not readable, the app falls back to: `working` when the Agent's CPU time grew in the last 2 s, else `idle`.

R-AGENT-014. `done` lasts until the user shows the Tab, sends input to the Session, or 10 minutes pass; then the status becomes `idle`.

R-AGENT-015. The status shows as an icon next to the Tab in the sidebar, on the Bench in the rail when the Bench is not selected, on the Lab in the Lab menu when the Lab is not selected, and in the agents Bench's list. Order of importance for one icon: `blocked`, `working`, `done`. `idle` shows nothing.

R-AGENT-016. A status change writes one line to the app log with the Session id, the old status, the new status and the rule that matched.

## The Transcript

R-AGENT-020. The app copies herdr-go's `internal/transcript`: `locate.go`, `follow.go`, `reader.go`, `record.go`, `reducer.go`, `tool.go`, `user.go` and their tests. The package knows the Agent's format; nothing else parses a Transcript.

R-AGENT-021. For Claude Code the app finds the session id in `<config>/sessions/<pid>.json`, where `<pid>` is the Agent's pid. The app checks the file's `start_time` against the process start time from `proc_pidinfo`, so a reused pid does not match an old file. The Transcript is `<config>/projects/<cwd slug>/<session>.jsonl`, with the slug as in `locate.go` (every byte not an ASCII letter or digit becomes `-`), else the first `*/<session>.jsonl` under `projects`.

R-AGENT-022. For Codex the app finds the rollout file the Agent holds open, by `proc_pidinfo(PROC_PIDLISTFDS)` and `PROC_PIDFDVNODEPATHINFO`, under `<config>/sessions`. The app never guesses by folder or newest file.

R-AGENT-023. The follower polls the Transcript file every 200 ms by size and reads new bytes from the last offset. It reads the last 2 MiB first and the earlier part when the user scrolls to the top of the Chat. It polls the session file every 1 s while the Agent is `idle` or `done`, so `/clear` and `/resume` are seen.

R-AGENT-024. The follower stops when the Agent is gone, the Tab is closed, or the Chat is not on screen for 5 minutes. It starts again when the Chat shows.

R-AGENT-025. A Transcript the app cannot find shows in the Chat as "No conversation to show" with the text "Start Claude Code in this terminal, or run `claude --resume` to continue a session". The Chat retries every 2 s.

## The agents Bench

### Panel

R-AGENT-030. The Panel lists every Agent the Daemon reports, across every Lab. Each row shows: the status icon, the Agent kind icon, the Tab's title, the path `Lab › Bench › Tab`, the Unread dot, the time since the status last changed. Rows order by status (`blocked`, `working`, `done`, `idle`) then by the time of the last change, newest first.

R-AGENT-031. Click on a row opens that Agent's Chat Tab in the focused Leaf, creating the Tab when the Bench has none for that Session. Double-click goes to the Agent's terminal Tab instead. The context menu has: Open chat, Go to terminal, New task, Copy session id.

R-AGENT-032. An Agent that is gone stays in the Panel for 60 s with a dimmed row, then leaves. Its Chat Tab stays and shows the last Conversation read-only.

### Chat

R-AGENT-040. A Chat Tab shows one Agent's Conversation. The type data is `{"session":"<daemon session id>","agent_session":"<transcript id>"}`.

R-AGENT-041. The Chat renders the Items of the active branch of the Conversation, newest at the bottom:

| Item | Rendering |
|---|---|
| Prompt | the user's text in a card at the right, with the time |
| Answer | markdown via goldmark into native blocks: paragraphs, headings, lists, code blocks with chroma highlight, tables, links (Cmd+click opens), images as Preview cards |
| Thinking | folded by default to one line "Thinking (n s)"; click unfolds |
| tool call | a card with the tool name, the input folded to 3 lines, the result folded to 3 lines, the duration; a file edit shows a diff via `internal/diffview` |
| Slash command | the command and its output in a code block |
| Notice | one dim line |
| Error | a red card |
| interrupt | one dim line "Interrupted" |
| compaction | one dim line "Context compacted" |
| turn end | no rendering; folds the Turn |

R-AGENT-042. A finished Turn folds to its final Answer, its Errors and its interrupts. Click on the Turn's header unfolds all its Items. The setting `agents.fold_turns` (default on) turns folding off.

R-AGENT-043. The Chat keeps the scroll at the bottom while the user has not scrolled up. New Items while scrolled up show a "↓ new" pill.

R-AGENT-044. The composer at the bottom is a multi-line text field. Enter sends; Shift+Enter inserts a newline. Sending writes the text to the Session, waits 300 ms, then writes `CR`. The composer refuses to send while the status is `blocked` or an approval card shows; it then shows "Answer the prompt in the terminal first".

R-AGENT-045. A sent prompt shows at once as a pending Prompt. When the Transcript does not record the Prompt within 10 s the pending card turns into an Error card "The prompt was not delivered; check the terminal" and the text returns to the composer.

R-AGENT-046. The composer keeps its draft per Tab in State.

R-AGENT-047. Cmd+J on a terminal Tab with an Agent opens that Agent's Chat in a Split right of the terminal Leaf, in the first agents Bench of the same Lab, created when missing. Cmd+J again in either Leaf closes the Chat Leaf.

R-AGENT-048. The Chat renders 1 000 Items within 100 ms after the Transcript is read. Items are measured lazily; only on-screen Items are laid out.

### Approvals

R-AGENT-050. When the status is `blocked`, the Chat shows an approval card above the composer with the prompt text and the options parsed from the Screen: lines in the last 12 rows that start with a digit, `.` and a space, or with `❯`.

R-AGENT-051. Click on an option re-reads the Screen; when the same option is still there, the Chat writes the option's digit to the Session, else the card updates. Escape in the card writes `ESC`.

R-AGENT-052. A Screen the parser cannot read shows the card with the text "Answer in the terminal" and a "Go to terminal" button.

### Unread

R-AGENT-060. When an Agent becomes `done` or `blocked` while its terminal Tab is not in any Leaf on screen of the front window, the terminal Tab and its Chat Tab become Unread.

R-AGENT-061. When the app's window is not front, or the Tab is Unread for 3 s, the app sends a system notification: title = the Tab's title, body = "needs your answer" or "finished", click goes to the Tab. The setting `agents.notify` (default on) turns notifications off. The setting `agents.notify_sound` (default off) adds the sound.

R-AGENT-062. Showing the Tab clears Unread. Focus on the Tab's Leaf is not required.

### Tasks

R-AGENT-070. Shift+Cmd+N on a terminal Tab with an Agent, or "New task" in the Panel, opens the Task sheet with: the prompt (multi-line, required), the mode (`fresh` or `fork`), the place (`same folder` or `new worktree`), the branch name for a worktree (default `task/<slug>` where the slug is the first 5 words of the prompt in lower-case with `-`), and Start.

R-AGENT-071. `fresh` runs a new Agent of the same kind in the chosen folder with `<agent> "<prompt>"`.

R-AGENT-072. `fork` copies the source Transcript to a new file with a new session id, rewrites the `sessionId` of every record, and runs `claude --resume <new id> "<prompt>"`. Fork is for Claude Code only; the sheet hides the mode for Codex.

R-AGENT-073. `new worktree` runs `git worktree add -b <branch> ../<repo>-<slug> HEAD` in the source folder's repository, with a 60 s limit. The Task's folder is the new worktree. When the folder is not inside a git repository the sheet hides the place.

R-AGENT-074. The Task opens a new terminal Tab in the same terminal Bench, in a Split right of the source Leaf, with the Session's `cwd` set to the Task's folder and `argv` from R-AGENT-071 or R-AGENT-072.

R-AGENT-075. The prompt goes on the command line, so the app does not wait for the Agent's composer. `[later]` interactive start with a poll as herdr-go's `task.go` does, for Agents that take no prompt argument.

R-AGENT-076. A Task that fails (worktree error, binary not found) shows the error in a Toast and in the sheet; the sheet stays open.

### Switcher

R-AGENT-080. The Switcher lists every Agent as "<kind> · <Tab title> · <Lab>" with its status icon. Enter goes to the terminal Tab; Cmd+Enter opens the Chat.

## MCP tools, domain `agents`

| Tool | Params | Returns | Access level |
|---|---|---|---|
| `agents.list` | `lab` (optional) | every Agent: `{session, tab, lab, bench, kind, status, since, unread}` | Read |
| `agents.get` | `session` | the Agent plus the last 50 Items of its Conversation as text | Read |
| `agents.prompt` | `session`, `text` | `{}` after the write; error `blocked` when the status is `blocked` | Full |
| `agents.task` | `session`, `prompt`, `mode`, `place`, `branch` (optional) | `{session, tab}` of the new Agent | Full |
| `agents.wait` | `session`, `status` (list), `timeout_ms` (max 600 000) | the status reached, or error `timeout` | Read |
| `[later]` `agents.approve` | `session`, `option` | | Full |

R-AGENT-090. A Client cannot prompt the Agent whose own Bridge it is: the server compares the Client's process tree with the Session's and answers error `self`.

## Failure modes

| Case | Behavior |
|---|---|
| Transcript missing | R-AGENT-025 |
| the Agent changed its prompt text in a new version | status by R-AGENT-013; the log line of R-AGENT-016 helps update the table |
| the session file names a pid that is reused | R-AGENT-021 start-time check; no Transcript, R-AGENT-025 |
| `/clear` in the Agent | the session file changes; the Chat empties and follows the new Transcript within 1 s |
| the Daemon restarts | every Agent is gone, then detected again within 2 s; Chat Tabs re-attach by the Session id when it still exists |
| two Agents in one Session (nested) | the one nearest the shell is the Agent; the other is ignored |

## Settings

| Key | Type | Default |
|---|---|---|
| `agents.fold_turns` | bool | true |
| `agents.notify` | bool | true |
| `agents.notify_sound` | bool | false |
| `agents.claude_config_dir` | string | empty = from the process |
| `agents.codex_home` | string | empty = from the process |

## Tests

- Daemon detection with a fake process tree table.
- Status rules with Screen fixtures captured from Claude Code 2.x and Codex, one file per rule.
- Transcript tests copied from herdr-go.
- Chat rendering with `ui.Tester`: 1 000 Items, fold and unfold, pending Prompt timeout.
- Task: fork rewrite of session ids on a fixture JSONL; worktree command with a temp repository.
