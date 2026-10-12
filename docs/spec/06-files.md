# Files Bench

> M3 status (2026-10-12). Copies and moves run `ditto` (modes, times and extended attributes kept). The clipboard holds file URLs, so Finder copies paste here and back. `[later]` in M3: system file icons (R-FILES-022), the progress bar for large copies (R-FILES-043), Undo of a move to the Trash, inline name fields (a sheet asks instead).


The files Bench is a file manager. A Tab is one Folder view. The Panel lists favorites and recent folders. The Bench reuses the Explorer's file operations (`05-editor.md`, R-EDIT-147) and adds copy, paste, move, compress and "open with".

## Tab

R-FILES-001. A files Tab holds one Folder view. Its type data is `{"path": "<absolute folder path>", "sort": {"column": "name", "desc": false}, "selected": ["<name>", ...], "scroll": n}`.

R-FILES-002. "New Tab" (Cmd+T) opens the Bench's effective root folder.

R-FILES-003. At restore, a Tab opens its `path`. A missing folder opens the nearest existing parent and shows a Toast "Folder not found: <path>".

R-FILES-004. The Tab title is the folder's name. The root of a volume is titled by the volume's name. The home folder is titled "Home".

R-FILES-005. Going to a folder changes the Tab's `path` in place. The Tab keeps a back and forward list of up to 50 folders. Cmd+[ and Cmd+] walk it. The list is not saved in State.

## Path bar

R-FILES-010. The path bar at the top of the Leaf shows one segment per folder from the volume root to the current folder. A click on a segment goes to that folder. A long path cuts segments from the left, keeps the last 3, and shows "…" at the cut.

R-FILES-011. Cmd+Shift+G turns the path bar into a text field with the current path. Enter goes to the typed path. `~` expands to the home folder. A relative path resolves against the current folder. A path that is not a folder shows the error "Not a folder" under the field. Esc restores the bar. Tab completes the next folder name.

R-FILES-012. Cmd+Up goes to the parent folder and selects the folder the Tab came from. Cmd+Down opens the selected entry.

## List

R-FILES-020. The list has the columns name, size, kind, modified. A click on a column header sorts by it; a second click reverses. The sort is saved in the Tab's type data and the default for new Tabs is saved in the Bench's settings.

R-FILES-021. Sorting rules: folders come first in every sort. Names sort case-insensitive with numbers in natural order (`file2` before `file10`). Size sorts folders as 0 unless computed (R-FILES-024). Kind sorts by extension then name.

R-FILES-022. The name column shows the system icon of the entry (16 pt), from the Launch Services icon for the file's type.

R-FILES-023. The size column shows bytes in units: `B`, `KB`, `MB`, `GB` with one decimal above 10 KB. A folder shows "—".

R-FILES-024. "Calculate size" in a folder's context menu walks the folder off the main thread and shows the total in the size column for that Tab. The walk stops after 30 s and shows "> <size>" when it stopped. A folder's size is not stored.

R-FILES-025. The modified column shows "Today 14:05", "Yesterday 09:12", or `2026-10-12`.

R-FILES-026. Hidden entries show when Cmd+Shift+. is on. The toggle is per Bench.

R-FILES-027. The list reads the folder off the main thread and polls its mtime every 500 ms while the Tab is on screen. A change reads the folder again and keeps the selection by name.

R-FILES-028. The list shows at most 10 000 entries. A larger folder shows the first 10 000 by the sort and a row "<n> more entries not shown". The row offers "Show all", which lifts the limit for that Tab.

R-FILES-029. The status bar at the bottom shows "<n> items" and, with a selection, "<k> of <n> selected, <size>". The size sums files only.

## Selection and keyboard

R-FILES-030. A click selects one entry. Cmd+click adds or removes one. Shift+click extends to a range. Cmd+A selects all. A click on empty space clears the selection. A drag on empty space selects a rectangle `[later]`.

R-FILES-031. Keys:

| Keys | Effect |
|---|---|
| Up, Down | move the selection |
| Shift+Up, Shift+Down | extend the selection |
| Enter | open (R-FILES-032) |
| Space | Quick Look (R-FILES-033) |
| Cmd+Up | parent folder |
| Cmd+Down | open |
| Cmd+O | open with the default app |
| letters | jump to the next entry that starts with the text; the text resets after 1 s |
| Home, End | first, last entry |

