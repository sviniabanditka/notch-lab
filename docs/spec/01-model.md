# Model: Lab, Bench, Tab, Layout, State

## Lab

R-MODEL-001. A Lab has an id (UUID), a name, an optional root folder, a color and an order index.

R-MODEL-002. The app has at least one Lab. Deleting the last Lab creates a new empty Lab named "Lab".

R-MODEL-003. The user selects one Lab at a time. The selection is per window. The notch panel and the main window can show different Labs.

R-MODEL-004. A Lab's root folder, when set, must be a folder that exists. The app shows a warning icon in the Lab menu when the folder is missing.

R-MODEL-005. Deleting a Lab deletes its Benches, Tabs and Layouts from State. The app asks for confirmation. Sessions of the Lab's terminal Benches are closed in the Daemon.

## Bench

R-MODEL-010. A Bench has an id, a Lab id, a type, a name, an optional root folder, an icon, an order index and a Layout.

R-MODEL-011. A Bench type is one of: `terminal`, `browser`, `editor`, `files`, `notes`, `http`, `db`, `ssh`, `pull-requests`, `agents`. The set is fixed in code.

R-MODEL-012. A Lab can hold several Benches of the same type.

R-MODEL-013. A Bench's effective root folder is its own root folder when set, else its Lab's root folder, else the user's home folder.

R-MODEL-014. The user creates a Bench from the rail's "+" button or from the Switcher. The app asks for the type and the name. The default name is the type's name.

R-MODEL-015. The user reorders Benches in the rail by drag. The user renames a Bench from its context menu.

R-MODEL-016. Deleting a Bench deletes its Tabs and Layout. The app asks for confirmation when the Bench has one or more Tabs.

R-MODEL-017. Each Bench type has settings of its own, stored as JSON in the Bench row. The type's document lists them.

## Tab

R-MODEL-020. A Tab has an id, a Bench id, a title, an order index, a pinned flag, and type data as JSON.

R-MODEL-021. A Tab belongs to one Bench. A Tab does not move to another Bench.

R-MODEL-022. The user reorders Tabs in the sidebar by drag. Pinned Tabs stay at the top of the list in their own order.

R-MODEL-023. Closing a Tab removes it from the Layout and from the sidebar. The app keeps the last 20 closed Tabs per Bench. Cmd+Shift+T reopens the last closed Tab of the selected Bench with its type data.

R-MODEL-024. Each Bench type defines what "new Tab" (Cmd+T) creates. The type's document states it.

R-MODEL-025. A Tab has a title the type supplies. The user can set a custom title from the context menu. A custom title stays until the user clears it.

R-MODEL-026. A Tab can be Unread. The sidebar shows a dot on an Unread Tab. The rail shows a dot on a Bench with an Unread Tab. The Lab menu shows a dot on a Lab with an Unread Tab.

## Layout

R-MODEL-030. A Layout is a tree. A node is a Split or a Leaf. A Split has a direction (`row` or `column`), a ratio and two children. A Leaf has a Tab id or is empty.

R-MODEL-031. A Bench with no Tabs has a Layout of one empty Leaf. An empty Leaf shows the Bench type's empty view with its "new Tab" action.

R-MODEL-032. A Tab appears in at most one Leaf. A Tab can be in no Leaf: the Tab is then "in the sidebar only".

R-MODEL-033. Selecting a Tab in the sidebar shows the Tab in the focused Leaf. The Tab that was in the focused Leaf becomes "in the sidebar only".

R-MODEL-034. Cmd+D splits the focused Leaf in a row: the focused Tab stays left, a new Tab opens right. Cmd+Shift+D splits in a column: the new Tab opens below. The ratio is 0.5.

R-MODEL-035. Alt+click on a Tab in the sidebar opens the Tab in a new Split right of the focused Leaf.

R-MODEL-036. The ratio of a Split is between 0.1 and 0.9. A Leaf is never narrower than 120 pt or shorter than 80 pt. A drag that would break the limit stops at the limit.

R-MODEL-037. Closing the Tab in a Leaf removes the Leaf. The Leaf's sibling takes the parent's place. The Leaf that was nearest in reading order receives Focus.

