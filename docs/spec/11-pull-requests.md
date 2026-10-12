# Pull-requests Bench

The pull-requests Bench shows the GitHub pull requests and issues that involve the user, and one of them in detail. The package is a port of herdr-go `internal/pullrequests` (MIT). Only GitHub is supported.

> M11 status (2026-10-12). Done: the package ported from herdr-go (`internal/pullrequests`, its tests pass), the Panel as its list, a Tab as its detail, the MCP tools; checked live against GitHub through `gh`. Changed from the text below, to keep the port's behaviour: the detail has Overview (with the checks) and Files (R-PR-050); the token comes from `gh auth token` found on PATH, `/opt/homebrew/bin` or `/usr/local/bin` (R-PR-010); the cache is `$NOTCH_LAB_HOME/pullrequests/cache/pullrequests.json` (R-PR-040); one detail shows per Bench at a time, a second Leaf shows "Show here"; "Send review comments to agent" pastes into the Lab's first terminal Tab with an Agent (R-PR-067 is done); Open in browser uses the system browser. `[later]`: the lightbox for images (R-PR-053), Checkout in a terminal Tab (R-PR-060), approve and comment (R-PR-068).

## Tab and Panel

R-PR-001. A Tab holds one Pull request or one Issue. The Tab's type data is `{"repo":"owner/name","number":123,"issue":false}`.

R-PR-002. The Panel is the list: a Review scope control, a search field, a "This repo" toggle and the rows.

R-PR-003. "New Tab" (Cmd+T) in a pull-requests Bench moves keyboard focus to the search field of the Panel. The Bench does not create an empty Tab.

R-PR-004. Selecting a row in the Panel opens the row in the focused Leaf. The app reuses the Tab of the same repo and number when one exists in the Bench.

R-PR-005. At restore, a Tab shows its cached detail at once, when the cache has one, and loads the detail again in the background. A Tab whose pull request GitHub does not return any more shows "Pull request not found" and a "Close" button.

R-PR-006. The Tab title is `#<number> <title>`. The sidebar shows the repo name below the title when "This repo" is off.

## Sign-in

R-PR-010. The app reads the GitHub token with `gh auth token` through `/bin/zsh -lic`. The app reads the token at the first use and again after each HTTP 401.

R-PR-011. When `gh` is absent or returns no token, the Panel shows "Run `gh auth login` in a terminal" and a "Retry" button. No request goes to GitHub.

R-PR-012. The token stays in memory only. The app does not write the token to State, to the log or to a Tool result.

R-PR-013. The app sends every request to `https://api.github.com` with `Authorization: Bearer <token>` and `User-Agent: notch-lab/<version>`.

## Review scope

R-PR-020. The Review scopes and their GitHub search queries:

| Scope | Query |
|---|---|
| Review | `is:open is:pr review-requested:@me` |
| Mine | `is:open is:pr author:@me` |
| Involved | `is:open is:pr involves:@me -author:@me -review-requested:@me` |
| Issues | `is:open is:issue assignee:@me` OR `is:open is:issue author:@me` |
| Search | the text the user typed, with `is:pr` or `is:issue` as the user wrote it |

R-PR-021. "This repo" adds `repo:owner/name` to each query. The repo is the one the Bench's effective root folder's `origin` remote names. The app reads it with `git -C <root> remote get-url origin` and accepts `https://github.com/o/n(.git)` and `git@github.com:o/n(.git)`.

R-PR-022. "This repo" is disabled with the tooltip "No GitHub remote in <root>" when the root folder has no GitHub origin.

R-PR-023. The selected scope and the "This repo" state are Bench settings, saved in State.

R-PR-024. The Search scope runs the query when the user presses Enter in the search field. Typing without Enter filters the rows of the selected scope by title, repo and number.

R-PR-025. The list shows at most 100 rows per scope. The app requests pages of 50 with GraphQL `search(type: ISSUE)`.

## Rows

R-PR-030. A row shows: the repo (when "This repo" is off), the number, the title, the author avatar, the time since the last update, the draft marker, and the checks summary.

R-PR-031. The checks summary is one icon: green when every check passed, red when one failed, yellow when one runs, grey when there are no checks. The icon carries the counts as a tooltip: "3 passed, 1 failed, 2 running".

R-PR-032. A row is Unread when the pull request has a comment, a review or a check result newer than the time the user last showed its Tab. The row shows a dot. Showing the Tab clears it.

R-PR-033. Rows sort by last update, newest first.

R-PR-034. Up and Down move the selection in the list. Enter opens the selected row in the focused Leaf. Cmd+Enter opens it in a new Split.

## Cache and refresh

R-PR-040. The app keeps every loaded list and detail in memory and in `$NOTCH_LAB_HOME/pullrequests/<account>.json`. The app writes the file at most once per 5 s.

R-PR-041. At open the Bench shows the cached lists at once and refreshes them in the background.

R-PR-042. The Bench refreshes the lists every 60 s while the Bench is on screen and the window has focus. The Bench refreshes the detail of the shown Tab every 60 s. The Bench refreshes the checks of the shown Tab every 30 s while one check runs.

R-PR-043. Cmd+R refreshes the lists and the shown detail at once.

R-PR-044. The app keeps the rate-limit headers of every response. When fewer than 100 core requests remain, the app stops the timed refresh until the reset time and shows "GitHub rate limit; refresh at <time>" in the Panel. Cmd+R still runs.

