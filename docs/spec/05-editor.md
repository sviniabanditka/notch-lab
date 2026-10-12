# Editor Bench

> M3 status (2026-10-12). Built on `ui.TextBuffer` (a balanced tree of chunks with a line index: R-EDIT-010 and R-EDIT-011 hold with it in place of a piece table), `ui.TextInputClient` for input methods, and undo by persistent snapshots of the buffer. Key changes where the shortcuts collided with the Shell (R-SHELL-130):
> - Duplicate line is Alt+Shift+Down; Cmd+Shift+Down keeps "extend to the end".
> - Upper and lower case are Ctrl+Shift+U and Ctrl+Shift+L (Cmd+K is the Switcher).
> - Diff against HEAD is Cmd+Alt+G; Cmd+Shift+G is Find Previous.
> - In an editor Tab, Cmd+D adds the next occurrence; Split Right stays in the View menu.
> - The `line:col` status shows in the window's status bar, not a bar of the Leaf.
> - New file, new folder and rename ask in a sheet, not an inline field.
>
> `[later]` in M3: indent conversion (R-EDIT-063), whitespace rendering (R-EDIT-070), incremental lexing (R-EDIT-081: the whole text is lexed again 150 ms after an edit, off the main thread), word occurrence marks (R-EDIT-085), the scroll bar and its match marks (R-EDIT-094), soft wrap (R-EDIT-122), smooth scrolling (R-EDIT-125), italic transient titles (R-EDIT-144), the Explorer filter (R-EDIT-151) and its 5 000-entry cap (R-EDIT-152), the removed-lines popover (R-EDIT-172), a Diff Tab that follows the Buffer as it changes (R-EDIT-173), Undo of a move to the Trash.


The editor Bench is a native text editor drawn with mygo `ui`. It uses no web view. A Tab is one File. The Panel is the Explorer. The Preview shows files the editor cannot edit.

Code from herdr-go that this Bench copies:

| herdr-go package | Used for | Change |
|---|---|---|
| `internal/filepreview` | the Preview: images, markdown, binary and oversized placeholders | a Preview opens in a Tab, not over the pane area; the stack goes away |
| `internal/filesearch` | Go to File (Cmd+P): index by git or by a walk, fuzzy query | the root is the focused Bench's effective root folder |
| `internal/diffview` | the Diff Tab of a File against HEAD | none |
| `cmd/herdr-go/explorer.go` and the sidebar tree | the Explorer | the root follows the Bench, not a terminal pane; file operations are added |
| chroma palette in `internal/diffview/highlight.go` | token colors | moved to `internal/platform/highlight` and shared |

herdr-go has no editor. The Buffer and the editing commands are new code.

## Tab

R-EDIT-001. An editor Tab holds one File. Its type data is `{"path": "<absolute path>", "line": n, "col": n, "scroll": n, "preview": bool}`.

R-EDIT-002. "New Tab" (Cmd+T) creates an empty Buffer with no path, titled "Untitled". The first Save opens the Save As sheet.

R-EDIT-003. At restore, a Tab reads its File from `path` and scrolls to `scroll`. A Tab whose File is missing shows the placeholder "File not found: <path>" with one button, "Close".

R-EDIT-004. The Tab title is the file name. A Buffer with unsaved changes shows a dot before the name. When two Tabs of the Bench have the same file name, each title gets the parent folder's name after it: `api.go — model`.

R-EDIT-005. Opening a path that is already open in a Tab of the same Bench selects that Tab. It does not open a second Tab.

R-EDIT-006. Closing a Tab with unsaved changes asks: "Save", "Don't Save", "Cancel". Cmd+W on a Tab with no changes closes at once.

R-EDIT-007. The status bar at the bottom of the Leaf shows, left to right: `line:col`, the selection count when a Selection exists (`3 sel`, or `12 chars` for one), the encoding (`UTF-8`), the line ending (`LF` or `CRLF`), the indent (`Spaces: 4` or `Tabs`), the language name. Clicking the indent or the line ending opens a menu to change it.

