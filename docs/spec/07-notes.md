# Notes Bench

The notes Bench keeps markdown Notes. A Tab is one Note. The Panel lists the Notes with search. The Note editor is the editor Bench's native editor in markdown mode (`05-editor.md`), with the Preview on Cmd+E.

> M10 status (2026-10-12). Done: the Lab and Global folders, the index with polling and search, the Panel with groups and its menu, new Notes, titles from `# `, autosave, the Preview through the editor Bench, image paste, the MCP tools. The notes Bench is the editor Bench with its own Panel, new Tab and title (package `internal/bench/notes`). `[later]` in M10: soft wrap (R-NOTES-014: the native editor does not wrap yet), the dimmed frontmatter (R-NOTES-016), `renameOnTitle` (R-NOTES-013), Undo of a delete (R-NOTES-018: the file goes to the Trash), Cmd+Shift+F to the search field (R-NOTES-033). Rename asks in a sheet, not inline (R-NOTES-038). An image pastes when the clipboard holds a PNG (R-NOTES-020).

## Storage

R-NOTES-001. The Bench's notes folder is `$NOTCH_LAB_HOME/notes/<lab id>/` by default. The Bench setting `folder` replaces it with any folder. The user can point it at an Obsidian vault folder.

R-NOTES-002. The app creates the default folder when the Bench first opens. A setting that names a missing folder shows the Panel message "Folder not found: <path>" with "Choose folder…" and "Use default".

R-NOTES-003. A Note is one file with the extension `.md` directly in the notes folder or in a folder below it. Other files are ignored. Folders named `.obsidian`, `.git`, `assets` and folders that start with `.` are not read.

R-NOTES-004. A second section, "Global", shows `$NOTCH_LAB_HOME/notes/global/`. The same rules apply. A Note in Global is the same file from every Lab.

R-NOTES-005. The Bench reads the folder when the Bench opens and polls the mtime of the folder and of each folder below it every 1 s while the Bench is on screen. A change reads that folder again.

R-NOTES-006. The Bench keeps the text of every Note in memory for search, up to 1 MB per Note and 200 MB in total. A Note above the limit is searched by title only. The text is read again when the file's mtime changes.

## Tab

R-NOTES-010. A notes Tab holds one Note. Its type data is `{"path": "<absolute path>", "preview": bool, "scroll": n, "line": n, "col": n}`.

R-NOTES-011. "New Tab" (Cmd+T) creates `<yyyy-mm-dd-hhmmss>.md` in the notes folder with the content `# ` and the cursor after it, and opens it. In the Global section, Cmd+T creates the Note there.

R-NOTES-012. The Note's title is the text of the first line that starts with `# `, else the file name without `.md`. The title is the Tab title and the Panel row.

R-NOTES-013. The Bench setting `renameOnTitle` (off) renames the file to the title when the Note is saved and the title changed. The file name is the title with characters other than letters, digits, `-`, `_` and space replaced by `-`, cut at 80 characters. A name that exists gets ` 2`, ` 3` and so on. Open Tabs follow the rename.

R-NOTES-014. The editor opens a Note with the markdown lexer, soft wrap on, and autosave on with a 1 s delay. The Bench setting `autosave` (on) turns autosave off.

R-NOTES-015. Cmd+E switches the Tab between the editor and the Preview. The Preview renders as in R-EDIT-162, with relative images resolved against the Note's folder and the notes folder.

R-NOTES-016. YAML frontmatter between `---` lines at the start of a Note is shown as text in the editor, with a dimmed background. The Preview does not render it. The editor does not change it except by the user's edits.

R-NOTES-017. At restore, a Tab reads its path. A missing Note shows the placeholder "Note not found" with "Close".

R-NOTES-018. Deleting a Note from the Panel moves the file to the Trash and closes its Tabs. A Toast with "Undo" shows for 5 s.

## Images

R-NOTES-020. Pasting an image from the clipboard into a Note writes it to `<notes folder>/assets/<yyyy-mm-dd-hhmmss>.png` and inserts `![](assets/<name>.png)` at the cursor. The folder is created when missing. A Note in a folder below the notes folder uses a relative path from the Note's folder.

R-NOTES-026. A Note shows without line numbers, whatever the editor setting. The status bar shows "Note · <file name>", not the full path.

R-NOTES-021. Dropping an image file on a Note copies it to the assets folder in the same way `[later]`.

## Panel

R-NOTES-030. The Panel has a search field, then the Lab section, then the Global section. Each section lists its Notes sorted by modified time, newest first. A row shows the title, the modified time in short form, and the first line of body text that is not a heading, cut at 80 characters.

R-NOTES-031. A click opens the Note in the focused Leaf. Cmd+click opens it in a new Split right.

R-NOTES-032. Notes in a folder below the notes folder show under a group row with the folder's name. The group opens and closes with a click. The state is saved in the Bench's settings.

R-NOTES-033. The search field (Cmd+Shift+F in the Panel) filters both sections to Notes whose title or text contains every word of the query, case-insensitive. The search runs 100 ms after the last keystroke, over the in-memory text, and finishes within 200 ms for 1 000 Notes of 10 KB on an M1. A row shows the first matching line under the title.

R-NOTES-034. Enter in the search field opens the first result. Esc clears the field.

R-NOTES-035. `[later]` tags from `#tag` words and frontmatter `tags:` shown as filter chips under the search field.

R-NOTES-036. `[later]` wikilinks `[[title]]`: Cmd+click opens the Note; the editor completes titles after `[[`.

R-NOTES-037. `[later]` daily note: Cmd+Shift+D opens or creates `daily/<yyyy-mm-dd>.md`.

R-NOTES-038. The Panel's context menu on a Note: open in new Split, rename (inline), move to Global or to the Lab, reveal in Finder, copy path, delete.

## MCP tools

Tool domain `notes`.

| Tool | Arguments | Result | Access |
|---|---|---|---|
| `notes.list` | optional `lab`, `scope` (`lab`, `global`, `all`; default `all`) | a list of `{path, title, modified, size}` | Read |
| `notes.read` | `path` | `{path, title, text}`; text cut at 1 MB | Read |
| `notes.search` | `query`, optional `lab`, `scope`, `limit` (default 20) | a list of `{path, title, line, text}` with the first matching line | Read |
| `notes.write` | `path` or `title`, `text` | writes the whole file; with `title` and no `path`, creates `<title as in R-NOTES-013>.md` in the Lab's folder; returns `{path}` | Full |
| `notes.append` | `path`, `text` | adds a line break when the file does not end with one, then the text; returns `{path}` | Full |

R-NOTES-040. A path given to a tool must be inside the Lab's notes folder or the Global folder. Any other path is refused with the error `path outside notes`.

R-NOTES-041. `notes.write` on a Note that is open in a Tab with unsaved changes is refused with the error `note has unsaved changes`. The agent can call `notes.read` and try again later.

R-NOTES-042. A write by a tool shows in an open Tab within 1 s by the editor's reload (R-EDIT-114).
