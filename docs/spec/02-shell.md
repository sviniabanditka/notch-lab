# Shell: windows, notch panel, rail, sidebar, Switcher, shortcuts, Settings

Vocabulary: `CONTEXT.md`. Model: `01-model.md`. Fork features used here: `14-mygo-fork.md`.

## Main window

R-SHELL-001. The app opens one main window at start when `window.main.open` was true at the last quit, or at first run. Cmd+N opens the main window when it is closed. Cmd+N does nothing when the main window is open; the window comes to the front.

R-SHELL-002. The main window's minimum size is 720 x 480 pt.

R-SHELL-003. The main window uses `TitleBarHidden`. The Shell draws the title bar: a 38 pt strip under the traffic lights that shows the focused Tab's title and is a drag area (`DragWindow`).

R-SHELL-004. The main window uses `Vibrancy` under the rail and the sidebar. The Layout area has an opaque background from the theme.

R-SHELL-005. The main window restores its frame from State row `window` with key `main`. A frame that lies fully outside every display is replaced by the primary display's centered 1200 x 800 pt frame.

R-SHELL-006. The main window supports macOS full screen. The Shell keeps the rail and the sidebar in full screen.

R-SHELL-007. Closing the main window with Cmd+W when no Tab is focused, or with the red button, hides the window. The app keeps running with the Dock icon and the tray icon. Cmd+Q quits.

R-SHELL-008. The main window has one focused Leaf. Keyboard input goes to the Tab in the focused Leaf. Focus moves with a click on a Leaf, with Cmd+Alt+arrows, and with the Switcher.

## Notch panel

> M5 status (2026-10-12). The notch panel is a second Shell (window key `notch`) in a screen flyout with `KeyOnClick`, at window level 25 (`NSStatusWindowLevel`), so AppKit keeps its top at the top of the display. The click trigger is an `NSEvent` global monitor of primary presses in notch-lab, not a fork patch. `[later]` in M5: the MCP trigger (M6), the static Snapshot of a Page in the other window (R-SHELL-017), the sidebar over the Layout in size S (R-SHELL-018). Dev runs: `nl panel` toggles the panel, `nl pshot <name>` captures it.

R-SHELL-009. While the notch panel shows a Tab in its Layout, the main window's Leaf of that Tab shows the Tab's title and "Shown in the notch panel", and the Tab's view draws in the panel only. A browser Tab's Web view moves to the window that draws the Tab (R-SHELL-017). The main window shows the Tab again when the panel hides.

R-SHELL-010. The notch panel is a non-activating panel window (`Panel: true`, `Level: floating`, `CollectionBehavior: canJoinAllSpaces | fullScreenAuxiliary | stationary`). The front app keeps keyboard focus until the user clicks or types inside the notch panel.

R-SHELL-011. The notch panel shows when one of these occurs:

| Trigger | Condition |
|---|---|
| click on the notch | the pointer is inside `Screen.Notch()` and the primary mouse button goes down |
| hover over the notch | the pointer stays inside `Screen.Notch()` for 300 ms |
| global shortcut | Alt+Space, from any app |
| tray menu | "Show panel" |
| MCP | a Client asks for the "notch" view mode (`13-mcp.md`) |

R-SHELL-012. The notch panel hides when one of these occurs:

| Trigger |
|---|
| the user clicks outside the notch panel |
| the user presses Esc while the notch panel has focus and the focused Bench is not a terminal or an editor Bench, which use Esc |
| the user presses Alt+Space while the notch panel shows |
| the user clicks the "Move to window" button in the notch panel's title strip |
| the display with the notch is removed |

R-SHELL-013. The notch panel sits with its top edge at the top of the display with the notch, centered on the notch's x center. Its sizes:

| Size | Width | Height |
|---|---|---|
| S | 720 pt | 480 pt |
| M | 1000 pt | 640 pt |
| L | 1280 pt | 800 pt |

A size wider than the display's work area minus 40 pt is clamped to that width. The height is clamped to the work area height minus 40 pt. The user selects the size with the S/M/L control in the notch panel's title strip. The size is stored in `settings.json` key `notch.size`.

R-SHELL-014. The notch panel slides down from the notch in 180 ms with ease-out, and slides up in 120 ms. During the slide the panel ignores mouse events.

