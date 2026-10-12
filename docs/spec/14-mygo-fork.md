# The fork of mygo

Vocabulary: `CONTEXT.md`. Users of these features: `02-shell.md`, `04-browser.md`.

## Location and rules

R-MYGO-001. The fork lives at `github.com/sviniabanditka/mygo`, branch `notch-lab`. It tracks upstream tag `v0.4.5`.

R-MYGO-002. `go.mod` of notch-lab requires `github.com/egoist/mygo v0.4.5` and has `replace github.com/egoist/mygo => github.com/sviniabanditka/mygo <pseudo-version of the pinned commit>`. The fork is private: builds set `GOPRIVATE=github.com/sviniabanditka` (`tools/dev`, `tools/verify/nl`, CI).

R-MYGO-003. No upstream pull requests. Every patch is one commit with the subject prefix `notch:` and one patch name from the table below. A commit never mixes two patches.

R-MYGO-004. A patch changes upstream files as little as possible. New code goes in new files under `notch_*.go` where the API allows it, so a rebase conflicts on exports only.

R-MYGO-005. Each patch has a program `examples/notch-<patch>/main.go` that shows it, and a `ui.Tester` test where the patch has no window-system dependence.

## Upstream already covers

mygo 0.4.5 has these. The fork does not patch them. The app uses them as they are.

| Need (02-shell.md) | Upstream API |
|---|---|
| a non-activating panel | `NewFlyout`: an `NSPanel` with `NSWindowStyleMaskNonactivatingPanel` |
| floating level, above other apps | `WindowOptions.AlwaysOnTop`, `Window.SetAlwaysOnTop`; a Tray or Screen flyout floats by itself |
| window frame get and set | `Window.Bounds`, `Window.SetBounds`, `SetPosition`, `SetSize` |
| Dock icon on and off | `App.SetActivationPolicy(ActivationPolicyRegular | ActivationPolicyAccessory)` |
| every Space, over full screen | `Window.SetVisibleOnAllWorkspaces`; a Screen flyout sets `canJoinAllSpaces` and `fullScreenAuxiliary` itself |
| mouse pass-through | `Window.SetIgnoreMouseEvents` |
| app deactivated, window key or not | `App.OnDidResignActive`, `App.OnDidBecomeActive`, `Window.OnFocus`, `Window.OnBlur` |
| full screen events | `Window.OnEnterFullScreen`, `Window.OnLeaveFullScreen`, `Window.IsFullScreen` |
| displays and the pointer | `Screen.Displays`, `Screen.PrimaryDisplay`, `Screen.CursorScreenPoint`, `Screen.OnDisplaysChanged` |
| extra `Info.plist` keys, minimum macOS | `mygo.json` `macos.infoPlist`, `macos.minimumSystemVersion` |
| a helper binary in the bundle, signed with the app | `resources/darwin-<arch>/bin/<name>`, copied to `Contents/Resources/bin/` |
| animation of a frame change | the app moves the window per frame with `SetBounds` (`02-shell.md` R-SHELL-014) |

R-MYGO-016. Documented gotcha, carried from notch-browser: `SetActivationPolicy` while a window is full screen switches desktops. The app never calls it while `IsFullScreen()` is true on any window; it waits for `OnLeaveFullScreen`.

## Patches: windows (M0, done)

| Patch | API | Requirement | Commit |
|---|---|---|---|
| screen-flyout | `FlyoutOptions.Screen` | R-MYGO-010 | `notch: screen flyout` |
| key-on-click | `FlyoutOptions.KeyOnClick` | R-MYGO-011 | `notch: flyout key on click` |
| screen-notch | `Screen.Notch() (Rectangle, bool)` | R-MYGO-019 | `notch: Screen.Notch` |
| global-pointer | `App.OnGlobalPointer(func(x, y int)) (off func())` | R-MYGO-020 | `notch: global pointer` |

R-MYGO-010. `FlyoutOptions.Screen: true` makes a flyout with neither a Parent nor a Tray. `Anchor` is then in screen coordinates (`Display.Bounds`). The flyout is placed against the anchor inside the bounds of the display the anchor's center is on, the menu bar included, not the work area. The panel has level floating and collection behavior `canJoinAllSpaces | transient | ignoresCycle | fullScreenAuxiliary`. Acceptance: `examples/notch-screen-flyout` hangs a panel from the top center of the primary display over the windows of other apps.

