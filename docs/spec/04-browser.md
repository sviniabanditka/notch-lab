# Browser Bench

The browser Bench shows web Pages in WebKit views inside the Layout. It loads Chrome and Firefox Extensions through `WKWebExtension` (macOS 15.4). The Extension layer, the Store install and the Probe are a port of Search by Office Commun (MIT). The app keeps Search's copyright line in `NOTICE`. The Shim JavaScript is reused as is.

Requirements are numbered `R-WEB-nnn`.

> M4 status (2026-10-12). Done: Tabs, Profiles, the Probe, default browser (stage A); the Extension controller and adapters, Store install with the "Add to notch-lab" button, CRX3 check, the Shim, the native answers for bookmarks, history, downloads, userScripts and scripting, action buttons with a popup, the options page, Settings › Extensions, daily updates (stage B). `tools/extcheck` checks the bridge against real WebKit and installs uBlock Origin Lite from the Store. `[later]` in M4: AMO installs and `.xpi`/`.zip` files (R-WEB-146, R-WEB-148), the second permission sheet (R-WEB-142), side panels, offscreen documents, `identity`, `webRequest`, `tabGroups` answers (R-WEB-151), capture (R-WEB-153), popup sizing to its content and the 300 ms reopen guard (R-WEB-161), the "more" menu for unpinned Extensions and the button's context menu (R-WEB-160, R-WEB-163), remembered grants (R-WEB-170), the permission diff on update (R-WEB-171), removal of an Extension's WebKit data (R-WEB-181), the notch panel as an Extension window (R-WEB-132), the 2 s wait of early Pages (R-WEB-134).


## Tabs and Web views

R-WEB-001. A browser Tab holds exactly one Page. The Tab's type data is `{"url": "<string>", "title": "<string>", "favicon": "<path or empty>", "zoom": <number>, "scroll": <number>}`.

R-WEB-002. Cmd+T in a browser Bench creates a Tab with the new tab page and puts keyboard focus in the address field. The new tab page shows the 8 most visited sites from history and the bookmarks bar.

R-WEB-003. A Tab restored from State does not create a Web view until a Leaf shows the Tab. The Tab is "asleep": its sidebar row shows the saved title and favicon. The first show loads the saved URL.

R-WEB-004. The app creates one Web view per awake Tab, once, on the window, through the mygo fork: `window.NewWebView(WebViewOptions{DataStoreID: <Profile id>, ExtensionController: <shared>, Inspectable: <Settings "browser.inspectable">})`. The Leaf shows the Web view with `ui.WebView`. A Leaf that stops showing the Tab hides the Web view and keeps its Page.

R-WEB-005. The Tab title is the Page's title. When the Page has no title the title is the host. When the Page has no host the title is "New Tab". A custom title (R-MODEL-025) wins.

R-WEB-006. The sidebar shows the Page's favicon at 16 pt. The app caches favicons in `$NOTCH_LAB_HOME/favicons/<sha256 of url>.png`. A Page with no favicon shows a globe icon.

R-WEB-007. The sidebar row shows a loading indicator while the Page loads, and a speaker icon while the Page plays audio.

R-WEB-008. A pinned Tab keeps its Web view awake. The app never puts a pinned Tab to sleep.

R-WEB-009. At most 12 Tabs per Bench are awake. When a 13th Tab wakes, the app puts to sleep the hidden Tab that was shown longest ago, after it saves the Tab's URL and scroll position. A Tab hidden for 30 minutes goes to sleep. A Tab that plays audio or holds a download does not go to sleep.

R-WEB-010. Cmd+W closes the focused Tab. When the Page has a `beforeunload` handler that asks, the app shows a sheet "Leave this page?" with Leave and Stay.

R-WEB-011. Cmd+Shift+T reopens the last closed Tab with its URL (R-MODEL-023).

## Address bar

R-WEB-020. Each browser Leaf has an address bar of 36 pt height at its top. From left to right: Back, Forward, Reload or Stop, the lock icon, the address field, the zoom indicator, the Extension buttons, the "more" menu.

R-WEB-021. Cmd+L puts keyboard focus in the address field and selects all text. Esc restores the Page's URL in the field and returns focus to the Page.

R-WEB-022. While the user types, the field shows at most 8 suggestions below it: first bookmarks, then history, both matched on the URL and the title, case-insensitive, ordered by visit count. Up and Down select a suggestion. Enter opens the selection, or the typed text when nothing is selected.

R-WEB-023. The lock icon shows closed for `https` with a valid certificate, open for `http`, and a warning sign for a certificate the system refused. A click on the icon shows the host and the certificate's issuer in a popover.

R-WEB-024. Reload is Cmd+R. Cmd+Shift+R reloads without the cache. While the Page loads, the button is Stop and Esc stops.

R-WEB-025. A long press of 400 ms on Back or Forward shows the list of the last 12 entries in that direction. A click on an entry goes to it.

