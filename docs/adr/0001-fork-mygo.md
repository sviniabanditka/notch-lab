# 0001. Use a fork of mygo

Date: 2026-10-12. Status: accepted.

## Context

mygo 0.4.5 draws native UI in pure Go and embeds a system WKWebView in a native window. mygo has no non-activating panel, no floating window level, no window position API, no Dock hiding, no WKWebExtension, no data store id for a web view. The notch panel and the browser Extensions need these.

## Decision

The app builds against `github.com/sviniabanditka/mygo`, branch `notch-lab`, through a `replace` directive in `go.mod`. The fork tracks upstream tag 0.4.5. Every patch is one commit with the prefix `notch:`. The fork sends no pull requests upstream. `docs/spec/14-mygo-fork.md` lists every patch.

## Consequences

- A new upstream tag needs a rebase of the `notch:` commits. The procedure is in `14-mygo-fork.md`.
- The app owns every ObjC call the patches add. The patches use purego only, no cgo, as upstream does.
