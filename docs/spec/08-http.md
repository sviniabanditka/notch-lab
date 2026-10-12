# http Bench

The http Bench sends HTTP requests and shows their responses. Requests live in `.http` files that git can track.

> M7 status (2026-10-12). Done: `.http` files with an exact round trip (CRLF too), REST Client file variables, Environments with Keychain secrets, the Panel, the Request Tab, sending with timings, redirects, the cookie jar, streaming, large bodies, the history, curl import and export, and the MCP tools. Packages: `internal/httpfile` (format, variables, curl), `internal/httpclient` (sending, jar, history), `internal/keychain`, `internal/bench/http`. `[later]` in M7: the multipart table (R-HTTP-069: a multipart body edits as REST Client text, `< path` lines read files), syntax highlight of the body (R-HTTP-067: a plain monospace field), the enabled column of Params, drag between Collections (R-HTTP-045: "Move to" in the menu), fold arrows, search and XML indent of a pretty body (R-HTTP-094), the HTML preview (R-HTTP-095: images show), one block per event of an event stream (R-HTTP-098: the text shows as it arrives), "Open in editor" of a large body (R-HTTP-097), the Divider position in the Bench settings (R-HTTP-062: kept per Tab while the app runs).

## Storage

R-HTTP-001. An http Bench has a Bench folder. The setting `folder` in the Bench settings holds its path.

R-HTTP-002. When `folder` is empty, the Bench folder is `<effective root>/.http` when the Bench has an effective root folder other than the home folder. Otherwise the Bench folder is `$NOTCH_LAB_HOME/http/<bench id>/`.

R-HTTP-003. The app creates the Bench folder when the user creates the first Collection.

R-HTTP-004. One `.http` file in the Bench folder is one Collection. The file name without `.http` is the Collection's name.

R-HTTP-005. One request block in a file is one Request. Blocks are separated by a line that starts with `###`.

R-HTTP-006. The app watches the Bench folder. A change on disk updates the Panel within 500 ms. A change to a file whose Request is open in a Tab with unsaved edits shows a bar "File changed on disk" with "Reload" and "Keep mine".

R-HTTP-007. The app writes a file only when the user saves a Request (Cmd+S) or when the user creates, renames, deletes or duplicates a Request or a Collection. The app writes the whole file. The app keeps text it does not understand (comments, unknown lines) in place.

## File format

R-HTTP-010. The app reads and writes the VS Code REST Client format. One Request block has these parts in this order:

| Part | Form | Required |
|---|---|---|
| metadata lines | `# @name <name>`, `# @no-redirect`, `# @no-cookie-jar`, `# @var <name> = <value>`, `# @timeout <seconds>` | no |
| request line | `<METHOD> <url> HTTP/1.1` or `<METHOD> <url>` | yes |
| header lines | `<Name>: <value>`, one per line, directly after the request line | no |
| blank line | one empty line | only before a body |
| body | the lines up to the next `###` or the end of the file | no |

R-HTTP-011. A line that starts with `#` or `//` outside a body is a comment. The app keeps comments.

R-HTTP-012. A Request without `# @name` has the name `<METHOD> <url path>`, cut to 60 characters.

R-HTTP-013. A body line of the form `< <path>` reads the body from the file at `<path>`. A relative path is relative to the `.http` file. A file that does not exist is an error shown before Send.

R-HTTP-014. The request line can continue on the next lines when those lines start with whitespace and `?` or `&`. The app joins them to the URL.

R-HTTP-015. A `# @timeout 60` line sets the timeout of the Request in seconds. The default is 30 s.

R-HTTP-016. The app writes a Request in this form, with one space after the method, no `HTTP/1.1`, and one blank line between headers and body.

Acceptance: a file written by VS Code REST Client with 10 requests of every part above loads with 10 Requests and writes back without a change when the user saves without editing.

## Variables

R-HTTP-020. A Request can use `{{name}}` in the URL, in a header value, in the body and in `# @var` values.

R-HTTP-021. The app resolves a variable in this order. The first match wins.

| Order | Source |
|---|---|
| 1 | `# @var` lines of the Request |
| 2 | the file's variables: `@name = value` lines, as REST Client writes them |
| 3 | the active Environment |
| 4 | the `$shared` section of `http-client.env.json` |
| 5 | built-ins |

R-HTTP-022. Built-ins:

| Name | Value |
|---|---|
| `{{$guid}}` | a new UUID v4 |
| `{{$timestamp}}` | Unix seconds now |
| `{{$timestamp -1 d}}` | Unix seconds now plus the offset; units `ms`, `s`, `m`, `h`, `d`, `w`, `M`, `y` |
| `{{$datetime iso8601}}` | the time now in ISO 8601; also `rfc1123` |
| `{{$randomInt a b}}` | an integer from `a` to `b` inclusive |
| `{{$processEnv NAME}}` | the app's environment variable `NAME` |