R-WEB-026. The zoom indicator shows when the zoom is not 100 %. A click resets to 100 %.

R-WEB-027. The address field shows the URL without `https://` and without a trailing `/` on the root path. The field shows the full URL while the user edits.

## Navigation rules

R-WEB-030. The app reads typed text as follows:

| Text | Action |
|---|---|
| has a scheme the app allows (`http`, `https`, `file`, `about`) | load as URL |
| has no spaces and has a dot or is `localhost` with an optional port | load as `https://<text>`; when that fails with a connection error, retry once as `http://<text>` |
| anything else | search with the search engine |

R-WEB-031. The search engine is a Settings value `browser.search` with the choices Google, DuckDuckGo, Bing, Kagi and Custom. Custom takes a URL with `%s`. The default is DuckDuckGo.

R-WEB-032. A `file://` URL loads only when its path is under the user's home folder. Another path shows the message "notch-lab opens files under your home folder only".

R-WEB-033. A `javascript:` URL never loads. The field shows the message "JavaScript addresses are not allowed".

R-WEB-034. A link with `target=_blank` or a `window.open` call opens a new Tab in the same Bench, right after the current Tab in the sidebar. The new Tab shows in the focused Leaf when the user clicked with the pointer. The new Tab stays in the sidebar only when a script opened it without a click, or when the user held Cmd.

R-WEB-035. A `window.open` call with no user gesture in the last 1 s is blocked. The address bar shows a "popup blocked" icon. A click on the icon opens the blocked URL.

R-WEB-036. Cmd+click on a link opens it in a new Tab in the sidebar only. Cmd+Shift+click opens it in the focused Leaf. Alt+click downloads it.

R-WEB-037. A navigation to a scheme another app handles (`mailto`, `tel`, `vscode`, …) asks "Open in <app name>?" with Open and Cancel. "Always for this site" remembers the choice per origin under `browser.schemes`.

## Downloads

R-WEB-040. A download goes to `~/Downloads`. When a file with that name exists, the app adds ` (2)`, ` (3)`, … before the extension.

R-WEB-041. The app shows a Toast "Downloading <name>" at the start and "Downloaded <name>" at the end. A click on the Toast reveals the file in Finder.

R-WEB-042. The Library has a Downloads section: name, size, progress, state (running, done, failed). Each row has "Show in Finder", "Open" and "Remove from list". "Clear" removes done rows. The app keeps the last 200 downloads in `downloads.json` in `$NOTCH_LAB_HOME`.

R-WEB-043. A download that the app cannot finish shows "failed" with the reason. "Retry" starts it again.

R-WEB-044. A download from an isolated Profile uses that Profile's cookies.

## Permissions

R-WEB-050. A Page's request for camera, microphone, location, notifications or screen capture shows a sheet on the window: the origin, the kind, Allow and Deny. The sheet never auto-allows.

R-WEB-051. The choice is remembered per origin and kind in `settings.json` under `browser.permissions`: `{"<origin>": {"camera": "allow"|"deny"}}`. Settings › Browser lists them with a Remove button.

R-WEB-052. A screen capture request from an Extension goes through the Extension's own consent (R-WEB-153), never through WebKit's dialog.

## Profiles

R-WEB-060. The shared Profile is `WKWebsiteDataStore(forIdentifier: 4e4f5443-484c-4142-0000-000000000001)`. Every browser Bench uses it unless its setting `isolated` is on.

R-WEB-061. A browser Bench with `isolated` on uses the Profile `WKWebsiteDataStore(forIdentifier: <Bench id>)`. The Bench's Tabs share that Profile. Nothing else uses it.

R-WEB-062. Turning `isolated` on or off reloads every awake Tab of the Bench in the new Profile. The app asks for confirmation: "Cookies and sign-ins of this Bench change. Continue?".

R-WEB-063. Deleting an isolated Bench removes its data store: the app calls `removeData` for all types, then `WKWebsiteDataStore.remove(forIdentifier:)`. When WebKit refuses because the store is in use, the app writes the id under `browser.erasing` in `settings.json` and tries again at the next start, as Search does.

R-WEB-064. Settings › Browser › Profiles lists the shared Profile and every isolated Profile with the Bench's name, the size on disk and a "Clear data" button. "Clear data" removes all website data of that Profile after confirmation.

R-WEB-065. The Extension controller is one per app. WebKit keeps an Extension's storage (`chrome.storage`, IndexedDB of the Extension's pages) in the controller's configuration data store, not per Profile. Consequence: Extension data is per app, not per Profile. The Profiles section states this limit: "Extensions keep their own data across all Profiles".

R-WEB-066. A private Tab `[later]`.

## Bookmarks

R-WEB-070. Cmd+Shift+B bookmarks the focused Page: a popover with the title, the folder and Done. Cmd+Shift+B on a bookmarked Page opens the same popover with Remove.

R-WEB-071. Bookmarks live in `$NOTCH_LAB_HOME/bookmarks.json`: a tree of folders and bookmarks, each with id, title, url, created time. The file is written at most once per 500 ms.

