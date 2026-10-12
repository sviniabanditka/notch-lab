# ssh Bench

The ssh Bench keeps Hosts and opens SSH sessions to them. An SSH session is a Session the Daemon runs with the system `ssh`.

## Hosts

R-SSH-001. A Host has: id, name, hostname, port (default 22), user, identity file (optional), jump host (optional, a Host id or an ssh alias), extra ssh options (a list of `-o Key=Value` strings), color, tags (a list of strings), order index.

R-SSH-002. Hosts are stored in `$NOTCH_LAB_HOME/ssh/hosts.json` as an array. The file is written after every change. Hosts are global, not per Lab.

R-SSH-003. Passwords for Hosts `[later]`. In v1 the app relies on keys and the ssh-agent; a server that asks for a password asks in the terminal.

R-SSH-004. The app imports Hosts from `~/.ssh/config`. The parser reads `Host` blocks and the keys `HostName`, `Port`, `User`, `IdentityFile`, `ProxyJump`. A `Host` with a wildcard (`*`, `?`) is skipped. `Include` lines are followed one level deep.

R-SSH-005. Imported Hosts show in the Panel under "ssh config" with an import marker. The app re-reads `~/.ssh/config` on change, within 1 s.

R-SSH-006. An imported Host cannot be edited. Its context menu has "Copy to my hosts", which creates a Host with the same values and no marker.

R-SSH-007. The Host sheet (new, edit) has a field per attribute of R-SSH-001. Hostname is required. The identity file field has a file picker that starts in `~/.ssh`.

R-SSH-008. Deleting a Host asks for confirmation. Open SSH sessions to that Host stay open.

## Panel

R-SSH-010. The Panel lists Hosts grouped by tag. A Host with several tags appears under each. A Host without tags appears under "Hosts". Imported Hosts appear under "ssh config".

R-SSH-011. The Panel header has a search field. The list shows Hosts whose name, hostname, user or tag contains the query.

R-SSH-012. Panel actions:

| Action | Where | Effect |
|---|---|---|
| Connect | click, Enter, or Cmd+T with a Host selected | opens an SSH session Tab |
| New Host | header "+" | the Host sheet |
| Edit | context menu | the Host sheet |
| Delete | context menu, Backspace | R-SSH-008 |
| Copy ssh command | context menu | copies the command of R-SSH-021 |
| Copy to my hosts | context menu, imported only | R-SSH-006 |

R-SSH-013. Ping status per Host `[later]`.

## SSH session Tab

R-SSH-020. A Tab holds one SSH session. Type data: `{"host": "<host id or alias>", "session": "<daemon session id>"}`. The title is the Host name.

R-SSH-021. The Daemon starts the Session with this command:

| Host kind | Command |
|---|---|
| own Host | `/usr/bin/ssh -o ServerAliveInterval=30 -o ServerAliveCountMax=3 -p <port> -l <user> [-i <identity>] [-J <jump>] [<extra options>] <hostname>` |
| imported Host | `/usr/bin/ssh -o ServerAliveInterval=30 -o ServerAliveCountMax=3 <alias>` |

A jump host that is a Host id expands to `<user>@<hostname>:<port>` of that Host.

R-SSH-022. The Session's environment is the user's login shell environment, with `TERM=xterm-256color` and `NOTCH_LAB=1`. The Session's working folder is the home folder.

R-SSH-023. The Tab shows the same terminal view as a terminal Tab, with the same shortcuts, selection, links, search and Emulator. See `03-terminal.md`.

R-SSH-024. Host key prompts and password prompts show in the terminal. The user answers there. The app adds no `StrictHostKeyChecking` option.

R-SSH-025. When the Session ends, the terminal shows the last Screen, then a line "ssh exited with code <n>. Press Enter to reconnect." in the secondary color. Enter starts a new Session with the same command in the same Tab. Any other key does nothing.

R-SSH-026. At restore, the Tab re-attaches to its Session when the Daemon still has it. Otherwise the Tab shows "Disconnected. Press Enter to reconnect." and Enter connects.

R-SSH-027. The Tab's current folder follows OSC 7 from the remote shell. The sidebar shows the folder under the title when known. Without OSC 7 the folder is unknown and nothing shows.

R-SSH-028. The Tab's title stays the Host name. OSC 0 and OSC 2 from the remote shell show as a subtitle under the title in the sidebar.

R-SSH-029. SFTP browsing `[later]`. Port forwards `[later]`.

R-SSH-030. Closing the Tab asks for confirmation when the Session is alive, then ends the Session in the Daemon.

## Shortcuts

| Shortcut | Command |
|---|---|
| Cmd+T | Connect to the selected Host, or show the Host list |
| Cmd+W | Close the Tab |
| Enter (ended Session) | Reconnect |
| Cmd+Shift+C | Copy ssh command (Panel) |

## MCP tools, domain `ssh`

R-SSH-040. Tools:

| Tool | Input | Output | Level |
|---|---|---|---|
| `ssh.hosts.list` | `{tag?}` | Hosts: id, name, hostname, port, user, tags, imported flag | Read |
| `ssh.session.open` | `{host}` | the Tab id and the Session id | Full |
| `ssh.session.run` | `{host, command, timeout?}` | stdout and stderr merged, the exit code, timed-out flag | Full |

R-SSH-041. `ssh.session.open` creates an SSH session Tab on the focused ssh Bench, or on the first ssh Bench of the selected Lab, or on a new ssh Bench named "ssh". The Client owns the Tab.

R-SSH-042. `ssh.session.run` starts a Session with the Host's command plus the argument `-- <command>` as a single shell string, collects the output until exit or the timeout (default 60 s, max 600 s), then ends the Session. The Session shows in a Tab the Client owns and the Tab closes after the result is returned. Output over 1 MiB is cut and marked `truncated: true`.

R-SSH-043. A host key prompt during `ssh.session.run` does not get an answer; the run times out and returns `host_key_prompt: true`. The Toast tells the user to connect once by hand.

R-SSH-044. Every Full tool call shows a Toast "<client> opened ssh to <host>" or "<client> ran a command on <host>".