## Buffer

R-EDIT-010. A Buffer is a piece table. The original file bytes are one read-only block. An add block receives every insert. A piece is `(block, offset, length)`. Edits change the list of pieces only.

R-EDIT-011. The Buffer keeps a line index: the byte offset of each line start. The index updates for the changed range after each edit. Line lookup by number is O(log n). Acceptance: a 10 MiB file with 200 000 lines reports any line start in under 1 ms.

R-EDIT-012. The Buffer reads UTF-8 only. A byte sequence that is not valid UTF-8 shows as U+FFFD. The Buffer keeps the original bytes and writes them back unchanged on Save when that range was not edited.

R-EDIT-013. The editor detects the line ending from the first line break in the File: `\r\n` is CRLF, else LF. A File with no line break uses the Settings default `editor.lineEnding` (LF). New line breaks use the detected ending. Save writes the detected ending.

R-EDIT-014. The editor keeps the File's final newline as found. A Setting `editor.insertFinalNewline` (off), or `.editorconfig` `insert_final_newline = true`, adds one on Save when missing.

R-EDIT-015. A File that starts with the UTF-8 BOM keeps it. The BOM is not shown and not editable. Save writes it back.

R-EDIT-016. Encodings other than UTF-8 open as a Preview with the note "Encoding not supported". `[later]` detect and convert UTF-16 and Latin-1.

R-EDIT-017. A File larger than 10 MiB opens as a Preview with the note "File too large to edit (<size>)". The Preview shows the first 1 MiB as plain text. The Setting `editor.maxEditSize` changes the limit.

R-EDIT-018. The editor reads a File off the main thread. The Tab shows a spinner after 100 ms. A read error shows the placeholder with the error text and "Close".

R-EDIT-019. The Buffer stores every edit in an undo stack. Edits join into one undo group when: they are typed less than 300 ms apart, they are at adjacent positions, and no cursor jump happened between them. A cursor jump is any move that is not caused by the edit. Cmd+Z undoes one group. Cmd+Shift+Z redoes one group. A new edit after an undo discards the redo stack.

R-EDIT-020. Undo restores the cursors and Selections that existed before the group.

R-EDIT-021. The undo stack has no limit inside one Tab. The stack is dropped when the Tab closes.

## Cursors and Selections

R-EDIT-030. A Buffer has one or more cursors. Each cursor has a position and an anchor. When the anchor differs from the position, the cursor has a Selection.

R-EDIT-031. Cursor commands:

| Command | Keys |
|---|---|
| add a cursor at the click | Cmd+click |
| select the next occurrence of the word under the cursor, or of the Selection, and add a cursor there | Cmd+D |
| select all occurrences, one cursor each | Cmd+Shift+L |
| column select | Alt+drag |
| add a cursor on the line above or below | Cmd+Alt+Up, Cmd+Alt+Down |
| back to one cursor (keeps the first cursor) | Esc |

R-EDIT-032. Cmd+D with no Selection first selects the word under the cursor. Each further Cmd+D selects the next occurrence after the last added cursor and wraps at the end of the Buffer. The search is case-sensitive and matches whole words when the first Selection was a word.

R-EDIT-033. Two cursors that land on the same position merge into one.

R-EDIT-034. Typing, deleting and every editing command apply to every cursor. Undo of a multi-cursor edit restores all cursors.

R-EDIT-035. Selection commands:

| Command | Keys |
|---|---|
| extend by one character | Shift+Left, Shift+Right |
| extend by one line | Shift+Up, Shift+Down |
| move or extend by one word | Alt+Left, Alt+Right, with Shift |
| move or extend to line start or end | Cmd+Left, Cmd+Right, with Shift |
| move or extend to Buffer start or end | Cmd+Up, Cmd+Down, with Shift |
| select all | Cmd+A |
| select the line | Cmd+L |
| expand the Selection to the enclosing bracket pair, then the next | Cmd+Shift+Up `[later]` |
| select the word | double click |
| select the line | triple click |