R-WEB-072. The Library's Bookmarks section shows the tree. Folders open and close. A click opens the bookmark in the focused Leaf. Cmd+click opens it in a new Tab. Drag reorders and moves between folders. The context menu has Open, Open in new Tab, Rename, Delete, New folder.

R-WEB-073. The bookmarks bar shows the folder named "Bar" under the address bar when the Settings value `browser.bookmarksBar` is on. Default off.

R-WEB-074. Import from Chrome, Arc and Safari `[later]`.

R-WEB-075. The browser Bench has no Panel: its sidebar is the Tab list. The Library button in the address bar opens the Library, a sheet over the window: Bookmarks, History, Downloads and Extensions (the controls of Settings › Extensions), and the "Isolated profile" switch (R-WEB-062). Opening an item closes the sheet; Esc and a click outside close it.

## History

R-WEB-080. The app writes every top-level navigation to the state database:

```sql
CREATE TABLE browser_history (url TEXT PRIMARY KEY, title TEXT, visited_at INTEGER NOT NULL, count INTEGER NOT NULL DEFAULT 1, profile TEXT NOT NULL);
CREATE INDEX browser_history_visited ON browser_history(visited_at);
```

A new visit of a known URL updates `title`, `visited_at` and `count`.

R-WEB-081. The app does not write a visit from an isolated Profile when the Bench's setting `history` is off. Default on.

R-WEB-082. The Library's History section lists visits grouped by day, newest first, with a search field that matches URL and title. A click opens the visit. The context menu has Open in new Tab, Copy link, Remove.

R-WEB-083. Settings › Browser has "Clear history" with the choices last hour, today, all. The app also removes the favicon cache of removed URLs.

R-WEB-084. The Switcher (`02-shell.md`) searches history by URL and title and opens a result in a new Tab of the selected Lab's first browser Bench.

R-WEB-085. The app keeps at most 100 000 rows. When the table is larger, the app removes the oldest rows at start.

## Find, zoom, print, media

R-WEB-090. Cmd+F shows a find bar at the top of the Leaf under the address bar: a field, the count "n of m", Previous, Next, Done. Matching is case-insensitive. Enter goes to the next match, Shift+Enter to the previous. Esc closes. Matches are highlighted through WebKit's find API.

R-WEB-091. Cmd+Plus, Cmd+Minus and Cmd+0 zoom in 10 % steps between 50 % and 300 %. Zoom is remembered per origin in `settings.json` under `browser.zoom`.

R-WEB-092. Cmd+P is Go to File. Cmd+Alt+P prints the Page with the system print sheet.

R-WEB-093. Media plays without a user gesture only when muted. The Settings value `browser.autoplay` turns this off per origin list.

R-WEB-094. Picture in picture `[later]`. Reader mode `[later]`.

## Context menu

R-WEB-100. The context menu on a Page has, when they apply: Back, Forward, Reload, Open link in new Tab, Copy link, Copy link text, Save image as…, Copy image, Copy image address, Search "<selection>", Inspect element, Ask agent about selection `[later]`.

R-WEB-101. "Inspect element" shows only when the Web view is Inspectable. It opens Safari's Web Inspector through `WKWebView.inspector` `[fork]`.

## Keyboard

R-WEB-110. In a browser Leaf, Cmd+[ goes back in the Page's own history when the Page has an entry behind it. When the Page has none, Cmd+[ goes back in the Shell's History. Cmd+] does the same forward. Cmd+Shift+[ and Cmd+Shift+] select the previous and next Tab in the sidebar.

R-WEB-111. Cmd+Left and Cmd+Right stay with the Page: text fields use them.

R-WEB-112. Space and Shift+Space scroll the Page when no text field has focus.

| Shortcut | Action |
|---|---|
| Cmd+T | new Tab |
| Cmd+L | focus the address field |
| Cmd+R / Cmd+Shift+R | reload / reload without cache |
| Cmd+F | find |
| Cmd+Shift+B | bookmark |
| Cmd+Alt+J | Console drawer |
| Cmd+Alt+P | print |
| Cmd+Plus / Minus / 0 | zoom |
| Cmd+[ / ] | back / forward |
| Esc | stop loading, close find, restore the address field |

## Default browser

R-WEB-120. The app bundle declares `http`, `https` and `file` URL schemes and the `.html`, `.htm` document types. Settings › Browser has "Make notch-lab the default browser", which calls `NSWorkspace.setDefaultApplication`.

R-WEB-121. A URL from another app opens a new Tab in the browser Bench of the selected Lab of the front main window, in the focused Leaf, and brings the window to the front. When the selected Lab has no browser Bench, the app creates one named "Browser". When no main window is open, the app opens one.

R-WEB-122. The notch panel never takes a URL from another app.

R-WEB-123. A `.html` file opened from Finder loads as `file://` under R-WEB-032.

## Extensions