R-SHELL-015. On a Mac with no notch (`Screen.Notch()` false, also on an external display) the notch panel slides from the top center of the primary display. The hover trigger uses a 8 pt strip at the top center, 200 pt wide, with a 500 ms hover delay instead of 300 ms, because that strip is also the menu bar. The click trigger is not available there. The Settings key `notch.hoverShows` turns the hover trigger off; Alt+Space, the tray menu and the Window menu stay.

R-SHELL-016. The notch panel shows the same State as the main window. It has its own selected Lab, Bench and focused Tab, stored in State row `window` with key `notch`. A change made in one window shows in the other within one frame.

R-SHELL-017. The same Tab can be in a Leaf of the notch panel and in a Leaf of the main window. The Tab's view is shared: a terminal Tab draws the same Session, a browser Tab shows the same Page. While the panel shows the Tab, the main window shows a placeholder (R-SHELL-009). A Page can be in one Web view only: the window that draws it owns the Web view. A static Snapshot of the Page in the other window, with a "Show here" button, is `[later]`.

R-SHELL-018. The notch panel's title strip holds: the Lab menu, the S/M/L control, the "Move to window" button, the focused Tab's title. The rail and the sidebar are present in sizes M and L. In size S the sidebar is hidden by default and Cmd+B shows it over the Layout.

R-SHELL-019. The "Move to window" button opens the main window, selects the notch panel's Lab, Bench and Tab there, and hides the notch panel.

R-SHELL-020. While the notch panel shows, the app's activation policy stays as it was. The app keeps the regular policy (a Dock icon, R-SHELL-030), so typing into the panel needs no switch. The policy never switches while any window is in full screen (`14-mygo-fork.md`, R-MYGO-016).

R-SHELL-022. The menu bar belongs to the window with the keyboard: while the notch panel is the key window, the menus act on the panel's Shell. Cmd+N opens the main window from either.

R-SHELL-021. The hover trigger is off while a drag with the primary button is in progress, and for 1000 ms after the notch panel hid.

Acceptance: open TextEdit, hover the notch 300 ms, the notch panel shows, TextEdit keeps the key window; type in TextEdit, the text goes to TextEdit; click in the notch panel's terminal Tab, type, the text goes to the Session; click outside, the notch panel hides.

## Dock and tray

R-SHELL-030. The app shows a Dock icon. The Dock menu lists "New window" and every Lab; a Lab item opens the main window on that Lab.

R-SHELL-031. The app shows a tray icon in the menu bar. Its menu:

| Item | Action |
|---|---|
| Show panel | shows the notch panel |
| Show window | opens the main window |
| separator | |
| one item per Lab, with a dot when the Lab has an Unread Tab | opens the main window on that Lab |
| separator | |
| Settings… | opens Settings |
| Quit notch-lab | quits |

R-SHELL-032. The tray icon shows a dot when any Tab is Unread.

## Rail

R-SHELL-040. The rail is 48 pt wide at the left edge of the Shell. It holds, top to bottom: the Lab menu button, one icon per Bench in order, the "+" button, a spacer, the Settings button.

R-SHELL-041. The Lab menu button is 40 x 40 pt and shows the Lab's color dot (10 pt) and the Lab's initial letter. A click opens the Lab menu: every Lab in order, each with its color dot, name and an Unread dot; a separator; "New Lab…"; "Lab settings…".

R-SHELL-042. A Bench icon is 32 x 32 pt inside a 40 x 40 pt hit area. The selected Bench's icon has the accent background. A Bench with an Unread Tab shows a 6 pt dot at the icon's top right.

R-SHELL-043. A Bench icon is the type's icon by default (Lucide shapes). `[later]` The user sets a custom icon from a fixed set of 40 symbols from the context menu.

R-SHELL-044. The user drags a Bench icon to reorder. The drop position shows as a 2 pt accent line. The order is written to State.

R-SHELL-045. The Bench context menu:

| Item | Action |
|---|---|
| Rename… | inline edit of the name |
| Set root folder… | folder dialog; "Clear" returns to the Lab's root folder |
| Change icon… | the icon set |
| Bench settings… | the type's settings sheet (R-MODEL-017) |
| separator | |
| Delete Bench | confirmation when the Bench has Tabs (R-MODEL-016) |

