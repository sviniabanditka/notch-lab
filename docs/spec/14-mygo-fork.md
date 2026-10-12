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

## Patches: WebKit (M4, done)

mygo 0.4.5 already has the Web view features the browser Bench needs: navigation, history, find, downloads, permissions, zoom, titles, load errors, `Eval`, new windows. The fork adds one patch, which opens the native objects. Everything WebKit-specific beyond that lives in notch-lab's `internal/webkit`, written with purego and the ObjC runtime.

| Patch | API | Requirement | Commit |
|---|---|---|---|
| webview-hooks | `PageOptions.Configure func(native uintptr)`, `WebView.NativeHandle()`, `Page.SetWebViewOpenHandler`, `WindowOpenRequest.Native`, `WebViewOptions.Native` | R-MYGO-030 | `notch: web view hooks` |

R-MYGO-030. `PageOptions.Configure` runs with the `WKWebViewConfiguration` before the Web view is made: the app sets the data store and the Extension controller there. `NativeHandle` returns the `WKWebView`. `SetWebViewOpenHandler` answers `window.open()` and `target=_blank` with a `*WebView` the app makes from `WindowOpenRequest.Native`, the configuration WebKit hands over; `WebViewOptions.Native` makes a Web view from such a configuration. An Extension popup uses the same field with the Extension's own configuration.

R-MYGO-031. `internal/webkit` holds, in notch-lab: the data store per Profile (`dataStoreForIdentifier:`), user scripts in named content worlds, script message handlers, `isInspectable`, the default browser calls, and the WebExtension bridge (`04-browser.md` R-WEB-130 … R-WEB-133): the controller and its delegate, contexts, and the classes `NotchExtTab`, `NotchExtWindow`, `NotchExtDelegate` registered at run time with `objc.RegisterClass`. Completion handlers are copied with `_Block_copy` and called through the block's invoke pointer.

R-MYGO-032. purego limits: a method registered with `objc.RegisterClass` cannot return a float, so the optional `zoomFactorForWebExtensionContext:` is not implemented; a struct argument (`CGSize` for `iconForSize:`) goes through `purego.RegisterFunc` on `objc_msgSend` with a Go struct, not an array.

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
| upstream changes `WebView` internals between tags | the web view hooks conflict | the patch is small and keeps new code in `notch_*.go` |
| `WKWebExtension` protocols gain required methods in a new macOS | the registered class misses a selector; WebKit logs and skips it | the classes in `internal/webkit` are checked against each new SDK; missing methods return defaults |
| purego callback from a non-main thread | WebKit calls some delegate methods off the main thread | every callback marshals to the main goroutine before touching Go state; reply handlers are called from the callback's thread |
| global pointer monitor costs CPU | the hover trigger polls on every mouse move | the monitor only compares against the notch rect; the app installs it only while `notch.hoverShows` is true |
| `setActivationPolicy` in full screen | desktop switch | refused by R-MYGO-013 |
| a display without a notch | `Screen.Notch()` false | the app uses the top-center strip (R-SHELL-015) |
| `KeyOnClick` and a press in the app's own menu bar | the local monitor sees no menu bar press; the flyout stays | the flyout also closes on `App.OnDidResignActive` and on Escape |