R-MODEL-038. Cmd+Alt+Left, Right, Up, Down move Focus to the neighbouring Leaf in that direction. Focus moves to the Leaf that overlaps most along the edge.

R-MODEL-039. Cmd+Shift+Enter zooms the focused Leaf: the Layout shows only that Leaf. The same shortcut, or any Focus change, unzooms.

R-MODEL-040. The user drags a Divider to change the ratio. The Divider is 1 pt wide and has an 8 pt hit area.

R-MODEL-041. A Layout holds Tabs of its own Bench only.

R-MODEL-042. The Layout is saved in State as JSON: `{"split":{"dir":"row","ratio":0.5,"a":...,"b":...}}` or `{"leaf":{"tab":"<id>"}}` or `{"leaf":{}}`. The focused Leaf carries `"focus":true`. At load, a Leaf whose Tab is missing or shown twice goes away, as when its Tab closes.

## State

R-MODEL-050. State lives in one SQLite database at `~/Library/Application Support/notch-lab/state.db`. The driver is `modernc.org/sqlite`.

R-MODEL-051. The schema:

```sql
CREATE TABLE lab   (id TEXT PRIMARY KEY, name TEXT NOT NULL, root TEXT, color TEXT, ord INTEGER NOT NULL);
CREATE TABLE bench (id TEXT PRIMARY KEY, lab TEXT NOT NULL REFERENCES lab(id) ON DELETE CASCADE,
                    type TEXT NOT NULL, name TEXT NOT NULL, root TEXT, icon TEXT, ord INTEGER NOT NULL,
                    layout TEXT NOT NULL, settings TEXT NOT NULL DEFAULT '{}');
CREATE TABLE tab   (id TEXT PRIMARY KEY, bench TEXT NOT NULL REFERENCES bench(id) ON DELETE CASCADE,
                    title TEXT, custom_title TEXT, ord INTEGER NOT NULL, pinned INTEGER NOT NULL DEFAULT 0,
                    unread INTEGER NOT NULL DEFAULT 0, data TEXT NOT NULL DEFAULT '{}');
CREATE TABLE closed_tab (bench TEXT NOT NULL, closed_at INTEGER NOT NULL, tab TEXT NOT NULL);
CREATE TABLE window (key TEXT PRIMARY KEY, lab TEXT, bench TEXT, focused_tab TEXT, frame TEXT, sidebar INTEGER);
CREATE TABLE kv    (key TEXT PRIMARY KEY, value TEXT NOT NULL);
PRAGMA user_version = 1;
```

R-MODEL-052. The app writes State at most once per 100 ms. Several changes inside that time go in one transaction. In v1 the transaction rewrites every row; the State is small.

R-MODEL-053. The app writes State before it quits. A write that fails shows a Toast and writes to the log.

R-MODEL-054. At start the app reads State and restores: every Lab, Bench and Tab, each Layout, the selected Lab and Bench per window, the window frame, the sidebar state. Each Bench type restores its Tabs' type data. The type's document states what a restored Tab does.

R-MODEL-055. A schema change increments `user_version`. The app migrates forward in order. The app refuses to open a database with a `user_version` newer than it knows and shows the message "This state was written by a newer notch-lab".

R-MODEL-056. The app writes a copy of `state.db` to `state.db.bak` once per day at the first write of the day.

R-MODEL-057. The `tools/dev` script sets `NOTCH_LAB_HOME` to a folder of its own. The app uses `$NOTCH_LAB_HOME` in place of `~/Library/Application Support/notch-lab` when the variable is set. The Daemon and the Bridge do the same.

## Identity

R-MODEL-060. Every id is a UUID v4 in lower-case text.

R-MODEL-061. Titles and names are at most 200 characters. The app cuts longer text and shows an ellipsis in the middle.

## Events

R-MODEL-070. `[later: M6, with the MCP server]` `internal/model` emits events on every change: `LabChanged`, `BenchChanged`, `TabChanged`, `LayoutChanged`, `SelectionChanged`. The Shell, the Switcher, Unread and the MCP server subscribe. Events carry ids, not objects.