R-HTTP-023. A variable that resolves to nothing stays as `{{name}}` in the sent text. The URL field shows it in the error color. Send shows a Toast "Unknown variable: name" and still sends.

R-HTTP-024. A variable value can hold `{{other}}`. The app resolves up to 10 levels. Deeper nesting is an error "Variable loop: name".

## Environments

R-HTTP-030. Environments live in `http-client.env.json` in the Bench folder. The format is a JSON object: each key is an Environment name, each value is an object of variable names to string values. The key `$shared` holds variables for every Environment.

R-HTTP-031. The Panel header has an Environment select. "No environment" is the first choice. The selected Environment is stored in the Bench settings as `environment`.

R-HTTP-032. A variable whose name starts with `secret.` is a secret. The file holds `"secret.token": ""`. The value lives in the Keychain under the service `dev.sviniabanditka.notch-lab` with the account `http/env/<environment>/<name>` (R-SHELL-165). The app talks to `/usr/bin/security` over its standard input, so a value never shows in a process's arguments.

R-HTTP-033. The Environment editor is a sheet over the window that the gear button of the Panel header opens. It shows variables of the selected Environment and of `$shared` in a table. A secret shows as dots with a "Set" button. Setting a secret writes the Keychain and writes the empty string to the file.

R-HTTP-034. The app never writes a secret value to a file, to the history, to the log or to an MCP result. A sent header or body that held a secret shows the secret as `••••` in the Response's request view.

R-HTTP-035. A missing `http-client.env.json` is not an error. The select shows "No environment" only.

## Panel

R-HTTP-040. The Panel shows a tree: Collections at the top level, Requests under them. Each Request row shows the method in its color and the name.

R-HTTP-041. Method colors: GET green, POST blue, PUT orange, PATCH yellow, DELETE red, HEAD and OPTIONS gray, other methods gray.

R-HTTP-042. The Panel header has a search field. The tree shows only Requests whose name, method or URL contains the query. A Collection with no matching Request hides.

R-HTTP-043. Panel actions:

| Action | Where | Effect |
|---|---|---|
| New Collection | header "+" | asks a name, creates `<name>.http` with no Requests |
| New Request | Collection context menu, or Cmd+T with a Collection selected | appends a `GET https://` block, opens it in a Tab |
| Rename | context menu, or Enter on a row | Collection: renames the file. Request: sets `# @name` |
| Delete | context menu, or Backspace | asks for confirmation; Collection: moves the file to the Trash; Request: removes the block |
| Duplicate | context menu, or Cmd+D in the Panel | appends a copy with " copy" added to the name |
| Import curl | header menu | see R-HTTP-050 |
| Reveal in Finder | Collection context menu | opens the folder |

R-HTTP-044. Cmd+T with no Collection selected creates the Request in the Collection of the focused Tab. With no Collection at all, Cmd+T creates the Collection "Requests" first.

R-HTTP-045. A Request can move to another Collection by drag in the tree. The app removes the block from one file and appends it to the other.

## Import and export

R-HTTP-050. Import curl shows a sheet with a text field. The user pastes a `curl` command line. The app parses it and creates a Request in the selected Collection.

R-HTTP-051. The curl parser understands:

| Option | Effect |
|---|---|
| `<url>` | the URL; the first non-option argument |
| `-X`, `--request` | the method |
| `-H`, `--header` | one header |
| `-d`, `--data`, `--data-raw`, `--data-binary`, `--data-urlencoded` | the body; method becomes POST when not set; several `-d` join with `&` |
| `--json` | the body, plus `Content-Type: application/json` and `Accept: application/json` |
| `-u`, `--user` | basic auth |
| `-F`, `--form` | one multipart field; `name=@path` is a file field |
| `-L`, `--location` | follow redirects on |
| `-k`, `--insecure` | verify TLS off |
| `-G`, `--get` | moves `-d` data to the query string |
| `--url` | the URL |

