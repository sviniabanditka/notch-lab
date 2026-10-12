# 0004. Lab, Bench, Tab; the Layout belongs to the Bench

Date: 2026-10-12. Status: accepted.

## Context

The first idea was typed tabs. Typed tabs make a flat list of mixed items and spread type-specific panels over the window. The reference (muxy-style) has a workspace menu, a vertical rail and a tab list.

## Decision

Three levels: Lab (menu), Bench (rail, typed), Tab (sidebar list). A Bench has exactly one type and owns a Layout of Splits whose Leaves show the Bench's own Tabs. There is no mixed-type Split. Names: "Lab" and "Bench" so that the word "project" never collides with a folder on disk.

## Consequences

- A terminal next to an editor is two Benches side by side in the rail, not one Split. The user switches with Cmd+Ctrl+] or the Switcher.
- The Split code is one package used by every Bench type.
- `[later]` A window-level Split of two Benches can be added without changing the model.