R-MYGO-011. `FlyoutOptions.KeyOnClick: true` lets the flyout become the key window, but shows it without the keyboard: the front app keeps typing until the user presses in the flyout. A press outside it, in the app or in another app, closes it (`OnClose` can keep it). Escape closes it. Acceptance: `examples/notch-key-on-click` over TextEdit: typing goes to TextEdit until a click in the flyout's text field.

R-MYGO-019. `Screen.Notch()` returns the notch rectangle in screen coordinates and `true` on a Mac whose built-in display has `safeAreaInsets.top > 0` (macOS 12 and later): the gap between `auxiliaryTopLeftArea` and `auxiliaryTopRightArea`, as tall as the top inset. `false` on a display without a notch. `OnDisplaysChanged` fires when the result can change. Acceptance: `examples/notch-screen-notch` prints `{918 0 220 38} true` on a 14-inch MacBook Pro and hangs a panel below it.

R-MYGO-020. `App.OnGlobalPointer` installs one global and one local `NSEvent` monitor for mouse moves while a listener is registered, and removes them when the last `off` runs. The callback runs on the main goroutine with the position in `Display.Bounds` coordinates. No Accessibility permission is needed. Acceptance: `examples/notch-pointer` prints positions while another app is in front.

## Patches: WebKit

| Patch | API | Requirement |
|---|---|---|
| datastore | `WebViewOptions.DataStoreID string` | R-MYGO-030 |
| extensions | `WebViewOptions.ExtensionController *webkit.ExtensionController` and package `webkit` | R-MYGO-031 … R-MYGO-036 |
| inspectable | `Page.SetInspectable(bool)` | R-MYGO-040 |
| userscript | `Page.AddUserScript(source, when, mainFrameOnly, world)` | R-MYGO-041 |
| handler | `Page.AddScriptMessageHandler(name, world, func(body any))` | R-MYGO-042 |
| evalworld | `Page.EvalInWorld(world, js) (any, error)` | R-MYGO-043 |
| download | `Page.OnDownload(func(d *Download))` | R-MYGO-044 |
| popup | `Page.OnCreateWebView(func(req NavigationRequest) *WebView)` | R-MYGO-045 |
| permission | `Page.OnPermission(func(kind, origin string) Decision)` | R-MYGO-046 |
| find | `Page.Find(text string, opts FindOptions) (count int, err error)` | R-MYGO-047 |
| zoom | `Page.SetZoom(float64)`, `Page.Zoom()` | R-MYGO-048 |
| snapshot | `Page.Snapshot(rect) (image.Image, error)` | R-MYGO-049 |
| contextmenu | `Page.OnContextMenu(func(info ContextMenuInfo) []MenuItem)` | R-MYGO-050 |
| title | `Page.OnTitleChanged`, `Page.OnURLChanged`, `Page.OnProgress`, `Page.OnFaviconChanged` | R-MYGO-051 |
| naverror | `Page.OnNavigationError(func(url string, code int, msg string))` | R-MYGO-052 |
| dialogs | `Page.OnJSDialog(func(kind, msg, def string) (ok bool, text string))` | R-MYGO-053 |
| auth | `Page.OnAuthChallenge(func(host string) (user, pass string, ok bool))` | R-MYGO-054 |

R-MYGO-030. `DataStoreID` is a UUID text. The fork creates the configuration with `[WKWebsiteDataStore dataStoreForIdentifier:]`. An empty value means the default store. `webkit.RemoveDataStore(id)` calls `removeDataStoreForIdentifier:completionHandler:` and returns after completion. `webkit.DataStoreIDs()` lists the identifiers that exist.

R-MYGO-031. Package `github.com/egoist/mygo/webkit` exposes, through purego and the ObjC runtime, these classes: `WKWebExtension`, `WKWebExtensionContext`, `WKWebExtensionController`, `WKWebExtensionAction`, `WKWebExtensionMatchPattern`, `WKWebExtensionMessagePort`, `WKWebExtensionDataRecord`. Each Go type wraps one ObjC object id and exposes the methods the browser Bench needs (`04-browser.md`). Objects are retained in Go and released in a finalizer.

R-MYGO-032. The fork registers two ObjC classes at run time with `objc.RegisterClass`: `NotchExtensionTab` conforming to `WKWebExtensionTab`, and `NotchExtensionWindow` conforming to `WKWebExtensionWindow`. Every protocol method calls a Go function through a table keyed by the object id. The Go side implements `webkit.ExtensionTab` and `webkit.ExtensionWindow` interfaces:

```go
type ExtensionTab interface {
    Window() ExtensionWindow
    Index() int
    ParentTab() ExtensionTab
    WebView() *mygo.WebView
    Title() string; URL() string; PendingURL() string
    IsLoadingComplete() bool
    IsSelected() bool; IsPinned() bool; IsMuted() bool; IsPlayingAudio() bool
    IsReaderModeAvailable() bool; IsReaderModeActive() bool
    Size() (w, h float64); ZoomFactor() float64
    SetZoomFactor(float64) error; SetPinned(bool) error; SetMuted(bool) error
    SetSelected(bool) error; SetReaderModeActive(bool) error
    DetectWebpageLocale() string
    Reload(fromOrigin bool) error; GoBack() error; GoForward() error
    LoadURL(string) error; Activate() error; Close() error
    Duplicate(opts TabOpts) (ExtensionTab, error)
    CaptureVisible() (image.Image, error)
    ShouldGrantPermissionsOnUserGesture() bool
    ShouldBypassPermissions() bool
}
type ExtensionWindow interface {
    Tabs() []ExtensionTab; ActiveTab() ExtensionTab
    Type() WindowType; State() WindowState
    IsPrivate() bool; IsFocused() bool
    Frame() Rect; ScreenFrame() Rect
    SetFrame(Rect) error; SetState(WindowState) error
    Focus() error; Close() error
}
```

R-MYGO-033. `ExtensionController` wraps `WKWebExtensionController` with a configuration whose identifier is a UUID (`[WKWebExtensionControllerConfiguration configurationWithIdentifier:]`), so Extension storage persists. The controller's delegate is a registered class `NotchExtensionControllerDelegate` that forwards to Go callbacks:

| Delegate method | Go callback |
|---|---|
| `openWindowsForExtensionContext:` | `OpenWindows() []ExtensionWindow` |
| `focusedWindowForExtensionContext:` | `FocusedWindow() ExtensionWindow` |
| `openNewWindowUsingConfiguration:forExtensionContext:completionHandler:` | `OpenNewWindow(cfg) (ExtensionWindow, error)` |
| `openNewTabUsingConfiguration:forExtensionContext:completionHandler:` | `OpenNewTab(cfg) (ExtensionTab, error)` |
| `openOptionsPageForExtensionContext:completionHandler:` | `OpenOptionsPage(ctx) error` |
| `promptForPermissions:inTab:forExtensionContext:completionHandler:` | `PromptPermissions(perms, tab) (granted []string, expiry)` |
| `promptForPermissionToAccessURLs:inTab:forExtensionContext:completionHandler:` | `PromptURLs(urls, tab) (granted, expiry)` |
| `promptForPermissionMatchPatterns:inTab:forExtensionContext:completionHandler:` | `PromptPatterns(patterns, tab) (granted, expiry)` |
| `presentPopupForAction:forExtensionContext:completionHandler:` | `PresentPopup(action) error` |
| `sendMessage:toApplicationWithIdentifier:forExtensionContext:replyHandler:` | `NativeMessage(appID string, msg any) (reply any, err error)` |
| `connectUsingMessagePort:forExtensionContext:completionHandler:` | `NativeConnect(port *MessagePort) error` |
| `didUpdateAction:forExtensionContext:` | `ActionUpdated(action)` |

R-MYGO-034. `ExtensionContext` exposes: `Load`, `Unload`, `Extension()`, `UniqueIdentifier`, `BaseURL`, `IsLoaded`, `Errors() []error`, `HasRequestedOptionalAccessToAllHosts`, `CurrentPermissions`, `CurrentPermissionMatchPatterns`, `GrantPermissions(perms, expiry)`, `GrantMatchPatterns(patterns, expiry)`, `RevokePermissions`, `RevokeMatchPatterns`, `Action(tab) *Action`, `PerformAction(tab)`, `PerformCommand(id)`, `Commands`, `MenuItems(tab) []MenuItem`, `DidOpenTab(tab)`, `DidCloseTab(tab, windowClosing)`, `DidActivateTab(tab, previous)`, `DidSelectTabs`, `DidDeselectTabs`, `DidMoveTab(tab, oldIndex, oldWindow)`, `DidReplaceTab(old, new)`, `DidChangeTabProperties(props, tab)`, `DidOpenWindow`, `DidCloseWindow`, `DidFocusWindow`, `OptionsPageURL`, `OverrideNewTabPageURL`, `InspectionName`, `SetInspectable`, `UnsupportedAPIs`, `WebViewConfiguration`.