R-HTTP-052. The parser handles single quotes, double quotes, `\` line continuations and `$'...'` strings as a POSIX shell does. An option the parser does not know is ignored and listed in a Toast "Ignored: -x, --foo".

R-HTTP-053. Cmd+Shift+C in a Request Tab copies the Request as a curl command with variables resolved. A secret shows as `$SECRET_NAME` in the copied text, not its value.

## Request Tab

R-HTTP-060. A Tab holds one Request. The Tab's type data is `{"file": "<path>", "name": "<request name>", "index": <block index>}`. The title is the Request name.

R-HTTP-061. At restore, a Tab whose file or block no longer exists shows "Request not found" with "Close".

R-HTTP-062. The Tab layout from top to bottom: the request form, the request detail tabs, a Divider, the Response. The Divider's position is stored in the Bench settings as `responseRatio`, default 0.5.

R-HTTP-063. The request form has: a method select (GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS, custom text), a URL field, a Send button, a Cancel button that shows while a Request runs.

R-HTTP-064. The URL field highlights `{{name}}` in the accent color when the variable resolves, in the error color when it does not. Hovering a variable shows its resolved value in a tooltip; a secret shows `••••`.

R-HTTP-065. Cmd+Enter sends. Esc while the Request runs cancels. Cmd+S saves the Request to its file.

R-HTTP-066. The request detail tabs:

| Tab | Content |
|---|---|
| Params | a key, value, enabled table built from the URL's query string; an edit rewrites the URL; an edit in the URL rewrites the table |
| Headers | a key, value, enabled table; disabled rows are kept as `# <Name>: <value>` lines in the file |
| Body | a type select: none, raw, form-urlencoded, multipart, binary |
| Auth | a type select: none, bearer, basic, api-key; inherit from Collection `[later]` |
| Settings | follow redirects (default on), timeout in seconds (default 30), verify TLS (default on), cookie jar (default on) |

R-HTTP-067. Raw body has a language select: json, xml, text, html. The body edits in the native editor with that language's highlight. The app sets `Content-Type` when the Request has none: `application/json`, `application/xml`, `text/plain`, `text/html`.

R-HTTP-068. Form-urlencoded body is a key, value, enabled table. The app writes it as `a=1&b=2` with percent encoding and sets `Content-Type: application/x-www-form-urlencoded`.

R-HTTP-069. Multipart body is a table of rows. A row is a text field or a file field. A file field holds a path; the app shows the file name and size. The app writes multipart with a random boundary and sets `Content-Type`.

R-HTTP-070. Binary body holds one file path. The app sends the file's bytes and sets `Content-Type` from the file's extension when the Request has none.

R-HTTP-071. Auth writes a header before Send and does not store the header in the file:

| Type | Header |
|---|---|
| bearer | `Authorization: Bearer <token>` |
| basic | `Authorization: Basic <base64 user:password>` |
| api-key | `<Key name>: <value>` as a header, or `?<key>=<value>` in the query |

Auth values are stored in the file as metadata lines `# @auth bearer {{secret.token}}`, `# @auth basic <user> <password>`, `# @auth apikey header <name> <value>`. A literal password in `# @auth basic` is allowed but the app shows a hint "Use a secret variable".

R-HTTP-072. Settings are stored as metadata lines: `# @no-redirect`, `# @no-verify-tls`, `# @no-cookie-jar`, `# @timeout <s>`.

R-HTTP-073. A body of a GET or HEAD Request is sent as written. The app shows a hint "Most servers ignore a GET body".

## Sending

R-HTTP-080. Send resolves variables, builds the request, and runs it with `net/http` and a `http.Client` of the Bench. The client uses the system proxy settings (`http.ProxyFromEnvironment` and the macOS proxy configuration).

R-HTTP-081. The timeout covers the whole exchange from dial to the last body byte. A timeout shows "Timed out after 30 s" in the Response's status place.

R-HTTP-082. With follow redirects on, the app follows up to 10 redirects and shows each hop in the Timeline. With follow redirects off, the app shows the 3xx Response.

R-HTTP-083. The cookie jar is one `net/http/cookiejar` per Bench, saved to `$NOTCH_LAB_HOME/http/cookies/<bench id>.json` after every Response, loaded at start. The file keeps each site's cookies by name and value, with path `/` and 30 days to live. A Request with `# @no-cookie-jar` sends no cookies and stores none.

R-HTTP-084. The Panel header menu has "Clear cookies". The app asks for confirmation.

R-HTTP-085. Verify TLS off sets `InsecureSkipVerify`. The Response shows a warning icon "TLS not verified".

R-HTTP-086. The app records timings with `net/http/httptrace`: DNS start and end, connect start and end, TLS start and end, first response byte, last body byte. The Timeline tab shows them as a bar per phase with milliseconds.

R-HTTP-087. One Request Tab runs at most one request at a time. Send while a request runs does nothing. Cancel stops the request through its context.

R-HTTP-088. Client certificates `[later]`. WebSocket, GraphQL and gRPC `[later]`. Pre-request scripts and test scripts are not planned.

## Response