R-EDIT-036. A word is a run of letters, digits and `_`. Alt+Left from inside a run of spaces skips the spaces then the word.

R-EDIT-037. Home and End work as Cmd+Left and Cmd+Right. A second Cmd+Left on an indented line moves between the first non-space character and column 0.

R-EDIT-038. The Selection paints with the theme's selection color under the glyphs. The Selection of the focused Leaf is opaque. The Selection of an unfocused Leaf is at half opacity.

R-EDIT-039. Cmd+C copies the Selection. With no Selection, Cmd+C copies the whole line with its line ending. Cmd+X does the same and deletes. Cmd+V pastes at every cursor. A paste with as many lines as there are cursors puts one line at each cursor.

R-EDIT-040. The clipboard holds text only. Rich text is pasted as plain text.

## Editing commands

R-EDIT-050. Editing commands:

| Command | Keys | Effect |
|---|---|---|
| duplicate line | Cmd+Shift+Down | copies the line, or every line of the Selection, below |
| move line up or down | Alt+Up, Alt+Down | moves the line, or every line of the Selection, by one line |
| delete line | Cmd+Shift+K | deletes the line, or every line of the Selection |
| toggle comment | Cmd+/ | see R-EDIT-051 |
| indent | Tab with a Selection | adds one indent unit at the start of each selected line |
| outdent | Shift+Tab | removes one indent unit from the start of each selected line, or of the cursor's line |
| join lines | Ctrl+J | joins the next line to the current one with one space; trims leading spaces of the joined line |
| transpose characters | Ctrl+T | swaps the characters around the cursor |
| upper case | Cmd+K then Cmd+U | upper-cases the Selection |
| lower case | Cmd+K then Cmd+L | lower-cases the Selection |
| delete word left or right | Alt+Backspace, Alt+Delete | deletes to the word boundary |
| delete to line start or end | Cmd+Backspace, Cmd+Delete | deletes to the line start or end |
| insert line below or above | Cmd+Enter, Cmd+Shift+Enter | opens a new indented line without splitting the current one |

R-EDIT-051. Toggle comment uses the language's line comment when the lexer has one (`//`, `#`, `--`, `;`), else its block comment (`/* */`, `<!-- -->`). With no Selection it toggles the cursor's line. With a Selection it toggles every line. All lines get a comment when any selected line has none. All lines lose the comment when every selected line has one. The comment marker goes at the smallest indent of the lines, with one space after it. A blank line inside the Selection is not changed.

R-EDIT-052. Tab with no Selection inserts one indent unit at the cursor: spaces to the next tab stop, or one tab character.

R-EDIT-053. Enter inserts a line break and copies the previous line's leading whitespace. When the character before the cursor is an opening bracket and the character after is its closing bracket, Enter inserts two line breaks and puts the cursor on an extra-indented line between them.

R-EDIT-054. Auto-close: typing `(`, `[`, `{`, `"`, `'`, or a backtick inserts the pair and leaves the cursor between. Rules:
- a closing bracket is inserted only when the next character is a space, a line end, or a closing bracket;
- a quote is not auto-closed when the previous character is a letter, a digit or the same quote;
- typing a closing character when the next character is that character moves over it (overtype) and inserts nothing;
- Backspace between an auto-closed pair deletes both;
- with a Selection, typing an opening character wraps the Selection in the pair.
The Setting `editor.autoClose` (on) turns the rule off.

R-EDIT-055. Typing a closing bracket on a line with only whitespace before the cursor outdents the line by one unit when the matching opening bracket's line is less indented.

## Indentation