### Controller and adapters

R-WEB-130. The app creates one `WKWebExtensionController` at start with a configuration whose `defaultWebsiteDataStore` is the shared Profile, and attaches the controller to every Web view configuration, including popups and the Probe's offscreen pages. The controller's web view configuration sets the same `applicationNameForUserAgent` as the Pages.

R-WEB-131. The app registers `chrome-extension` as the custom URL scheme for Extension pages with `WKWebExtension.MatchPattern.registerCustomURLScheme`. An Extension's pages have the address `chrome-extension://<id>/…`, as in Chrome.

R-WEB-132. The app implements `WKWebExtensionTab` for every browser Tab and `WKWebExtensionWindow` for the main window and for the notch panel, as ObjC classes registered at run time through purego. The mapping:

| Protocol member | Value |
|---|---|
| tab `indexInWindow` | the Tab's position among the Tabs of its Bench, pinned first |
| tab `isSelected` | the Tab is in the focused Leaf |
| tab `window` | the window whose Layout shows the Tab, else the main window |
| tab `webView` | the Tab's Web view, or `nil` while asleep |
| tab `url`, `title`, `isLoadingComplete` | from the Page |
| tab `isPinned`, `isMuted`, `isPlayingAudio`, `isReaderModeAvailable` | pinned flag, muted flag, audio flag, false |
| tab `close`, `activate`, `reload`, `goBack`, `goForward`, `setPinned`, `setMuted`, `loadURL`, `duplicate` | the app's own commands |
| window `tabs` | the Tabs of the selected browser Bench of that window |
| window `activeTab` | the focused Leaf's Tab |
| window `frame`, `screenFrame`, `isFocused`, `focus`, `close` | the window's own |
| window `windowType` | `normal` for the main window, `popup` for the notch panel |
| controller delegate `openWindows` | the main window and the notch panel, front first |
| controller delegate `focusedWindow` | the key window |
| controller delegate `openNewTab` | a new Tab in the selected browser Bench of the given window, with the given URL, selected when asked |
| controller delegate `openNewWindow` | a new Tab in the main window (one window in v1) |
| controller delegate `openOptionsPage` | a Tab with the options page |
| controller delegate `promptForPermissions`, `promptForPermissionToAccessURLs`, `promptForPermissionMatchPatterns` | the permission sheet (R-WEB-170) |
| controller delegate `presentPopup` | the popup popover (R-WEB-160) |
| controller delegate `sendMessage`, `connectUsing port` | the native message handler (R-WEB-150) |

R-WEB-133. The app tells the controller about every Tab change: `didOpenTab`, `didCloseTab`, `didActivateTab`, `didSelectTabs`, `didMoveTab`, `didChangeTabProperties`, and about windows with `didOpenWindow`, `didCloseWindow`, `didFocusWindow`. The app sends these from the model events of `01-model.md`.