R-PR-045. A request that fails with a network error keeps the cached data and shows the error in the Panel's footer. The next timed refresh retries.

R-PR-046. Freshness: a list shows from cache for any age. A detail shows from cache up to 10 minutes and loads again after. A diff shows from cache up to 1 hour.

## Detail

R-PR-050. A Tab has three views, selected by a segmented control at the top: Overview, Checks, Files. Cmd+1, Cmd+2, Cmd+3 select them while the Tab has Focus. The selected view is saved in the Tab's type data.

R-PR-051. The header shows: title, number, state (open, draft, merged, closed), author, base and head branch, the labels, the counts of additions and deletions, and the buttons Checkout, Merge, Open in browser, Copy link.

R-PR-052. Overview shows the body as rendered markdown with images, then the comments and reviews in time order. Each comment shows author, time and body. Review threads show the file path and line. Bot comments fold under "n bot comments". Resolved threads fold under "n resolved threads".

R-PR-053. Images in bodies and comments load through the token for `user-attachments` URLs. An image that fails to load shows as a link. A click on an image opens it in the lightbox.

R-PR-054. Checks shows every check run and status context of the head commit: name, state, duration, and a "Logs" link that opens the check's `detailsUrl` in a browser Bench of the Lab.

R-PR-055. Files shows a tree of the changed files with per-file additions and deletions. Selecting a file shows its diff in the diffview below the tree. A file with no patch (binary, or over GitHub's size limit) shows "No diff".

R-PR-056. The diff loads from `GET /repos/{o}/{n}/pulls/{num}/files`, 100 per page, up to 3000 files. More files show "GitHub returns at most 3000 files".

R-PR-057. An Issue Tab shows Overview only: labels, assignees, body and comments. Issues are read-only.

## Actions

R-PR-060. Checkout creates a terminal Tab in the first terminal Bench of the Lab (the app creates a terminal Bench when the Lab has none) with the Lab's root folder as the current folder and runs:

```
git fetch origin <head-ref> && git checkout <head-branch>
```

For a pull request from a fork the command is:

```
git fetch origin pull/<number>/head:<head-branch> && git checkout <head-branch>
```

R-PR-061. Checkout is disabled with the tooltip "Root folder is not this repo" when the Bench's effective root folder's origin is not the pull request's repo.

R-PR-062. Merge opens a sheet with the method (merge, squash, rebase; only the methods the repo allows), the commit title and body (for merge and squash), and a "Delete branch after merge" toggle. The sheet's button reads "Merge #<number>". The app remembers the method last used per repo.

R-PR-063. Merge is disabled when the pull request is a draft, is closed, has a failed required check, or has conflicts. The tooltip states the reason.

R-PR-064. After a merge the Tab reloads its detail and the lists refresh. A merge that fails shows GitHub's message in a Toast and in the sheet.

R-PR-065. Open in browser opens the pull request's `html_url` in a browser Bench of the Lab (the app creates one when the Lab has none) in a new Tab.

R-PR-066. Copy link puts the `html_url` on the clipboard and shows the Toast "Link copied".

R-PR-067. "Send to agent" writes the unresolved review threads as a prompt (file, line, comments in order) and opens the Task sheet (`12-agents.md`) with that prompt. `[later]`

R-PR-068. Approve, request changes and comment from the app. `[later]`

## Layout

R-PR-070. At a Leaf width under 520 pt the detail header folds the branch line and shows the buttons in a menu.

R-PR-071. The Panel's list has a minimum width of 240 pt in the sidebar.

## MCP tools, domain `pull-requests`

R-PR-080. Every tool in the domain is Read level. The Bench id is optional in each call: the app uses the first pull-requests Bench of the selected Lab when it is absent.

| Tool | Params | Result |
|---|---|---|
| `pull_requests.list` | `scope: "review"\|"mine"\|"involved"\|"issues"`, `this_repo?: bool`, `bench?: string` | `[{repo, number, title, author, updated_at, draft, checks: {passed, failed, running}, unread}]` |
| `pull_requests.get` | `repo: string`, `number: int` | `{repo, number, title, state, author, base, head, body, labels, additions, deletions, html_url, mergeable}` |
| `pull_requests.diff` | `repo`, `number`, `path?: string` | `{files: [{path, additions, deletions, patch}]}` (one file when `path` is set; patches cut at 256 KB each, the result says `truncated: true`) |
| `pull_requests.checks` | `repo`, `number` | `[{name, state, conclusion, duration_ms, details_url}]` |
| `pull_requests.comments` | `repo`, `number`, `unresolved_only?: bool` | `[{author, created_at, body, path?, line?, resolved?}]` |
| `pull_requests.search` | `query: string` | as `list` | 

R-PR-081. A tool that needs a token when the app has none fails with code `-32010` and the message "GitHub sign-in needed; run `gh auth login`".

## Tests

R-PR-090. Unit tests from herdr-go port with the package: checks summary, cache freshness, remote parsing, merge rules, `forAgent`, rate limit. New: the Checkout command for a fork and for a branch of the same repo.

R-PR-091. Acceptance: with a `gh` token, open a pull-requests Bench; the list shows within 2 s from cache or within 5 s from GitHub; select a row; the Tab shows the Overview; Cmd+3 shows the Files; select a file; the diff shows.