R-EDIT-060. The editor detects the indent of a File when the File opens: among the first 1 000 lines that start with whitespace, tabs win when more lines start with a tab than with spaces; else the width is the most common difference between indents of consecutive lines, from the set {2, 4, 8}. A File with no indented line uses the default.

R-EDIT-061. The default is Settings `editor.indent`: `{"style": "space", "size": 4}`.

R-EDIT-062. The editor reads `.editorconfig` files from the File's folder up to the root folder or a file with `root = true`. Supported keys: `indent_style`, `indent_size`, `tab_width`, `end_of_line`, `insert_final_newline`, `trim_trailing_whitespace`, `charset` (only `utf-8` accepted). `.editorconfig` wins over detection. Other keys are ignored.

R-EDIT-063. Changing the indent from the status bar changes new indents only. "Convert indentation to spaces/tabs" in the same menu rewrites the leading whitespace of every line as one undo group.

## Whitespace

R-EDIT-070. The Setting `editor.renderWhitespace` is `none`, `selection` (default) or `all`. Spaces paint as centered dots and tabs as arrows in the theme's faint color.

R-EDIT-071. The Setting `editor.trimTrailingWhitespace` (off), or `.editorconfig` `trim_trailing_whitespace = true`, removes trailing spaces and tabs from every line on Save, except on the cursor's line.

R-EDIT-072. Tabs paint at the next multiple of the tab width. A cursor inside a tab is not possible; Left and Right move over the whole tab.

## Highlighting

R-EDIT-080. The editor picks a chroma lexer by the file name, then by the extension, then by a `#!` shebang in the first line, then falls back to plain text. The user can set the language from the status bar for the Tab.

R-EDIT-081. Tokens are cached per line. After an edit, the editor re-lexes from the first changed line while the lexer state at the end of a line differs from the cached state, and stops at the first line whose end state matches. Chroma has no resumable state, so the editor lexes from the start of the changed line's enclosing block: the nearest line above with a cached "stable" mark, which is set on a line that ends outside any string or comment.

R-EDIT-082. Lexing runs on the main thread in slices of at most 16 ms per frame. Lines not yet lexed paint in the plain text color. A File of 100 000 lines is fully lexed within 2 s of opening on an M1.

R-EDIT-083. Token colors come from `internal/platform/highlight`, shared with the diff view and the Preview. The palette has one color per chroma token category for the dark theme and one for the light theme.

R-EDIT-084. Bracket matching: when the cursor touches `(`, `[`, `{` or their pair, the editor paints a box around both. The search is limited to 10 000 characters each way.

R-EDIT-085. After the cursor rests for 300 ms on a word with no Selection, every other whole-word occurrence of that word on screen paints with the theme's match color. Any edit or move clears the marks.

## Find and replace

R-EDIT-090. Cmd+F opens the find bar at the top of the Leaf with the Selection, or the word under the cursor, as the query. Cmd+Alt+F opens the bar with the replace field. Esc closes the bar and keeps the cursor on the current match.

R-EDIT-091. The bar has toggles: regex (Go RE2 syntax), match case, whole word, in Selection. It shows `<current> of <count>`, or "No results". The count stops at 10 000 and shows "10 000+".

R-EDIT-092. The search runs after each change to the query with a 100 ms delay, on the main thread in 16 ms slices. The first match at or after the cursor is selected when the search ends.

R-EDIT-093. Enter selects the next match. Shift+Enter selects the previous. Cmd+G and Cmd+Shift+G do the same with the bar closed. The search wraps at the end and shows "Wrapped" in the bar for 1 s.

R-EDIT-094. Every match on screen paints with the theme's match color. A scroll bar mark shows each match's position. The marks are limited to 10 000.

R-EDIT-095. "Replace" (Enter in the replace field) replaces the selected match and selects the next. "Replace all" (Cmd+Enter) replaces every match as one undo group. In regex mode `$1` to `$9` and `$0` in the replacement insert groups.

