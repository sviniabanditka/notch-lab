# 0002. Browser on WebKit with WKWebExtension

Date: 2026-10-12. Status: accepted.

## Context

The browser Bench must be fast, stable and load Chrome and Firefox Extensions, installed from the Store page with its own button. The options were: WKWebView with `WKWebExtension` (macOS 15.4), stock Chrome as a companion process driven by CDP with overlay windows, a fork of Chromium, a fork of Firefox.

| Option | Embedding | Extensions | Build and upkeep |
|---|---|---|---|
| WKWebView + WKWebExtension | real, in the Layout | Safari's API coverage; no blocking `webRequest`; Store install by the app | none; Apple ships the engine |
| Chrome + CDP overlay | fake; overlay windows break on Spaces, full screen, z-order | all | none, but the hack |
| Chromium fork | real through CALayerHost patch | all, no Sync | 100 GB build, permanent rebase |
| Firefox fork | real through a new CALayerHost patch | all, Sync works | 15 GB build, rebase per ESR |

Search (Office Commun, MIT) ships a complete Extension layer on WKWebExtension: Store install with CRX signature check, a Store page relay that replaces the install button, Shims for `bookmarks`, `history`, `downloads`, `sidePanel`, `offscreen`, `tabGroups`, `identity`, popups and permissions. About 8 800 lines.

## Decision

The browser Bench uses WKWebView through the mygo fork and `WKWebExtension`. The app ports Search's Extension layer to Go and reuses its Shim JavaScript as is, with Search's NOTICE kept. Minimum macOS is 15.4.

## Consequences

- No Chrome Sync. Extensions and settings live in the app's Profile.
- Extensions that need blocking `webRequest` or Chrome-only APIs do not load. `04-browser.md` lists the limits.
- The companion-Chrome path stays a fallback. It is not in the specification.