R-FILES-032. Enter on a folder goes into it. Enter on a file opens it in the Lab's first editor Bench as a Tab, and creates an editor Bench named "editor" when the Lab has none. Enter on a file the editor cannot open (R-EDIT-160) opens a Preview Tab there.

R-FILES-033. Space runs `qlmanage -p <paths>` with the selected files. A second Space, or Esc in the Quick Look window, closes it. The process is killed when the Tab closes.

## Operations

R-FILES-040. The context menu and the keys:

| Operation | Keys | Behaviour |
|---|---|---|
| new folder | Cmd+Shift+N | an inline name field; Enter creates |
| new file | Cmd+N | an inline name field; Enter creates an empty file |
| open | Enter | R-FILES-032 |
| open with default app | Cmd+O | `open <path>` |
| open with… | — | the system "open with" list from Launch Services; `open -b <bundle id> <path>` |
| rename | Enter with Alt, or F2 | an inline field, name without extension selected |
| copy | Cmd+C | puts the selected paths on the clipboard as file URLs |
| cut | Cmd+X | as copy, marks the entries for a move; the entries paint dimmed until pasted or the mark is dropped |
| paste | Cmd+V | copies the clipboard's files into the current folder; after a cut, moves them |
| move here | Cmd+Alt+V | moves the clipboard's files into the current folder |
| duplicate | Cmd+D | copies next to the entry as `<name> copy<.ext>`; a second copy is `<name> copy 2<.ext>` |
| move to Trash | Cmd+Backspace | the system Trash; Toast "Moved <n> items to Trash" with "Undo" for 5 s |
| delete permanently | Cmd+Alt+Backspace | asks "Delete <n> items permanently? This cannot be undone."; deletes on "Delete" |
| compress | — | `ditto -c -k --sequesterRsrc --keepParent <path> <name>.zip` next to the entry; several entries go in `Archive.zip` |
| copy path | Cmd+Alt+C | absolute paths, one per line |
| reveal in Finder | Cmd+Alt+R | `open -R <path>` |
| open in terminal | — | a new Session in the Lab's first terminal Bench with the folder as the current folder |
| get info | Cmd+I | a popover with name, kind, size, created, modified, permissions in `rwx` form, owner, and the resolved target for a symlink |
| add to favorites | — | adds the folder to the Panel's favorites |

R-FILES-041. A copy or move that finds the same name in the target asks: "Replace", "Keep both", "Skip", "Cancel", with "Apply to all" for more than one conflict. "Keep both" names the new entry `<name> 2<.ext>`.

R-FILES-042. A copy or move of a folder into itself or into one of its own folders is refused with the message "Cannot move a folder into itself".

R-FILES-043. A copy, move or delete of more than 100 entries, or of more than 100 MB, shows a progress bar in the status bar with "Cancel". Cancel stops after the current entry and keeps what was done.

R-FILES-044. A copy keeps the mode bits, the modification time and the extended attributes. A move inside one volume renames. A move across volumes copies then deletes, and deletes only after the whole copy succeeded.

R-FILES-045. The Trash operation uses the system call that the Finder uses, so the Finder's "Put Back" works.

R-FILES-046. A paste from the Finder's clipboard works. A copy in the files Bench pastes in the Finder.

R-FILES-047. `[later]` drag and drop inside the list, between Tabs, to and from the Finder, and onto a terminal Tab to type the path.

R-FILES-048. Symlinks show with an arrow badge. Enter on a symlink to a folder goes to the target. The path bar shows the symlink's path, not the target's.

## Panel

R-FILES-050. The Panel has two sections: Favorites and Recent.

R-FILES-051. Favorites is a list of folders the user added. The default list is Home, Desktop, Documents, Downloads and the Lab's root folder. A click goes to the folder in the focused Tab. Cmd+click opens a new Tab. The context menu removes a favorite. Reorder by drag `[later]`. The list is saved in the Bench's settings.

R-FILES-052. Recent is the last 10 folders the Bench's Tabs went to, newest first, without the current folder. The list is saved in the Bench's settings.

R-FILES-053. A favorite whose folder is missing paints dimmed and shows "Folder not found" as a tooltip.

## MCP tools

R-FILES-060. `[later]` The tool domain `files` has no tools in v1. An agent reads and changes files on disk with its own tools and calls `editor.reveal` to show one to the user.