R-EDIT-096. With "in Selection" on, the search and the replace apply inside the Selection that existed when the toggle was set.

R-EDIT-097. An invalid regex shows the error under the query field in red and finds nothing.

## Navigation

R-EDIT-100. Ctrl+G opens the "Go to line" field at the top of the Leaf. Input `<line>` or `<line>:<col>`. Enter moves the cursor and centers it. A line beyond the end goes to the last line. Esc cancels and restores the scroll.

R-EDIT-101. Cmd+P opens Go to File over the window with the focused Bench's effective root folder. Enter opens the file in a Tab of the focused Bench. Cmd+Enter reveals it in the Explorer. Alt+Enter opens it in the external editor.

R-EDIT-102. `[later]` Cmd+Shift+O opens a symbol outline from chroma's name tokens.

R-EDIT-103. `[later]` LSP: completion, go to definition, diagnostics, through a language server started per root folder.

R-EDIT-104. Cmd+click on a path in the Buffer opens that file in a Tab. The path resolves against the File's folder, then the root folder. A `path:line` suffix scrolls to the line.

## Saving

R-EDIT-110. Cmd+S writes the Buffer to its path. Cmd+Alt+S writes every Buffer of the Bench with unsaved changes. A Buffer with no path opens the Save As sheet.

R-EDIT-111. Save writes to a temporary file in the same folder, then renames it over the File. The File's mode bits are kept. A write error shows a dialog with the error and keeps the Buffer unsaved.

R-EDIT-112. Shift+Cmd+S opens the Save As sheet (the system save panel) with the File's folder and name. The Tab's path changes to the new path after a successful write.

R-EDIT-113. The Bench setting `autosave` (off) saves a Buffer 1 s after its last edit. Autosave skips a Buffer that has no path, that has a conflict (R-EDIT-115), or whose last save failed.

R-EDIT-114. The editor polls the File's mtime and size every 1 s while the Tab is on screen, and once when the Tab comes on screen. When they changed and the Buffer has no unsaved changes, the editor reloads the File and keeps the cursor and the scroll. When the Buffer has unsaved changes, a bar at the top of the Leaf says "File changed on disk" with "Reload" and "Keep mine". "Reload" discards the Buffer's changes.

R-EDIT-115. Save checks the mtime read at load or at the last save. When the mtime differs, a dialog asks: "Overwrite", "Reload", "Cancel". "Overwrite" writes. "Reload" discards the Buffer. The check is skipped when the File did not exist at load.

R-EDIT-116. A File deleted on disk while open keeps its Buffer. The Tab title shows "(deleted)". Save recreates the File.

## Display

R-EDIT-120. The gutter shows line numbers right-aligned in the faint color. The current line's number is in the normal color. The gutter width fits the largest line number plus 2 characters. Clicking a line number selects the line. Dragging selects lines.

R-EDIT-121. The current line's background paints with the theme's current-line color when no Selection exists.

R-EDIT-122. Alt+Z toggles soft wrap for the Tab. Wrapped lines wrap at the Leaf width at word boundaries and show the continuation with the same indent plus one unit. Line numbers show once per logical line. The default comes from Settings `editor.wrap` (off), with `on` forced for markdown and plain text.

R-EDIT-123. Settings `editor.rulers` is a list of columns (default `[]`). Each paints as a 1 pt vertical line in the faint color.

R-EDIT-124. The editor allows scrolling until the last line is at the top of the Leaf.

R-EDIT-125. Scrolling by wheel and by keyboard animates over 80 ms. Page Up and Page Down move by the Leaf height minus 2 lines.

