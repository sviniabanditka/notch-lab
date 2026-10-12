# notch-lab

A native macOS development environment that lives under the notch and in a full window. One app holds terminals, a browser, an editor, files, notes, an HTTP client, a database manager, SSH hosts, pull requests and coding agents. Agents control all of it through MCP.

This file is the vocabulary. Use these words in code, documents and the interface. Do not use the words marked _Avoid_.

All documents in this repository are written in ASD-STE100 Simplified Technical English. See `docs/spec/00-writing.md`.

## Structure

**Lab**:
The top-level group of work. The user selects one Lab at a time in the Lab menu. A Lab can have a root folder on disk.
_Avoid_: workspace, space, project

**Bench**:
One typed area inside a Lab. A Bench has exactly one type: terminal, browser, editor, files, notes, http, db, ssh, pull-requests or agents. The rail shows the Benches of the selected Lab.
_Avoid_: project, module, area, view

**Bench type**:
The kind of a Bench. The type decides what a Tab is, what the panel shows and what commands apply.
_Avoid_: kind, mode

**Tab**:
One item open in a Bench: a terminal session, a page, a file, a folder, a note, a request, a query, an SSH session, a pull request or an agent. The sidebar lists the Tabs of the selected Bench.
_Avoid_: page, document, item, buffer

**Panel**:
The part of a Bench that is not a Tab: the file tree of an editor Bench, the collections of an http Bench, the connections of a db Bench, the hosts of an ssh Bench. The panel shows in the sidebar below the Tabs.
_Avoid_: tree, explorer (that is the editor's panel's name), list