R-MYGO-035. `Extension` exposes: `NewExtensionFromDirectory(path) (*Extension, error)`, `Manifest() map[string]any`, `ManifestVersion`, `DisplayName`, `DisplayShortName`, `DisplayVersion`, `DisplayDescription`, `Version`, `Icon(size) image.Image`, `ActionIcon(size)`, `RequestedPermissions`, `OptionalPermissions`, `RequestedPermissionMatchPatterns`, `OptionalPermissionMatchPatterns`, `AllRequestedMatchPatterns`, `HasBackgroundContent`, `HasPersistentBackgroundContent`, `HasInjectedContent`, `HasOptionsPage`, `HasOverrideNewTabPage`, `HasCommands`, `HasContentModificationRules`, `Errors`, `DefaultLocale`, `SupportsManifestVersion(n) bool`.

R-MYGO-036. `Action` exposes: `Label`, `Icon(size)`, `BadgeText`, `HasUnreadBadgeText`, `SetHasUnreadBadgeText`, `IsEnabled`, `PresentsPopup`, `PopupWebView() *mygo.WebView`, `ClosePopup`, `InlinePresentationPopover`, `MenuItems`, `AssociatedTab`. `MessagePort` exposes `ApplicationIdentifier`, `SendMessage(any) error`, `OnMessage(func(any, error))`, `OnDisconnect(func(error))`, `Disconnect()`, `IsDisconnected`.

R-MYGO-037. `WebViewOptions.ExtensionController` sets `configuration.webExtensionController` at creation. The controller is set at creation only; a Web view created without it never loads Extensions.

R-MYGO-038. `WebViewOptions.UserContentWorld string` names a content world for the app's own scripts. The fork creates one `WKContentWorld` per name with `[WKContentWorld worldWithName:]`.

R-MYGO-040. `SetInspectable(true)` sets `isInspectable = YES` so Safari's Web Inspector can attach. Available on macOS 13.3 and later; the fork no-ops below.

R-MYGO-041. `AddUserScript(source, when, mainFrameOnly, world)`: `when` is `AtDocumentStart` or `AtDocumentEnd`; `world` is `WorldPage`, `WorldDefaultClient` or a named world (R-MYGO-038). `RemoveAllUserScripts()` exists.

R-MYGO-042. `AddScriptMessageHandler(name, world, fn)` registers a `WKScriptMessageHandlerWithReply` in that world. `fn` receives the JSON-decoded body and returns `(reply any, err error)`; the reply resolves the page's `webkit.messageHandlers.<name>.postMessage()` promise. A body larger than 4 MB is refused with an error to the page.

R-MYGO-043. `EvalInWorld(world, js)` calls `callAsyncJavaScript:arguments:inFrame:inContentWorld:completionHandler:` and returns the JSON-decoded result. A result that cannot be serialized returns `ErrNotSerializable`.

R-MYGO-044. `OnDownload` fires from `navigationResponse` with a non-displayable MIME type and from a `download` attribute link. `Download` exposes `SuggestedFilename`, `URL`, `MIME`, `Size`, `Start(dest string)`, `Cancel()`, `OnProgress(func(received, total int64))`, `OnFinish(func(err error))`. The app chooses the destination.

R-MYGO-045. `OnCreateWebView` fires from `webView:createWebViewWithConfiguration:forNavigationAction:windowFeatures:`. The callback returns a `*WebView` created on the same window with the given configuration, or nil to block. The fork passes the configuration through `WebViewOptions.ParentConfiguration` so the new view shares the data store, the extension controller and the process pool.

R-MYGO-046. `OnPermission(kind, origin)` covers `camera`, `microphone`, `cameraAndMicrophone`, `geolocation`, `notifications`, `deviceOrientation`. `Decision` is `Grant`, `Deny` or `Prompt`. `Prompt` shows WebKit's own prompt.

R-MYGO-047. `Find(text, opts)` uses `WKFindConfiguration`: `CaseSensitive`, `Backwards`, `Wraps`. It returns the count of matches via the page's `find` result; `FindNext`, `FindPrevious`, `ClearFind` exist. Match highlighting uses WebKit's own highlight.

R-MYGO-048. `SetZoom` sets `pageZoom`. Range 0.25 to 5.0.

R-MYGO-049. `Snapshot(rect)` calls `takeSnapshotWithConfiguration:completionHandler:` and returns an `image.RGBA`. An empty rect means the visible area.