R-WEB-134. When the first Web view is made, or Settings › Extensions opens, the app makes the controller and loads enabled Extensions one after another (WebKit's Extension parts cost about 190 MB, which an app with no browser Tab awake does not pay), 400 ms apart when the previous one has background content: WebKit fails some workers when all start at once. The app starts loading after the first frame. A Page that loads before every Extension is loaded waits at most 2 s for them, then loads.

R-WEB-135. An Extension that fails to load shows in Settings › Extensions with the error text and stays disabled.

### Install from a Store

R-WEB-140. On `https://chromewebstore.google.com`, the app injects a user script at document end in the main frame, in a content world of its own with a random name, as Search's `StoreRelay` does. The app tells the script the installed ids and the busy id through the attribute `data-notch-store` on the document element, which both worlds see. The script:

1. Hides the "Switch to Chrome" dialog, known by the Chrome logo image it carries.
2. Hides the banner block around the enabled button whose `aria-label` names Chrome, going up from the button while the block is under 160 characters and holds no install button.
3. Finds the disabled button whose text names Chrome, clones it without `disabled`, `jsaction`, `jscontroller`, `jsname`, `jslog`, `aria-describedby`, labels the clone "Add to notch-lab", hides the original and puts the clone after it.
4. Repeats on every DOM change, at most once per 60 ms, through a `MutationObserver`.
5. On a click on the clone, caught on the window in the capture phase, posts `{add: true}` to the app and stops the event.
6. Labels the clone "Added to notch-lab" when the id is installed and "Adding…" while it installs; the app tells the page the installed ids and the busy id on every change.

R-WEB-141. The app reads the Extension id from the Tab's URL only: the path matches `^/detail/(?:[^/]+/)?([a-p]{32})(?:/.*)?$` on `chromewebstore.google.com` or `^/webstore/detail/(?:[^/]+/)?([a-p]{32})(?:/.*)?$` on `chrome.google.com`. The URL must be `https` with no user or password. The page's own words are never read for the id.

R-WEB-142. Before the download the app shows a sheet: the Extension's name and icon from the store page title, the text "notch-lab downloads this extension from the Chrome Web Store and checks its signature", and Add and Cancel. After the download and the check, a second sheet lists the manifest's `permissions` and `host_permissions` in plain words, with Add and Cancel. The second sheet is skipped when the lists are empty.

R-WEB-143. The download URL is `https://clients2.google.com/service/update2/crx?response=redirect&prodversion=140.0.0.0&acceptformat=crx3&x=id%3D<id>%26installsource%3Dondemand%26uc`, over https only, with a 60 s timeout. A status outside 200–299 refuses with "The Chrome Web Store answered <code>". An empty body refuses with "The Chrome Web Store has nothing for that id — it may have been taken down, or only exist for old versions of Chrome".

R-WEB-144. The CRX3 check, before any file of the package is read:

1. The first 4 bytes are `Cr24`, the next 4 are version 3 little-endian, the next 4 are the header length. Else refuse with "What came back isn't a Chrome extension".
2. The header is a protobuf. Field 10000 is the signed header; its field 1 is the crx id as 16 bytes. The id's letters (each half byte as a letter a–p) must equal the asked id.
3. The signed message is `"CRX3 SignedData"` + one zero byte + the signed header's length as 4 bytes little-endian + the signed header + the zip.
4. Among the field 2 proofs (RSA with SHA-256), one proof's public key must hash (SHA-256, first 16 bytes, as letters) to the id, and that proof's signature must verify over the message with PKCS#1 v1.5. The Store's own proof does not count.
5. Else refuse with "The extension's signature doesn't hold up".

The acceptance test: a CRX with one byte of the zip changed is refused; a CRX for another id is refused; uBlock Origin Lite installs.

R-WEB-145. The zip is unpacked with `/usr/bin/ditto -x -k` into a scratch folder. The result must hold `manifest.json`. A package with a symbolic link anywhere is refused with "The extension couldn't be unpacked". The folder then replaces `$NOTCH_LAB_HOME/extensions/<id>/` whole. The folder mode is 0700.

R-WEB-146. On `https://addons.mozilla.org` the app injects a user script that replaces the "Add to Firefox" button with "Add to notch-lab". The id is the listing's slug from the URL path `/<locale>/firefox/addon/<slug>/`. The XPI downloads from `https://addons.mozilla.org/firefox/downloads/latest/<slug>/latest.xpi` over https. The XPI is a zip; the Mozilla signature in `META-INF` is not checked in v1 `[later]`. The permission sheet shows the text "This extension's signature is not checked" in v1.

R-WEB-147. Settings › Extensions has a field "Paste a link to an extension, or its id" with Add. The app finds 32 letters a–p in the text, or an AMO link, and installs as above. "Open the Store" opens `https://chromewebstore.google.com/category/extensions` in a new Tab.

R-WEB-148. Settings › Extensions has "Load from folder…". The folder is loaded in place, with the id Chrome gives an unpacked folder: the first 16 bytes of the SHA-256 of the path, as letters a–p. The app writes the Shim into the folder (R-WEB-150). "Reload" re-reads the folder. "Load from file…" takes a `.crx`, `.xpi` or `.zip`: a `.crx` goes through R-WEB-144 with the id from its header; a `.xpi` and a `.zip` are unpacked under R-WEB-145 with the id from the manifest's `browser_specific_settings.gecko.id` or `key`, else `local-…`.

R-WEB-149. The list of installed Extensions is `$NOTCH_LAB_HOME/extensions/installed.json`: an array of `{id, name, version, enabled, pinned, local, updated}`: `local` is the folder of an unpacked Extension, `updated` the Unix time of the last download.

### The Shim

R-WEB-150. After the signature check, the app prepares the folder as Search's `ExtensionShims.prepare` does:

1. Writes `notch-shim.js` into the folder. The file is Search's `search-shim.js` with the native application name changed from `search` to `notchlab` and the markers changed to `/* notch-lab: Chrome APIs WebKit lacks, filled in */` and `/* notch-lab: end of shim */`. The rest is kept verbatim.
2. Adds `nativeMessaging` to the manifest's `permissions`, and `scripting` when the manifest asks for `userScripts`. Writes the added names to `.notch-added` so an update's permission diff ignores them.
3. Puts the Shim in front of the background: a classic service worker gets the Shim text at the top of its file between the markers; a module worker gets `import "/notch-shim.js";` as its first line; background `scripts` get `notch-shim.js` first.
4. Puts the Shim in front of every HTML page the manifest names (popup, options, side panel, offscreen) as a `<script src="/notch-shim.js">` first in `<head>`.
5. Writes `.notch-shim` with the Shim's version (first 8 hex of the SHA-256 of the Shim text). A folder with the same stamp is not prepared again.

A worker path that resolves outside the folder or is a link is left alone.

R-WEB-151. The native message handler in Go answers the Shim's messages. One handler per Extension context. The Shim's messages and the app's answers:

| Message prefix | Answered from |
|---|---|
| `bookmarks.*` | the bookmarks tree (R-WEB-071) |
| `history.*` | the history table (R-WEB-080) |
| `downloads.*` | the downloads list (R-WEB-042); `download` starts one |
| `sidePanel.*` | a drawer at the right of the Leaf, 320 pt wide, that shows the panel page |
| `offscreen.*` | a hidden Web view per Extension, one at a time, closed on `closeDocument` or when the Extension unloads |
| `tabGroups.*` | groups kept in memory per Bench, `[later]` shown in the sidebar |
| `identity.*` | `launchWebAuthFlow` opens the URL in a popup Web view window 480×640 pt; the app returns the redirect URL that starts with the Extension's redirect prefix and closes the window; `getAuthToken` is refused with "not supported" |
| `webRequest.*` | observation only: the app sends `onBeforeRequest`, `onCompleted` and `onErrorOccurred` from the Probe's network entries of Tabs the Extension has host permission for |
| `runtime.userAgent` | the Chrome user agent string with version 140.0.0.0 |

R-WEB-152. A message the handler does not know answers with `{"error": "unsupported: <name>"}` and writes one log line per name per Extension per run.

R-WEB-153. Screen capture from an Extension (`tabCapture`, `desktopCapture`) asks on a sheet named for the Extension and is remembered per Extension, as Search's `ExtensionCapture`. The app never shows WebKit's own dialog for it.

### Popups and pages

R-WEB-160. Each enabled Extension with an action shows a 24 pt button at the right end of the address bar, in the Extensions' pinned order; unpinned ones sit in the "more" menu. The button shows the action's icon and badge text on the action's badge color. The button is disabled when the action is disabled for the Tab.

R-WEB-161. A click on the button opens the action's popup in a popover anchored to the button. The popup is a Web view made from the Extension's web view configuration, not WebKit's own popover: for some Extensions WebKit's popup never reaches the worker. The popover sizes to the page's content between 25 and 800 pt wide and up to 600 pt tall, measured after load and on every change through a 100 ms timer. `window.close()` closes it. Esc closes it. A click outside closes it. A click on the same button within 300 ms after a close does not reopen it.

R-WEB-162. The popup's `WKWebExtensionTab` has no window of its own: "the current window" from a popup is the window that holds the button.

R-WEB-163. An Extension's options page opens as a Tab in the Bench with the title "<Extension name> options". The context menu of the Extension button has Options, Pin/Unpin, Disable, Remove, Manage.

R-WEB-164. A side panel (R-WEB-151) opens in a drawer at the right of the Leaf. Cmd+Alt+S toggles the drawer. One side panel is open per Leaf at a time.

### Permissions and updates

R-WEB-170. A permission request from an Extension (`permissions.request`, host access on first use) shows a sheet named for the Extension with the permissions in plain words, Allow and Deny. The choice is remembered per Extension in `installed.json` under `granted`.

R-WEB-171. Every hour the app downloads again each Store Extension whose last download is older than a day. A new version installs in place and reloads the Extension; the same version only updates `updated`. The text below is the full rule `[later]`: the app asks the Store for each Store Extension's current version through the same download URL with a HEAD request. When the version in the redirect's file name is newer, the app downloads and checks the CRX (R-WEB-143, R-WEB-144), compares the manifest's permissions with the installed ones minus `.notch-added`, and:

- installs in place when no permission was added, then reloads the Extension;
- else shows a Toast "<name> asks for new permissions" with Review, which opens the permission sheet; Allow installs.

R-WEB-172. An update replaces the folder only after the check passes. The old folder is kept until the new one is in place.

### Settings › Extensions

R-WEB-180. The section lists every installed Extension with icon, name, version, source (Store, AMO, folder, file), a toggle, Pin, Options, Reload (folder only), Remove, and an Errors disclosure with the last 40 errors the Extension's pages and worker raised.

R-WEB-181. Remove unloads the Extension, deletes its folder and its data in the controller's data store, and removes it from `installed.json`. The app asks for confirmation.

R-WEB-182. Disable unloads the Extension and keeps its folder and data.

### Known limits

| Limit | Reason |
|---|---|
| no blocking `webRequest` | WebKit has `declarativeNetRequest` only |
| no `devtools` panels | WebKit does not host them |
| no Extension sync | no account |
| no `chrome.identity.getAuthToken` | Google-account only |
| no `chrome.tabCapture` of other Tabs | WebKit gives a Tab its own capture only |
| Extension data is per app, not per Profile | one controller data store |
| no `chrome.sessions`, `chrome.fontSettings`, `chrome.enterprise.*`, `chrome.printing` | not shimmed |

Settings › Extensions shows this table under "What does not work".

## The Probe

R-WEB-190. The Probe is two user scripts the app adds at document start to every frame of a Tab that is watched: a Tab an agent opened, a Tab an agent asked about, or a Tab whose Console drawer is open.

1. The watcher runs in the page's world. It wraps `console.log`, `info`, `warn`, `error`, `debug`, `trace`, `assert`, listens to `error` and `unhandledrejection`, wraps `fetch` and `XMLHttpRequest`, and observes `PerformanceObserver` entries of type `navigation` and `resource`. It reports each item as a `CustomEvent` on `document` with a name made at launch from a random UUID, carrying one string.
2. The listener runs in a private content world named `notchlab`. It hears the events and posts them to the app in batches of at most 50 or every 100 ms.

The listener never runs in the page's world. The watcher never has a reference the page can reach except the event name.

R-WEB-191. The app keeps only the fields it knows, with its own clock and its own frame id: `time`, `frame`, `kind` (console, error, request, response, resource, navigation), `level`, `text` cut at 4 KB, `url`, `method`, `status`, `headers` cut at 32 entries, `body` cut at 64 KB, `duration`.

R-WEB-192. The Probe masks secrets before they leave the page: the user and password of a URL; the value of any query or fragment parameter whose name matches `^(authorization|proxy-authorization|cookie|set-cookie|auth|sid|key|code|sig)$|token|secret|passw|pwd|passcode|otp|csrf|xsrf|api[-_]?key|apikey|credential|session[-_]?id|signature|private[-_]?key|security[-_]?token` case-insensitive; the value of any header with such a name; any JSON field with such a name in a body. The mask is `•••`.

R-WEB-193. The app keeps a ring buffer of 2 000 entries per Tab. A Tab put to sleep keeps its buffer. A closed Tab drops it.

R-WEB-194. Cmd+Alt+J opens the Console drawer at the bottom of the Leaf, 240 pt tall, resizable. It has tabs Console and Network, a filter field, a level filter (all, warn, error), Clear, and Preserve on navigation (default off). A console row shows time, level, text and the frame. A network row shows method, URL, status, size, duration; a click shows request and response headers and bodies.

R-WEB-195. Documents and failed loads come from the Web view's navigation events, not from the Probe.

R-WEB-196. WebSocket frames `[later]`. Worker console `[later]`.

## Snapshot and actions for agents

R-WEB-200. A snapshot is an outline of the page built in the listener's world from the DOM: one line per element that has a role, a name, is focusable, is a link, a form control, a heading, an image with alt, or a landmark. Each line has: a ref `e<n>`, the role, the name cut at 120 characters, the state (checked, disabled, expanded, selected), the value of a form control (masked for password fields), the bounds in page coordinates, and a depth. Hidden elements (`display:none`, `visibility:hidden`, `aria-hidden`, zero size) are left out. The outline is at most 2 000 lines; the app cuts it and adds a last line "… <n> more".

R-WEB-201. A ref maps to its element in the agents' own content world, which the app makes at launch and keeps alive. The map holds the elements strongly until the next snapshot: a `WeakRef` there holds that world's wrapper, which nothing else holds, and loses it at the next collection. Refs expire on navigation. An action with an expired ref answers "ref expired, take a new snapshot".

R-WEB-205. A Web view that no Leaf placed (a Tab no Leaf shows, or a window that draws no frames) gets a 1280 x 800 pt frame before a tool acts on it, so its page lays out and takes input.

R-WEB-202. Actions are sent as trusted events through the mygo fork's input injection into the Web view, at the element's center in window coordinates, never as JavaScript `dispatchEvent`:

| Action | How |
|---|---|
| click (ref or x, y) | scroll the element into view, then mouse down and up |
| type (ref, text) | click the element, select all when asked, then key events per character; Enter when asked |
| select (ref, value) | click, then set the option through the page world and fire `input` and `change` |
| press (key, modifiers) | key down and up to the Web view |
| scroll (ref, or dx, dy) | the page scrolls the element into view, or scrolls by dx, dy (wheel events `[later]`) |
| hover (ref) | mouse move to the point |

R-WEB-203. The app refuses an action while a permission sheet or a `beforeunload` sheet is up: "the page waits for the user".

R-WEB-204. A screenshot is `WebView.CapturePage` of the Tab's Web view as PNG, scaled so the longer side is at most 1 600 px. `Window.CapturePage` is never used for a Page: it shows a hole where the page is.

## MCP tools, domain `browser`

Every tool takes `tab` (a Tab id) unless noted. `R` = Read, `F` = Full. A tool with level `F` is refused at Read with "needs Full access".

| Tool | Params | Level | Returns |
|---|---|---|---|
| `browser.tabs.list` | `bench?` | R | Tabs of browser Benches the Client may see: id, bench, title, url, selected, asleep |
| `browser.tabs.open` | `url?`, `bench?`, `show?` | F | the new Tab's id; the Client owns the Tab |
| `browser.tabs.close` | `tab` | F | ok; refused for a Tab the Client does not own unless the Bench setting "agents may close" is on |
| `browser.navigate` | `tab`, `url` | F | ok after `load` or 30 s |
| `browser.back`, `browser.forward`, `browser.reload` | `tab` | F | ok |
| `browser.snapshot` | `tab`, `frame?` | R | the outline (R-WEB-200) |
| `browser.screenshot` | `tab`, `full?` | R | PNG as base64 |
| `browser.click` | `tab`, `ref?`, `x?`, `y?`, `button?` | F | ok |
| `browser.type` | `tab`, `ref`, `text`, `clear?`, `enter?` | F | ok |
| `browser.select` | `tab`, `ref`, `value` | F | ok |
| `browser.press` | `tab`, `key`, `modifiers?` | F | ok |
| `browser.scroll` | `tab`, `ref?`, `x?`, `y?`, `dx`, `dy` | F | ok |
| `browser.hover` | `tab`, `ref` | F | ok |
| `browser.eval` | `tab`, `js`, `frame?` | F | the JSON result, cut at 256 KB; runs in the page world only, never in the listener's world |
| `browser.console.read` | `tab`, `since?`, `level?` | R | entries (R-WEB-191) |
| `browser.network.read` | `tab`, `since?`, `url_filter?` | R | entries |
| `browser.find` | `tab`, `text` | R | the count and the first 20 refs that hold the text |
| `browser.wait_for` | `tab`, `load?`, `selector?`, `text?`, `timeout?` (default 10 s, max 60 s) | R | ok or "timeout" |
| `browser.cookies.list` | `tab`, `domain?` | F | name and value of the page's `document.cookie`; HttpOnly cookies from `WKHTTPCookieStore` are `[later]` |
| `browser.downloads.list` | — | R | the downloads list |

R-WEB-210. A Client sees the Tabs of a browser Bench with an isolated Profile only when the Bench setting `agentsMayUse` is on. Default off. The shared Profile's Benches are visible by default.

R-WEB-211. A snapshot, a screenshot, `eval`, console and network entries never return a saved password or an autofill value: password fields read `•••`; the app never calls WebKit's autofill for a Client.

R-WEB-212. Every tool call goes to the Audit log with the Tab, the params with secrets masked, and the result's size.

R-WEB-213. A Tab a Client opened closes when the Client disconnects and Settings › Agents "Close an agent's own Tabs" is on. Keeping a Tab the user interacted with is `[later]`.

## Performance

| Measure | Limit |
|---|---|
| memory of an asleep Tab | 0 (no Web view) |
| awake Tabs per Bench | 12 |
| time to show an awake hidden Tab | 1 frame |
| Store install, download to loaded, for a 5 MB CRX | 5 s |
| snapshot of a page with 5 000 elements | 300 ms |
| Probe batch delay | 100 ms |

R-WEB-220. Web views of one Profile share one WebKit process pool. The app never creates a process pool per Tab.

## Security

R-WEB-230. Every Store and AMO download is over https. A redirect to `http` fails the download.

R-WEB-231. The CRX check (R-WEB-144) runs before any file of the package is read. Unpacking (R-WEB-145) refuses links.

R-WEB-232. `$NOTCH_LAB_HOME/extensions/` and every folder under it are mode 0700.

R-WEB-233. An Extension never loads from a `file://` URL in a Page. Extension pages load from `chrome-extension://` only.

R-WEB-234. The Store user script reads the id from the Tab's URL, not from the page (R-WEB-141).

R-WEB-235. The Probe masks secrets (R-WEB-192). A snapshot masks password fields and fields whose `name`, `id` or `autocomplete` matches the secret pattern.

R-WEB-236. The listener's content world name and the event name are made at launch and never written to a page.

R-WEB-237. The app refuses `browser.eval` and `browser.cookies.list` at Read level and logs the refusal.

## Acceptance tests

| Requirement | Test |
|---|---|
| R-WEB-003 | restart with 20 Tabs: 0 Web views until a Leaf shows one |
| R-WEB-030 | type `localhost:3000` → `https://localhost:3000`, then `http://localhost:3000` on connection error; type `go generics` → search |
| R-WEB-034 | a page's `window.open` on load → Tab in the sidebar only; a click on a `_blank` link → Tab shown |
| R-WEB-063 | delete an isolated Bench → `allDataStoreIdentifiers` lacks the id within 20 s |
| R-WEB-140 | open uBlock Origin Lite's Store page → "Add to notch-lab" button in place within 1 s; click → sheet |
| R-WEB-144 | the three cases named in the requirement |
| R-WEB-150 | after install, the worker's first line is the Shim marker; `.notch-shim` holds the version |
| R-WEB-161 | Bitwarden's popup opens and reaches its worker (no spinner after 3 s) |
| R-WEB-192 | fetch `https://x/?token=abc` → network entry URL `https://x/?token=•••` |
| R-WEB-200 | snapshot of `example.com` has a heading and a link with refs; click by ref navigates |
| R-WEB-202 | type into a React input by ref → React's state updates (trusted events) |
| R-WEB-210 | a Client lists no Tab of an isolated Bench until `agentsMayUse` is on |