**Root folder**:
The folder on disk a Lab or a Bench works in. A Bench uses its Lab's root folder unless the user sets one for the Bench.
_Avoid_: project folder, cwd (that is a terminal's current folder), repository

**Layout**:
The tree of splits that arranges the Tabs a Bench shows on screen. Each leaf shows one Tab. A Bench owns its Layout.
_Avoid_: grid, arrangement, split tree

**Split**:
A Layout node that divides an area in two along a direction at a ratio.
_Avoid_: divider (that is the handle), separator, pane

**Leaf**:
A Layout node that shows one Tab. The focused Leaf receives keyboard input.
_Avoid_: pane, cell, slot

**Divider**:
The handle the user drags between the two sides of a Split.
_Avoid_: split, separator, gutter

**Focus**:
The one Leaf that receives keyboard input. There is exactly one focused Leaf in the window.
_Avoid_: active, selected (that is a Tab in the sidebar), current

**State**:
Everything the app restores at start: Labs, Benches, Tabs, Layouts, the selected Lab and Bench, each Tab's own data. Stored in the state database.
_Avoid_: session (that is a terminal's), persistence, snapshot

## Windows

**Main window**:
The full window with the rail, the sidebar and the Layout. Opened with Cmd+N or at start.
_Avoid_: window (alone), full window, browser window

**Notch panel**:
The panel that drops down from the notch when the user clicks the notch, moves the pointer over it, or presses the notch shortcut. It shows the same State as the main window.
_Avoid_: notch window, popover, HUD

**Shell**:
The part of the window around the Layout: the rail, the sidebar, the title bar and the status bar.
_Avoid_: chrome, frame, UI

**Rail**:
The vertical column of Bench icons at the left edge of the Shell. One icon per Bench of the selected Lab.
_Avoid_: dock, toolbar, bench bar

**Sidebar**:
The column right of the rail that lists the Tabs of the selected Bench and shows the Bench's Panel. Cmd+B hides and shows it.
_Avoid_: tab bar, tab list, drawer

**Lab menu**:
The menu at the top of the rail that selects a Lab.
_Avoid_: dropdown, workspace switcher, picker

**Switcher**:
The Cmd+K palette that finds a Lab, Bench, Tab, agent or pull request by a fuzzy query and goes to it.
_Avoid_: palette, command palette, quick open

**Go to File**:
The Cmd+P palette that finds a file under the focused Bench's root folder by a fuzzy query on its path.
_Avoid_: file finder, quick open, Switcher

**History**:
The Tabs the window showed, in order, which Back and Forward walk as a browser does.
_Avoid_: recent, MRU, navigation stack

**Unread**:
A Tab whose agent became done or blocked, or whose request or query finished, while the Tab was not on screen. It stays unread until the user shows the Tab.
_Avoid_: notification, badge (that is the drawing), new

**Toast**:
A short message the app shows at the bottom of the window for a few seconds.
_Avoid_: notification (that is the system's), alert, banner

**Shortcut**:
A Cmd key combination bound to a command.
_Avoid_: hotkey, keybinding, accelerator

## Terminal

**Daemon**:
The background process `notch-labd` that owns every terminal session. The app connects to it. Sessions continue when the app quits.
_Avoid_: server, service, agent (that is a coding agent), helper

**Session**:
One pseudo-terminal the Daemon runs: a shell, a program or a coding agent, with its scrollback. A terminal Tab shows one Session.
_Avoid_: terminal, pane, shell (that is the program), pty

**Screen**:
The visible cells of a Session at its current size.
_Avoid_: surface, frame, buffer

**Scrollback**:
A Session's lines above the Screen. The Daemon keeps them up to a limit.
_Avoid_: history, buffer, log

**Emulator**:
The code that turns a Session's byte stream into cells. The app runs one Emulator per terminal Tab.
_Avoid_: vt, parser, renderer (that paints cells)

## Browser

**Page**:
One web page open in a browser Tab. A browser Tab holds exactly one Page.
_Avoid_: tab (in prose about the web), site, document

**Web view**:
The WebKit view that shows a Page. One per browser Tab.
_Avoid_: webview (one word), browser, engine

**Profile**:
A set of cookies, storage and extension data. A Lab uses the shared Profile unless the user sets a browser Bench to an isolated Profile.
_Avoid_: container, data store, space

**Extension**:
A Chrome or Firefox WebExtension loaded into the browser through WKWebExtension.
_Avoid_: add-on, plugin, addon

**Store**:
The Chrome Web Store or addons.mozilla.org. The app installs Extensions from a Store page with its own install button.
_Avoid_: marketplace, shop

**Shim**:
The script the app puts in front of an Extension's scripts to supply Chrome APIs WebKit does not have.
_Avoid_: polyfill, patch, bridge

**Probe**:
The script the app injects into a Page to collect console messages, network requests and errors for agents and for the user.
_Avoid_: devtools, inspector (that is Safari's Web Inspector), tracer

## Editor

**File**:
One file open in an editor Tab. The Tab holds the file's Buffer.
_Avoid_: document, source

**Buffer**:
The text of a File in memory with its undo history. Saved to disk on Save or by autosave.
_Avoid_: document, model, text

**Explorer**:
The editor Bench's Panel: the tree of the root folder. Entries ignored by git show dimmed.
_Avoid_: file tree, navigator, file browser

**Preview**:
A read-only view of a File the editor cannot edit: an image, a rendered markdown file, a binary file.
_Avoid_: viewer, reader

**Selection**:
The text between two cursors in a Buffer. A Buffer can have several cursors.
_Avoid_: range, highlight, region

## Files

**Folder view**:
A files Tab: one folder shown as a list with a path bar.
_Avoid_: finder, directory listing, explorer

## Notes

**Note**:
One markdown file in a Lab's notes folder, open in a notes Tab.
_Avoid_: document, page, memo

## HTTP

**Request**:
One HTTP request stored as a `.http` file: method, URL, headers, body, auth. An http Tab holds one Request.
_Avoid_: call, endpoint, item

**Collection**:
A folder of Requests. The http Bench's Panel lists Collections.
_Avoid_: group, workspace, project

**Environment**:
A named set of variables a Request can use as `{{name}}`. Secret values live in the Keychain.
_Avoid_: env, profile, config

**Response**:
What the server returned for a Request: status, time, size, headers, body.
_Avoid_: result, reply, output

## Database

**Connection**:
Saved settings to reach one database server: engine, host, port, database, user. The password lives in the Keychain. The db Bench's Panel lists Connections.
_Avoid_: datasource, server, link

**Query**:
One SQL text and its last Result, open in a db Tab against one Connection.
_Avoid_: script, statement (that is one SQL statement inside a Query), editor

**Result**:
The rows a Query returned, shown in a grid with pages.
_Avoid_: resultset, output, data

**Tunnel**:
An SSH port forward the app opens so a Connection can reach a server behind a host.
_Avoid_: proxy, bastion, jump

## SSH

**Host**:
Saved settings to reach one machine over SSH: name, hostname, port, user, key. The ssh Bench's Panel lists Hosts.
_Avoid_: server, machine, connection

**SSH session**:
A Session the Daemon runs with `ssh <host>`, shown in an ssh Tab.
_Avoid_: remote terminal, shell

## GitHub

**Pull request**:
A GitHub pull request involving the user. The pull-requests Bench lists open ones and shows one's checks, comments and diff.
_Avoid_: PR (in prose), merge request, change

**Review scope**:
Which pull requests the list shows: Review, Mine, Involved, Issues or Search.
_Avoid_: filter, inbox

**Check**:
One CI result on a pull request's head commit: passed, failed or running.
_Avoid_: build, job, status

## Agents

**Agent**:
A coding agent (Claude Code, Codex) the Daemon detected in a Session.
_Avoid_: bot, assistant, AI

**Agent status**:
What an Agent does now: idle, working, blocked (waits for the user) or done.
_Avoid_: state, activity

**Transcript**:
The Agent's own JSONL record of its conversation, which the app follows to draw the chat.
_Avoid_: log, history

**Chat**:
The agents Bench's view of one Agent's Transcript, with a composer that sends text to the Agent's Session.
_Avoid_: thread, conversation (that is the parsed Transcript), chat view

**Task**:
A new Agent the user starts from an existing one with a prompt: fresh (new session, same folder) or fork (continues a copy of the conversation), in the same folder or in a new git worktree.
_Avoid_: job, run, thread

## MCP

**Bridge**:
The `notch-lab --mcp` process an agent client starts. It speaks MCP on stdio and forwards to the app's socket.
_Avoid_: server (the app is the server), proxy, adapter

**Client**:
One agent process connected through a Bridge. Each Client has an id and owns the Tabs it created.
_Avoid_: agent (that is a detected coding agent), session, connection

**Access level**:
How much a Client may do: Off, Read or Full. Set globally and per tool domain.
_Avoid_: permission, mode, scope

**Tool domain**:
One group of MCP tools: lab, terminal, browser, editor, http, db, ssh, notes, pull-requests, agents.
_Avoid_: namespace, module, category

**Audit log**:
The list of every tool call a Client made, with its result, shown in Settings.
_Avoid_: history, trace

## Settings

**Settings**:
The user's preferences, stored in `settings.json` in Application Support. Secrets never go there.
_Avoid_: preferences, config, options

**Keychain**:
The macOS keychain, where the app stores every password, token and secret variable.
_Avoid_: vault, secret store, credentials
