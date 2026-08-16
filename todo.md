- [x] load, support and browse envinronments.
- [x] better env browing
- [x] support varialbles form both collections and envinronments
- [x] :i to get info on both folders and requests.
- [x] better serach
- [x] better topbar , one right collum with context clues, loaded collection , loaded env etc . middle bar with help stuff. working right bar with functioning logo
- [x] edit request!
- [x] history
- [x] sessions
- [x] debug mode
- [x] scripts (pre-request and test scripts with barebones pm API)
  - [x] goja JavaScript runtime integration
  - [x] pm.test() for defining tests
  - [x] pm.response.to.have.status() for status assertions
  - [x] pm.collectionVariables.set() and pm.environmentVariables.set()
  - [x] pm.collectionVariables.get() and pm.environmentVariables.get()
  - [x] pm.variables.get() and pm.variables.set() with precedence (env > collection)
  - [x] responseBody global variable
  - [x] Pre-request scripts (executed before HTTP request)
  - [x] Test scripts (executed after HTTP response)
  - [x] Script inspection in Info mode (both pre-request and test)
  - [x] Test results display in Response mode
  - [x] pm.response.json() for parsing JSON responses
  - [x] Script timeouts (5 second default, configurable)
  - [x] Variable persistence (auto-save collection/environment after script execution)
  - [x] Script editing in TUI (future)
- [x] ingest openapi
- [x] makefile
- [x] ingest openapi
- [x] workflow files
- xx] rethinking hotkeys (ctrl e for editing, maye edit json)
- [x] file browsing and completion

- [x] add requests
- [x] ctrl c and ctrl v add reqeusts and endpoints /
- [x] :h should display all keyboard shortcuts grouped by context
- [x] ? should display shortcuts in the current cotnext

## Feature ideas from ChatGPT (roadmap brainstorm)

You already have most of the obvious API-client surface area. I'd focus next on features that make PostOffice feel **faster than Postman**, rather than reproducing more Postman features.

### High-value additions

1. **Request history + instant rerun.** Keep a chronological history of executions with request, resolved variables, response status, duration, and timestamp. Something like `:history`, then Enter to rerun. This would be one of my first additions.

2. **Response diffing.** Select two executions/saved responses and diff JSON, headers, status, and timing. Especially useful after editing a request or switching environments.

3. **Collection-wide fuzzy finder.** A global `Ctrl-P`/`/` style picker for requests, folders, workflows, environments, and commands. Search by things like `POST users`, `grpc auth`, or `folder/request`. Your existing search could evolve into this.

4. **Command palette.** `:` commands are great when you know them. Add fuzzy command discovery for when you don't: `:del` → delete request, delete folder, delete environment variable, etc., with descriptions and shortcuts.

5. **Variable resolution inspector.** Put the cursor on `{{token}}` and show where its current value comes from:
   `request → collection → environment → global`, including overridden values. API clients become painful when variable scopes get complicated.

6. **Resolved-request preview.** A toggle that shows the *actual* URL, headers, query params, and body about to be sent after variable/substitution/script processing. Bonus points for highlighting what changed.

7. **Request timing breakdown.** DNS, connect, TLS, TTFB, download, total. It makes the response viewer useful for debugging performance rather than merely inspecting payloads.

8. **Replay with modifications.** From history, clone an execution into a temporary request, tweak something, send it, then discard it without touching the collection.

### Features that fit a terminal particularly well

A **watch mode** could be excellent:

```text
:watch 5s
```

Repeatedly execute the selected request and show status, latency, response changes, and failures. This could become a lightweight endpoint monitor while developing.

I'd also consider **request chaining without formal workflows**. For example, run login, extract `.token`, then immediately use it in another request. A small extraction syntax such as `token = $.access_token` would cover a lot of everyday API testing without requiring users to construct a workflow.

Another strong terminal-native feature is **pipes / shell integration**. Things like sending the response through `jq`, opening JSON in `$PAGER`, exporting a request to stdout, or reading a body from stdin would make PostOffice compose naturally with Unix tooling.

### Testing would unlock a lot

You already have pre/post scripts, so assertions feel like a natural next step:

```javascript
expect(response.status).toBe(200)
expect(response.json.user.id).toExist()
```

Then show something compact:

```text
POST /login       200   143ms   ✓ 4
GET  /profile     200    82ms   ✓ 7
GET  /permissions 403    91ms   ✗ 1/5
```

That leads naturally into collection/workflow test runs, failure navigation, exit codes for CI, JUnit/JSON output, and eventually `postoffice run collection.json --env staging`. **Headless execution** in particular could turn this from "a Postman TUI" into a useful developer tool beyond the TUI.

### Small QoL features with disproportionate payoff

I'd look at these too:

* Bookmark/favorite requests and workflows.
* Recently used requests.
* Pin multiple responses for comparison.
* Pretty/raw/headers/body split views.
* Toggle line wrapping.
* JSON folding.
* Jump to JSON path.
* Copy JSON path under cursor.
* Response-body search with next/previous matches.
* Automatic syntax highlighting based on content type.
* Human-readable and raw byte sizes.
* Show elapsed request time continuously while a slow request runs.
* Cancel an in-flight request.
* Configurable request timeout.
* Retry failed requests.
* Configurable retry/backoff policies.
* Follow/don't-follow redirects.
* SSL verification toggle and custom CA support.
* Proxy configuration.
* Cookie jar inspection/editing.
* Auth helpers for Bearer, Basic, API key, OAuth2 and client credentials.
* Secret masking so tokens don't accidentally appear in logs/history.
* Undo/redo for collection edits.
* Multi-select requests for move/delete/tag/export/run.
* Drag-equivalent keyboard commands for moving requests between folders.
* Rename anything with one consistent binding.
* Configurable keybindings.
* A `:checkhealth`/`:doctor` command for config, dependencies and filesystem problems.

One subtle improvement: make **every destructive operation undoable**. In a keyboard-driven UI, `dd` being instant is wonderful until the user's finger slips. "Deleted request — `u` to undo" is much nicer than confirmation dialogs everywhere.

### One feature I'd seriously consider differentiating around

Make **execution state first-class**.

Instead of the tree only representing Postman collections, annotate it with what happened:

```text
▼ Auth
  ✓ POST Login              200   121ms
  ✓ GET  Current User       200    44ms
▼ Orders
  ✓ GET  List Orders        200    83ms
  ✗ POST Create Order       422    71ms
  · DELETE Order
```

Now the collection tree doubles as a live debugging dashboard. Run a folder and immediately see failures. Run it again and show latency deltas. Switch environments and rerun. Jump directly to the failing assertion/response.

That feels very **k9s-like** and gives PostOffice more of its own identity.

If I were prioritizing the roadmap, I'd probably go **history → fuzzy picker → resolved-variable inspector → assertions → headless/CI runner → response diff → execution-state tree → watch mode**. Those build on each other and move PostOffice toward being something I'd choose *because* it's a TUI, rather than despite being one.