R-HTTP-090. The Response area shows a status line: the status code and text in a color (2xx green, 3xx blue, 4xx orange, 5xx red, error red), the time in ms, the body size in bytes or KB or MB, the HTTP version.

R-HTTP-091. Before the first Send the Response area shows "Send a request (Cmd+Enter)". While a request runs, the status line shows a spinner and the time counting.

R-HTTP-092. A network error (no DNS, connection refused, TLS failure) shows the Go error text in the status line in red. The Body tab shows the full error.

R-HTTP-093. Response tabs:

| Tab | Content |
|---|---|
| Body | the body; a format select: pretty, raw, preview |
| Headers | a name, value table; a click copies the value |
| Cookies | cookies the Response set: name, value, domain, path, expires, flags |
| Timeline | phases from R-HTTP-086; redirect hops with their status |
| Request | the request as sent: the request line, headers, the body; secrets as `••••` |

R-HTTP-094. Pretty format applies to `application/json` and `+json` bodies: indentation of 2 spaces, fold arrows on objects and arrays, a search field (Cmd+F) that highlights matches and walks them with Enter. XML bodies pretty-print with indentation. Other bodies show as raw.

R-HTTP-095. Preview format shows `text/html` bodies in a Web view with JavaScript off and no network, and image bodies as an image. Other bodies show as raw.

R-HTTP-096. The Body tab has "Copy" (the body text) and "Save" (a file save sheet; the default name comes from the URL's last path part and the content type).

R-HTTP-097. A body over 10 MiB is written to `$NOTCH_LAB_HOME/http/tmp/<guid>` as it arrives. The Body tab shows the first 1 MiB and a bar "10.4 MB body. Showing the first 1 MB." with "Save" and "Open in editor". The app deletes tmp files older than 1 day at start.

R-HTTP-098. A `text/event-stream` body or a chunked body without `Content-Length` renders as it arrives. The Body tab appends each chunk within 50 ms of receipt. The status line shows "Streaming" and a Stop button. Stop closes the body and keeps what arrived. Event-stream bodies show one block per event with `event`, `id` and `data`.

R-HTTP-099. A Response that arrives while the Tab is not on screen marks the Tab Unread.

## History

R-HTTP-100. The app appends every Response to `$NOTCH_LAB_HOME/http/history/<hash>.jsonl`. `<hash>` is the SHA-256 of the file path and the Request name, in hex. One line holds: timestamp, status, time in ms, size, the request line, headers, and the body up to 256 KB. Secrets are replaced by `••••` before the write.

R-HTTP-101. The app keeps the last 50 lines per file. The 51st Response removes the oldest line.

R-HTTP-102. The Response area has a "History" button. It shows the list with timestamp, status, time, size. A click shows that Response in the Response area with a bar "From history, <timestamp>" and "Back to latest".

R-HTTP-103. The Panel header menu has "Clear history" for the Bench. The app asks for confirmation.

## Shortcuts

| Shortcut | Command |
|---|---|
| Cmd+Enter | Send |
| Esc | Cancel |
| Cmd+S | Save Request |
| Cmd+T | New Request |
| Cmd+Shift+C | Copy as curl |
| Cmd+F | Search in the Response body |
| Cmd+L | Focus the URL field |
| Cmd+1 to Cmd+5 | the request detail tab 1 to 5 |

## MCP tools, domain `http`

R-HTTP-110. Tools and their Access level:

| Tool | Input | Output | Level |
|---|---|---|---|
| `http.collections.list` | `{bench?}` | Collections with file paths and Request counts | Read |
| `http.requests.list` | `{bench?, collection?}` | Requests: id, name, method, URL (variables not resolved) | Read |
| `http.request.get` | `{id}` | the Request's parts as written, secrets as `••••` | Read |
| `http.request.send` | `{id, overrides?: {url?, headers?, body?, environment?}}` | the Response: status, time, size, headers, body up to 256 KB, truncated flag | Full |
| `http.send` | `{method, url, headers?, body?, timeout?}` | the same as above | Full |
| `http.response.last` | `{id}` | the last Response of that Request from history | Read |

R-HTTP-111. A Request id is `<file path>#<block index>`.

R-HTTP-112. `http.request.send` and `http.send` open or focus the Request in a Tab, run it there, and return when the Response completes or the timeout passes. A streaming Response returns after 30 s with what arrived and `streaming: true`.

R-HTTP-113. `http.send` creates a Tab in the Client's ad-hoc Collection "Agent" of the focused http Bench. The Client owns that Tab.

R-HTTP-114. No tool returns a secret value. A header or body that held a secret shows `••••`.

R-HTTP-115. Every Full tool call shows a Toast "<client name> sent GET https://…".