R-EDIT-126. The font is Settings `editor.font` (default the theme's mono family at 13 pt). Cmd+= and Cmd+- change the size for the Bench by 1 pt between 8 and 40. Cmd+0 resets.

R-EDIT-127. The editor paints only the lines on screen plus 20 above and below. Scrolling a 10 MiB file runs at 60 fps.

R-EDIT-128. `[later]` minimap.

R-EDIT-129. The editor supports the system IME: marked text paints underlined at the cursor; the Buffer changes only on commit.

## Explorer

R-EDIT-140. The Explorer is the editor Bench's Panel. Its root is the Bench's effective root folder. Changing the Bench's root folder re-roots the Explorer.

R-EDIT-141. The Explorer shows every entry of a folder, sorted folders first then names, case-insensitive. Entries that git ignores paint dimmed. Hidden entries (names that start with `.`) show when the toggle Cmd+Shift+. is on. The toggle is per Bench and is saved in the Bench's settings.

R-EDIT-142. The Explorer reads a folder when the user opens it, and reads it again when its mtime changes. It polls the mtime of every shown folder every 500 ms. The `.gitignore` state is read again on Cmd+R and when any `.gitignore` under the root changes.

R-EDIT-143. The Explorer remembers per root, in State `kv` under `explorer:<root>`: the open folders, the selected entry and the scroll position.

R-EDIT-144. A single click selects an entry. A single click on a file opens it in the focused Leaf as a transient Tab: the title paints in italics, and the next single click on another file replaces it. A double click, or an edit, makes the Tab permanent.

R-EDIT-145. Enter on a selected file opens it as a permanent Tab. Enter on a folder opens or closes it. Left closes the folder or goes to the parent. Right opens the folder. Space opens the file in a Preview `[later]`.

R-EDIT-146. Typing letters while the Explorer has focus jumps to the next entry whose name starts with the typed text. The text resets after 1 s.

R-EDIT-147. File operations from the context menu and the keys:

| Operation | Keys | Behaviour |
|---|---|---|
| new file | Cmd+N in the Panel | an inline name field in the selected folder; Enter creates and opens the file |
| new folder | Cmd+Shift+N in the Panel | an inline name field; Enter creates the folder |
| rename | F2 or Enter on a selected entry with the Alt key | an inline field with the name without its extension selected |
| delete | Cmd+Backspace | moves the entry to the Trash with the system call; shows a Toast "Moved <name> to Trash" with "Undo" for 5 s |
| duplicate | Cmd+D in the Panel | copies the entry next to itself as `<name> copy<.ext>` |
| copy path | Cmd+Alt+C | puts the absolute path on the clipboard |
| copy relative path | Cmd+Shift+Alt+C | puts the path relative to the root on the clipboard |
| reveal in Finder | — | `open -R <path>` |
| open in terminal | — | a new Session in the Lab's first terminal Bench with the folder as the current folder; creates the Bench when missing |
| open in external editor | — | runs Settings `editor.external` (R-EDIT-150) |
| refresh | Cmd+R | reads every open folder again |

R-EDIT-148. A name field refuses an empty name, a name with `/`, and a name that exists in the folder. The field shows the reason under it and stays open.

R-EDIT-149. `[later]` drag and drop to move entries inside the Explorer and from the Finder.

R-EDIT-150. Settings `editor.external` is a command template with `{path}`, `{line}` and `{col}`. Default `cursor --goto {path}:{line}:{col}`. The command runs with the user's login shell. A missing command shows a Toast with the shell's error.

R-EDIT-151. Cmd+Shift+F in the Panel shows a filter field above the tree. The tree shows only entries whose path under the root contains the filter text, case-insensitive, with their parent folders open. An empty field shows the full tree. Esc clears and closes the field.

R-EDIT-152. The Explorer shows at most 5 000 entries per folder. A larger folder shows the first 5 000 and a row "<n> more entries not shown".

## Preview

R-EDIT-160. A Preview is a read-only Tab. The editor opens a Preview for: images (PNG, JPEG, GIF, WebP, BMP, SVG), binary files (a NUL byte in the first 8 KB), files over the edit limit, files in an unsupported encoding.

R-EDIT-161. An image Preview fits the image in the Leaf and shows the size in pixels in the status bar. Wheel with Cmd zooms between 10 % and 800 %. A double click fits again. A GIF shows its first frame with a "GIF" badge; clicking it opens the lightbox, which plays it.

R-EDIT-162. A markdown File opens in the editor. Cmd+E switches the Tab between the editor and the rendered Preview. The Tab's type data keeps `preview`. The Preview renders with goldmark and GFM: headings, lists, task lists, tables, code blocks with highlighting, links, images under the root folder, block quotes, horizontal rules. Images outside the root and web images stay links.

R-EDIT-163. A Preview of a binary or oversized file shows the file name, the size, and the buttons "Open in external editor" and "Reveal in Finder".

R-EDIT-164. A Preview Tab has the same title rules as a File Tab. Its type data has `"preview": true` and it restores as a Preview.

## Git

R-EDIT-170. When the effective root folder is inside a git work tree, the Bench runs `git status --porcelain=v2 -z --untracked-files=all` in the root every 2 s while the Bench is on screen, and once when the Bench comes on screen or the window regains focus. The run has a 5 s timeout. A failed run keeps the last result and writes the error to the log once per Bench.

R-EDIT-171. The Explorer paints the name of each entry by its status:

| Status | Color | Letter after the name |
|---|---|---|
| modified | yellow | M |
| added (staged new) | green | A |
| untracked | green | U |
| deleted | red, struck through | D |
| renamed | yellow | R |
| conflicted | red | C |
| ignored | dimmed | — |

A folder shows the color of the "highest" status among its entries, in the order conflicted, modified, added, untracked. The Tab title shows the same color.

R-EDIT-172. The editor computes gutter marks for the Buffer against HEAD: it runs `git diff -U0 --no-color HEAD -- <path>` with the Buffer's text on stdin via `--no-index` against a temporary file, 500 ms after the last edit and after each status run. Added lines show a green bar in the gutter, modified lines a yellow bar, a deletion a red triangle between lines. Clicking a mark shows the removed lines in a popover.

R-EDIT-173. Cmd+Shift+G opens a Diff Tab in the Bench: the diff view (copied from herdr-go) of the Buffer against HEAD, side by side when the Leaf is 900 pt or wider, else unified. The Diff Tab's title is "Diff: <name>". Its type data is `{"diff": "<path>"}`. It updates when the Buffer changes.

R-EDIT-174. "Discard changes" in the Explorer's context menu, on a modified or deleted File, asks "Discard changes to <name>? This cannot be undone." and runs `git checkout -- <path>` on "Discard". An untracked File offers "Delete" to the Trash instead.

R-EDIT-175. Commits, branches, stash, push and pull are not in the editor. The user runs git in a terminal Tab. "Open in terminal" on the root folder is the shortcut for that.

## MCP tools

Tool domain `editor`. All tools need Access level Read, except where stated.

| Tool | Arguments | Result | Access |
|---|---|---|---|
| `editor.open` | `path`, optional `line`, `col`, `bench` | opens the file in a Tab of the given editor Bench, or of the focused one, or of the Lab's first editor Bench, and returns the Tab id | Full |
| `editor.reveal` | `path` | selects the file in the Explorer and opens its folders | Full |
| `editor.get_open_files` | optional `bench` | a list of `{tab, path, dirty, line, col}` | Read |
| `editor.get_selection` | optional `tab` | `{path, start: {line, col}, end: {line, col}, text}` of the focused editor Tab, or of the given one; text is cut at 64 KB | Read |
| `editor.goto` | `tab` or `path`, `line`, optional `col` | moves the cursor and centers the line | Full |

R-EDIT-180. No MCP tool writes a Buffer. An agent writes the file on disk. The editor reloads it by R-EDIT-114.

R-EDIT-181. `editor.open` with a path outside every root folder of the Lab is allowed; the Tab's title then shows the full path.