R-SHELL-046. The "+" button opens the "New Bench" sheet: the type list with icons and one-line descriptions, a name field with the type's name, a root folder field with the Lab's root folder, Create and Cancel. Enter creates.

R-SHELL-047. Hover over a Bench icon for 500 ms shows a tooltip: the Bench name and its effective root folder.

R-SHELL-048. Cmd+Shift+1 to Cmd+Shift+9 select the Bench at that position. A position with no Bench does nothing.

## Sidebar

R-SHELL-050. The sidebar is right of the rail, 240 pt wide by default. The user drags its right edge between 160 and 480 pt. The width is stored in `settings.json` key `sidebar.width`.

R-SHELL-051. Cmd+B hides and shows the sidebar. The state is per window, stored in State row `window` column `sidebar`.

R-SHELL-052. The sidebar holds, top to bottom: the Bench header (name, Tab count, "new Tab" button), the Tab list, a divider, the Panel.

R-SHELL-053. The Tab list shows pinned Tabs first under a "Pinned" caption, then the other Tabs in order. A row is 28 pt: the type's icon or favicon (16 pt), the title, a status glyph the type supplies, an Unread dot, a close button on hover.

R-SHELL-054. The selected Tab (the one in the focused Leaf) has the accent background. A Tab that is in another Leaf of the Layout has a muted background. A Tab in no Leaf has no background.

R-SHELL-055. A click on a Tab row shows the Tab in the focused Leaf (R-MODEL-033). Alt+click opens it in a new Split (R-MODEL-035). `[later]` Middle-click closes it (mygo reports no middle click).

R-SHELL-056. The user drags a Tab row to reorder. A pinned Tab can be dropped among pinned Tabs only.

R-SHELL-057. The Tab context menu:

| Item | Action |
|---|---|
| Pin / Unpin | toggles pinned |
| Rename… | sets the custom title; an empty name clears it |
| Open in new Split | R-MODEL-035 |
| separator | |
| Close | closes the Tab |
| Close others | closes every other unpinned Tab of the Bench |
| Close to bottom | closes every unpinned Tab below this one |
| separator | |
| Reveal | the type's reveal action: in the Explorer, in Finder, in the Store |

R-SHELL-058. `[later: with the first Bench type that has a Panel, M3]` The divider between the Tab list and the Panel is draggable. The Panel height is between 80 pt and the sidebar height minus 120 pt. A Bench type with no Panel shows no divider and the Tab list fills the sidebar.

R-SHELL-059. Cmd+1 to Cmd+9 select the Tab at that position in the visible list (pinned first). A position with no Tab does nothing.