R-MYGO-050. `OnContextMenu` receives `ContextMenuInfo{LinkURL, ImageURL, SelectedText, IsEditable, MediaURL}` and returns extra items appended to WebKit's menu. An item is `{Label, Action func(), Enabled}`. Items are built with `WKContextMenuElementInfo` where available; the fork falls back to a native menu at the pointer when WebKit gives no hook.

R-MYGO-051. `OnTitleChanged(func(string))`, `OnURLChanged(func(string))`, `OnProgress(func(float64))` observe `title`, `URL`, `estimatedProgress` with KVO through a registered observer class. `OnFaviconChanged(func(url string))` runs a user script at document end that posts `<link rel="icon">` hrefs to a handler named `notchFavicon` in a named world, and resolves `/favicon.ico` when none is declared.

R-MYGO-052. `OnNavigationError` fires from `didFailNavigation` and `didFailProvisionalNavigation` with the error's URL, code and localized description. A cancelled navigation (`NSURLErrorCancelled`) is not reported.

R-MYGO-053. `OnJSDialog(kind, msg, def)` covers `alert`, `confirm`, `prompt`. The callback returns on the main goroutine; the fork answers the completion handler after the callback returns. A callback that takes longer than 30 s answers `false`.

R-MYGO-054. `OnAuthChallenge(host)` answers HTTP basic and digest challenges. A `false` return cancels the challenge.

## Not patched

R-MYGO-060. The fork adds no terminal widget. The app paints terminal cells with `ui` primitives (`03-terminal.md`).

R-MYGO-061. The fork uses no private API. Every selector used is in the macOS 15.4 SDK headers. `docs/fork/selectors.md` in the fork lists every selector with the header it comes from.

## Run-time checks

R-MYGO-070. The fork exposes `mygo.OSVersion() (major, minor, patch int)` from `NSProcessInfo.operatingSystemVersion`.

R-MYGO-071. The app refuses to start on macOS below 15.4 with a native alert "notch-lab needs macOS 15.4 or later" and quits. The `webkit` package panics with a clear message when a `WKWebExtension` class is missing at run time, which the app prevents with R-MYGO-071.

## Build tool

R-MYGO-090. The fork changes nothing in `mygo build`. The Daemon ships as `resources/darwin-<arch>/bin/notch-labd` (`15-build-test.md`), and extra `Info.plist` keys go in `macos.infoPlist`.

## Rebase procedure

R-MYGO-080. To move the fork to a new upstream tag:

1. Fetch upstream. Create branch `notch-lab-<new tag>` from the new tag.
2. Cherry-pick every `notch:` commit from `notch-lab` in order.
3. Resolve each conflict inside that one commit. Keep the commit's single patch.
4. Run `go build ./...` and every `examples/notch-*` program. Run `go test ./...`.
5. Run `mygo migrate-ui` in notch-lab when the tag's notes ask for it.
6. Update the `replace` commit in notch-lab's `go.mod`. Run notch-lab's tests and the verify smoke run.
7. Force-move `notch-lab` to the new branch. Tag the old head `notch-lab-<old tag>-final`.

R-MYGO-081. `docs/fork/patches.md` in the fork lists every patch, its commit subject and its requirement id here. The list is updated in the same commit as the patch.

## Risks

| Risk | Effect | Response |
|---|---|---|
| upstream changes `WebView` internals between tags | the WebKit patches conflict | patches keep new code in `notch_*.go`; conflicts stay on the wiring lines |
| `WKWebExtension` protocols gain required methods in a new macOS | the registered class misses a selector; WebKit logs and skips it | `docs/fork/selectors.md` is checked against each new SDK; missing methods return defaults |
| purego callback from a non-main thread | WebKit calls some delegate methods off the main thread | every callback marshals to the main goroutine before touching Go state; reply handlers are called from the callback's thread |
| global pointer monitor costs CPU | the hover trigger polls on every mouse move | the monitor only compares against the notch rect; the app installs it only while `notch.hoverShows` is true |
| `setActivationPolicy` in full screen | desktop switch | refused by R-MYGO-013 |
| a display without a notch | `Screen.Notch()` false | the app uses the top-center strip (R-SHELL-015) |
| `KeyOnClick` and a press in the app's own menu bar | the local monitor sees no menu bar press; the flyout stays | the flyout also closes on `App.OnDidResignActive` and on Escape |