R-SHELL-060. Cmd+Shift+] and Cmd+Shift+[ select the next and previous Tab in the list, with wrap.

## Status bar

R-SHELL-070. The status bar is 24 pt high at the bottom of the window, over the full width right of the rail.

R-SHELL-071. The status bar shows, left to right: the focused Tab's status text supplied by its type (truncated with an ellipsis at the right), a spacer, the Daemon status, the MCP Client count, the Unread count.

R-SHELL-072. The Daemon status shows a green dot and "daemon" when connected, a red dot and "daemon off" when not. A click on it restarts the connection.

R-SHELL-073. The MCP Client count shows "n agents" when n is greater than 0, else nothing. A click opens Settings on the Agents/MCP section.

R-SHELL-074. The Unread count shows "n unread" when n is greater than 0. A click goes to the oldest Unread Tab.

## Switcher

R-SHELL-080. Cmd+K opens the Switcher as a sheet at the top center of the window, 600 pt wide, up to 12 rows. Esc closes it.

R-SHELL-081. The Switcher searches these items with a fuzzy match on their name and path:

| Item | Shown as | Enter |
|---|---|---|
| Lab | color dot, name | selects the Lab |
| Bench | type icon, Lab › Bench | selects the Lab and Bench |
| Tab | type icon, Lab › Bench › Tab | selects the Lab and Bench, shows the Tab in the focused Leaf |
| Agent | agent glyph with status, Lab › Bench › Tab | as Tab |
| Pull request | number, title, repo | opens the Pull request in a pull-requests Bench of the current Lab |

R-SHELL-082. With an empty query the Switcher lists the 12 most recently shown Tabs across every Lab, most recently shown first, then Benches, then Labs.

R-SHELL-083. The match ranks: an exact prefix of the name first, then matches with every query character in order with the fewest gaps, then by MRU time. The match is case-insensitive.

R-SHELL-084. Cmd+Enter opens a Tab result in a new Split right of the focused Leaf. For a result of another Bench, the Bench is selected first.

R-SHELL-085. The last two rows are actions: "New Bench…" (R-SHELL-046) and "New Lab…". They show for every query, after the results.

R-SHELL-086. A result row shows the match characters in bold. Up and Down move the selection, with wrap. Typing resets the selection to the first row.

R-SHELL-087. The Switcher answers a query over 10 000 items in 50 ms. Acceptance: a `ui.Tester` run with 10 000 generated Tabs measures the time from the key event to the rendered rows.

## Go to File

R-SHELL-090. `[M3]` Cmd+P opens Go to File with the same sheet as the Switcher. It uses the `filesearch` package copied from herdr-go.

R-SHELL-091. The root is the focused Bench's effective root folder. The index holds the files git tracks under the root, or every file when the root is outside a git repository. Entries over 200 000 files stop the index and the sheet shows "Index limit reached".

R-SHELL-092. Enter opens the file in an editor Bench of the current Lab: the first editor Bench whose effective root folder contains the file, else the first editor Bench, else a new editor Bench named "Editor" on the current root folder.

R-SHELL-093. Cmd+Enter reveals the file in that editor Bench's Explorer without opening it.

R-SHELL-094. The index refreshes when the sheet opens and the root's git index changed, or after 30 s.

## History

R-SHELL-100. Each window has a History of the Tabs it showed in the focused Leaf, with the Leaf's position. Cmd+[ goes Back, Cmd+] goes Forward.

R-SHELL-101. Showing a Tab adds an entry and drops the entries ahead of the current one. Showing the same Tab again adds no entry. The History holds 200 entries.

R-SHELL-102. Back to a Tab that is closed skips that entry. Back to a Tab of another Bench selects that Bench. Back to a Tab of another Lab selects that Lab.

R-SHELL-103. A browser Tab consumes Cmd+[ and Cmd+] for its own page history when the Page can go back or forward. When the Page cannot, the window History acts.

## Unread

R-SHELL-110. A Tab becomes Unread when its type reports an Unread event (an Agent done or blocked, a Request or Query finished) and the Tab is not in a Leaf of a window that is on screen and in front.

R-SHELL-111. A Tab stops being Unread when it is shown in the focused Leaf of a window in front, or when the user closes it.

R-SHELL-112. Unread shows as a 6 pt accent dot on the Tab row, the Bench icon, the Lab menu item and the tray icon.

R-SHELL-113. The Unread flag is in State. It survives restart.

## Toast

R-SHELL-120. A Toast shows at the bottom center of the window, above the status bar, 360 pt wide at most, for 4 s. A click dismisses it. The pointer over it pauses the timer.

R-SHELL-121. At most 3 Toasts stack upward. A fourth replaces the oldest.

R-SHELL-122. A Toast can hold one action button. The MCP write-action Toast holds "Show" that focuses the Tab the tool changed (`13-mcp.md`). `[later]` "Undo".

## Shortcuts

R-SHELL-130. Shortcuts are fixed in v1. The Settings window shows them read-only. `[later]` user-defined shortcuts.

| Shortcut | Command | Scope |
|---|---|---|
| Cmd+N | open the main window | app |
| Cmd+T | new Tab of the selected Bench's type | window |
| Cmd+W | close the focused Tab; with no Tab, hide the window | window |
| Cmd+Shift+T | reopen the last closed Tab of the selected Bench | window |
| Cmd+1 … Cmd+9 | select Tab 1 … 9 | window |
| Cmd+Shift+1 … Cmd+Shift+9 | select Bench 1 … 9 | window |
| Cmd+Shift+] / Cmd+Shift+[ | next / previous Tab | window |
| Cmd+Ctrl+] / Cmd+Ctrl+[ | next / previous Bench | window |
| Cmd+Ctrl+Shift+] / Cmd+Ctrl+Shift+[ | next / previous Lab | window |
| Cmd+D | split right | window |
| Cmd+Shift+D | split down | window |
| Cmd+Alt+Left / Right / Up / Down | focus the neighbouring Leaf | window |
| Cmd+Shift+Enter | zoom the focused Leaf | window |
| Cmd+K | Switcher | window |
| Cmd+P | Go to File | window |
| Cmd+B | toggle the sidebar | window |
| Cmd+[ / Cmd+] | History Back / Forward | window |
| Cmd+R | the type's reload: Explorer re-read, Page reload, Request send, Query run, Pull request refresh | window |
| Cmd+, | Settings | app |
| Cmd+Q | quit | app |
| Alt+Space | show / hide the notch panel | global |
| Cmd+Shift+F | find in the focused Tab, where the type supports it | window |
| Cmd+Alt+I | Web Inspector for a browser Tab | window |

R-SHELL-131. A Tab's type can consume a shortcut before the Shell. The type's document lists which. A terminal Tab passes every key without Cmd to the Session.

R-SHELL-132. Cmd+letter shortcuts go through the menu bar so that a Cyrillic or other layout maps them by key position.

## Menu bar

R-SHELL-140. The menu bar:

| Menu | Items |
|---|---|
| notch-lab | About notch-lab; Settings… (Cmd+,); Services; Hide, Hide Others, Show All; Quit notch-lab (Cmd+Q) |
| File | New Tab (Cmd+T); New Bench…; New Lab…; Reopen Closed Tab (Cmd+Shift+T); Close Tab (Cmd+W); separator; Save (Cmd+S, editor and notes); Save All (Cmd+Alt+S); Revert |
| Edit | Undo, Redo, Cut, Copy, Paste, Paste and Match Style, Select All; separator; Find (Cmd+F), Find and Replace (Cmd+Alt+F), Find Next, Find Previous; separator; Start Dictation, Emoji & Symbols |
| View | Toggle Sidebar (Cmd+B); Split Right (Cmd+D); Split Down (Cmd+Shift+D); Zoom Leaf (Cmd+Shift+Enter); Focus Left/Right/Up/Down; separator; Back (Cmd+[); Forward (Cmd+]); separator; Reload (Cmd+R); Actual Size, Zoom In, Zoom Out (browser, editor); separator; Enter Full Screen |
| Lab | one item per Lab (checkmark on the selected); separator; Next Lab, Previous Lab; Lab Settings…; Delete Lab… |
| Bench | one item per Bench of the selected Lab (Cmd+Shift+n); separator; Next Bench, Previous Bench; Rename Bench…; Set Root Folder…; Bench Settings…; Delete Bench… |
| Tab | one item per Tab of the selected Bench (Cmd+n for the first 9); separator; Next Tab, Previous Tab; Pin Tab; Rename Tab…; Open in New Split; Reveal |
| Window | Minimize, Zoom; Show Notch Panel (Alt+Space); Move to Window; separator; Bring All to Front; the window list |
| Help | notch-lab Help; Keyboard Shortcuts; Open Log Folder; Report a Problem… |

R-SHELL-141. Menu items whose command does not apply to the focused Tab are disabled.

## Theme

R-SHELL-150. The theme follows the system appearance: dark or light. `settings.json` key `appearance.theme` with values `system`, `dark`, `light` overrides it.

R-SHELL-151. The theme is mygo's default theme, which follows the system and the accent color; Settings sets its font size. `[later]` named presets generated by `tools/themegen` from herdr-go, user presets and import.

R-SHELL-152. A theme change applies to every open window within one frame, including the Settings window.

R-SHELL-153. The 16 ANSI colors for terminal Tabs are part of the theme.

## Settings

R-SHELL-160. Cmd+, opens the Settings window, 720 x 520 pt, with a section list at the left.

R-SHELL-161. Sections and their controls:

| Section | Controls |
|---|---|
| General | open main window at start; show notch panel on hover; notch size; default Lab at start (last used / named); confirm before closing a Tab with unsaved changes |
| Appearance | theme (system / dark / light); UI font size (12–16 pt); sidebar width; show status bar |
| Shortcuts | the table of R-SHELL-130, read-only |
| Terminal | font family and size; line height; Scrollback limit; cursor style; shell path (empty = login shell); bell (sound / none); copy on select |
| Browser | search engine; home page; download folder; ask where to save; block popups; default Profile; "Set as default browser" button |
| Extensions | the installed list with enable, pin, remove, reload, permissions; "Install from folder…"; "Install from file…"; "Open the Store" |
| Editor | font family and size; tab width; insert spaces; autosave (off / 1 s / on focus loss); trim trailing space on save; show line numbers; wrap |
| Agents/MCP | global Access level; per Tool domain Access level; Client list with disconnect; Audit log; "Connect to Claude Code", "Connect to Codex", "Connect to Cursor" buttons |
| Advanced | state folder path (read-only, "Reveal"); log level; "Open log folder"; "Reset State…" with confirmation |

R-SHELL-162. Settings live in `~/Library/Application Support/notch-lab/settings.json`. The app writes the file on every change, 100 ms batched. A missing or unreadable file means defaults; the app shows a Toast when the file was present but unreadable.

R-SHELL-163. The `settings.json` schema and defaults:

```json
{
  "version": 1,
  "general": {
    "openMainWindowAtStart": true,
    "confirmCloseUnsaved": true,
    "startLab": "last"
  },
  "notch": { "size": "M", "hoverShows": true, "hoverDelayMs": 300 },
  "appearance": { "theme": "system", "uiFontSize": 13, "showStatusBar": true },
  "sidebar": { "width": 240 },
  "window": { "main": { "open": true } },
  "terminal": {
    "fontFamily": "Menlo", "fontSize": 13, "lineHeight": 1.2,
    "scrollbackLines": 10000, "cursor": "block", "shell": "",
    "bell": "none", "copyOnSelect": false
  },
  "browser": {
    "searchEngine": "https://duckduckgo.com/?q=%s",
    "homePage": "about:blank",
    "downloadFolder": "~/Downloads", "askWhereToSave": false,
    "blockPopups": true, "defaultProfile": "shared"
  },
  "editor": {
    "fontFamily": "Menlo", "fontSize": 13, "tabWidth": 4, "insertSpaces": true,
    "autosave": "off", "trimTrailingSpace": false, "lineNumbers": true, "wrap": false
  },
  "mcp": {
    "access": "off",
    "domains": {},
    "toastOnWrite": true
  },
  "advanced": { "logLevel": "info" }
}
```

R-SHELL-164. A key not in the schema is kept as is and written back. A value of the wrong type is replaced by the default and the app logs it.

R-SHELL-165. Secrets never go in `settings.json`. The Keychain service name is `dev.sviniabanditka.notch-lab`. The account name is the item's path, for example `http/env/<environment id>/<variable>` or `db/<connection id>/password`.

R-SHELL-166. `[M7, with the first secret]` Settings on macOS Keychain use the data protection keychain through the Security framework via purego. A keychain read that needs the user's approval shows the system prompt once per item.

## Notifications

R-SHELL-170. `[M2, with the first Agent]` The app posts a system notification when an Agent becomes blocked or done and no window of the app is in front. The notification's title is the Agent's name and Lab, the body is the Agent's last line.

R-SHELL-171. A click on the notification opens the main window, selects the Tab, and clears Unread.

R-SHELL-172. The app asks for notification permission at the first Agent event, not at start.

## First run

R-SHELL-180. At first run (no `state.db`) the app creates Lab "Lab" with no root folder, a terminal Bench "Terminal" and an editor Bench "Editor" on the home folder, and opens the main window on the terminal Bench with one Session.

R-SHELL-181. At first run the app shows a one-time sheet with the main shortcuts. `[M4]` It adds a "Set as default browser" button.

## Accessibility

R-SHELL-190. Every control in the Shell is reachable with Tab and operable with Space or Enter.

R-SHELL-191. Every icon-only control has an accessibility label through mygo's label API.

R-SHELL-192. Focus has a visible 2 pt ring in the accent color.

R-SHELL-193. The Shell honours the system "Reduce motion" setting: the notch panel shows and hides without the slide.

## Window keys in State

R-SHELL-200. State row `window` holds one row per window key: `main`, `notch`, `settings`. Columns: `lab`, `bench`, `focused_tab`, `frame` (JSON `{x,y,w,h}`), `sidebar` (0 or 1).
