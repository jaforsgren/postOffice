# PostOffice

A terminal UI for Postman collections.

Browse, edit, and execute HTTP and gRPC requests. Vim-style navigation. Built in Go.

## Features

- Vim-style navigation and search.
- HTTP and gRPC request execution.
- Postman collections and environments.
- Request editing and variable autocomplete.
- Pre-request scripts and tests.
- JavaScript workflow automation.
- Data-driven collection runs.
- Headless CLI for CI pipelines.
- JUnit and JSON reports.
- Collection and environment merging.
- Persistent sessions and saved responses.

## Installation

Build from source.

```
go build -o postOffice
./postOffice
```

Enable debug logging.

```
./postOffice --log debug.log
```

## Navigation

### Lists

```
j/k       Move up/down
g/G       First/last item
enter     Open or execute
h/esc     Go back
/         Search (collections, requests, environments, variables, workflows)
i         Info (also on a collection)
|         Toggle split view
ctrl+o    Jump back (previous folder or collection, also from other lists)
ctrl+i    Jump forward (also tab)
O         Reveal the collection/environment file (Finder/Explorer)
r         Refresh
ctrl+n    Next environment
1-4       Collections / Requests / Environments / Variables
?         Help
q         Quit
```

### Requests

```
e         Edit request
E         Edit scripts
d         Delete
D         Duplicate
i         Request info + response
J         Raw JSON
H         Saved responses
V         Edit used variables
ctrl+e    Re-execute / run folder
ctrl+x    Cancel the request being sent (HTTP and gRPC)
z         Expand/collapse folder in place
Z         Expand/collapse all folders
.         Repeat last action (execute, delete, duplicate, favorite, expand)
u         Undo last saved change
ctrl+r    Redo
```

### Responses

```
j/k       Scroll
g/G       Top/bottom
d/u       Half-page scroll
ctrl+e    Resend request
t         Timing breakdown
s         Save response
ya        Copy request + response
yb        Copy response body
yr        Copy as curl/grpcurl
yy        Copy resolved URL
o         Open Markdown report
```

## Commands

Press `:` to enter command mode.

```
:l <path>       Load collection
:le <path>      Load environment
:c              Collections
:r              Requests
:env            Environments
:var            Variables
:edit           Edit request
:wf             Workflows
:ro             Run options
:changes        Unsaved changes
:session        Restore the last session
:hist           Request history
:t              Timing breakdown (response view)
:w              Save
:wq             Save and quit
:q              Quit
:h              Help
```

Collections and environments are restored on startup.

Paths support `~/` expansion.

```
:load ~/postman/api.json
```

## Request History

Every execution started from the TUI (HTTP, gRPC, Service Bus and AMQP, including replays) is recorded with its resolved URL/target, headers and body, environment name, status, duration, timestamp and response size. Workflow runs are not recorded.

- Open it with `ctrl+y` or `:history` (`:hist`, `:hi`)
- `enter` replays the entry with exactly the recorded resolved inputs. Variables are not re-resolved and pre-request/test scripts do not run
- `=` shows a line diff (`-` history entry, `+` current response) of status, sorted headers and pretty-printed body

History is persisted to `~/.postoffice_history.json` (capped at the 200 most recent entries). Because entries contain resolved secrets such as auth headers and connection strings, the file is written with owner-only permissions (`0600`).

### Timing Breakdown

Press `t` (or run `:timing`) in the response view to open a waterfall popup of the request's phases:

- **HTTP** — DNS lookup, TCP connect, TLS handshake, TTFB and content download. On a reused connection, DNS/TCP/TLS show as `reused`; phases that don't apply (DNS for an IP address, TLS for plain HTTP) show as `n/a`.
- **gRPC** (workflow steps) — connect + reflection, invoke
- **AMQP** — dial (DNS + TCP), handshake (TLS + AMQP), open channel, publish
- **Azure Service Bus** — total only

Phases the request never reached because it failed are left out. Press `t`, `q` or `esc` to close.

## Editing Requests

Select a request and press `e`.

Edit the name, method, URL, path parameters, headers, or body.

```
j/k       Select field
enter     Edit / save single-line
ctrl+s    Save multi-line
esc       Cancel / exit
:w        Write changes to file
```

Path parameters such as `/users/:id` have dedicated fields.

Values persist during the session.

Type `{{` for variable autocomplete.

Press `Tab` to insert or cycle through suggestions. Resolved values are shown in the editor.

Use `V` to edit a request's variables at their source.

A variable can hold a list of choices (`"choices": ["01", "02"]` next to its `value` in the environment or collection file). Its `value` is the chosen one. In the `V` panel, `enter` opens a dropdown to pick another choice, `a` adds a choice (turning a plain variable into a choice list) and `d` deletes one. Reference a specific choice with `{{persona_id[1]}}` (zero-based).

Press `u` to undo the last saved change (delete, duplicate, add, edit, rename, editor edits and variable values) and `ctrl+r` to redo it, in the collections, requests and environments views. Undo restores the file, so save or discard unsaved edits first.

Unsaved edits are marked with `*` on the request, every folder above it and its collection (and on edited environments).

Use `:changes` to review unsaved edits. Press `d` to discard one or `ctrl+d` to discard all.

## Scripts and Tests

Postman-style scripts run before and after requests.

Execution order:

1. Collection scripts.
2. Parent folder scripts.
3. Request scripts.

Scripts share a sandbox. Implicit globals can be reused across scripts.

Supported APIs include:

- `pm.test` and `pm.expect` with Chai-style assertions.
- `pm.request` and `pm.response`.
- `pm.variables`, `pm.environment`, `pm.collectionVariables`.
- `pm.iterationData` and `pm.info`.
- `pm.sendRequest` with callbacks or async/await.
- `pm.execution.skipRequest()`.
- `console`, timers, `CryptoJS`, `atob`, `btoa`.

Pre-request changes affect the outgoing request.

`pm.execution.setNextRequest()` works in headless collection runs. TUI workflows execute in defined order.

Synchronous execution is limited to 5 seconds per script or callback. Async operations may take up to 2 minutes in total.

## Workflows

Automate request sequences with JavaScript.

Each workflow is a YAML file in `workflows/` beside its collection.

```
id: login-and-fetch
name: Login and Fetch Users
version: 1

steps:
  - id: login
    request: Auth/Login
  - id: users
    request: Users/List Users

script: |
  export default async function (wf, pm) {
    const login = await wf.run("login");

    if (login.response.code !== 201) {
      wf.fail("Login failed");
      return;
    }

    const users = await wf.run("users");
    console.log(users.response.json());
  }
```

### Run a folder

Select a folder and press `ctrl+e`.

Confirm with `y`.

Requests execute in collection order, including subfolders.

The first run creates a workflow YAML file. Later runs reuse it.

Unedited workflows can regenerate when folder contents change. Hand-edited workflows are preserved.

### Workflow controls

```
:wf             Browse workflows
:wf new <id>    Create workflow
:wf run <id>    Run workflow

a               Add step
e/E             Edit request/scripts
<               Run up to step
>               Run from step
x               Run step
R, ctrl+e       Run all
.               Repeat the last partial run
r               Reload workflows from disk
enter           Step response
|               Step panel ([ / ] scroll it)
i               Step info
ctrl+d          Delete step
o               Open run report
```

Reports open in `$VISUAL`, `$EDITOR`, or `nvim`.

Vim folding is supported.

### Run options

Configure with `:runopts` or `:ro`.

```
:ro stop on
:ro stop off
:ro iterations 5
:ro env staging
:ro env active
:ro data users.csv
:ro data off
:ro reset
```

Supports CSV and JSON data files.

Data values override environment and collection variables for each iteration.

Variable changes carry over between iterations.

Run summaries show passed, failed, and skipped steps and tests.

See docs/workflows.md for details.

## gRPC

Execute unary gRPC requests directly from Postman collections.

Supports:

- Server reflection.
- Protobuf descriptor sets.
- TLS and insecure connections.
- Metadata and binary headers.
- Variable substitution.
- Pre-request scripts and tests.
- Workflow execution.

### Editing

Press `e` on a gRPC request.

Fields include endpoint, service/method, metadata, JSON message, and TLS.

```
j/k       Select field
enter     Edit / toggle TLS
ctrl+s    Save multiline field
ctrl+f    Browse server reflection
esc       Back
```

Reflection lists services and methods. Selecting a method fills its path and generates a JSON message template.

Reflection browsing uses an insecure connection. TLS services require manual method entry.

### Without reflection

Generate a descriptor set.

```
protoc -I protos --include_imports \
  --descriptor_set_out=services.protoset \
  $(find protos -name '*.proto')
```

Set `GRPC_PROTOSET` in your collection or environment.

PostOffice uses it when reflection is unavailable.

## Headless CLI

Run requests, folders, collections, and workflows without the TUI.

### Execute

```
# Single request
./postOffice run collection.json \
  "Users/Create User" -e dev.json

# Folder
./postOffice run collection.json \
  --folder smoke -e staging.json

# Collection
./postOffice run collection.json -e staging.json

# Workflow
./postOffice workflow run workflows/check.yaml \
  -e staging.json
```

Collections, environments and workflows can also be given by name instead of file path. Names are matched against loaded files (see `ls`), ignoring case: a collection or environment by its own name or file name (`SAPI` for `SAPI.postman_collection.json`), and a workflow by its ID, name or file name.

```
./postOffice run SAPI -e "Big tusks"
./postOffice workflow run health-check -e "Big tusks"
```

### Selection

Select a collection, environment and workflow once, and headless commands default to them:

```
./postOffice -c SAPI -e "Big tusks"     # same as: postOffice use -c SAPI -e "Big tusks"
./postOffice run                        # runs SAPI with Big tusks
./postOffice run "Users/Get User"       # one request in SAPI
./postOffice -w health-check
./postOffice workflow run               # runs health-check with Big tusks

./postOffice use                        # show the selection
./postOffice use -e ""                  # unselect the environment
./postOffice use --clear                # unselect everything
```

An explicit `-e` or collection argument still wins. The selection is stored under `cli` in `~/.postoffice_session.json`, and `ls` marks it with `*`.

Request paths use `/` between folders.

Escape literal slashes in names with `\/` and quote the path.

Headless collection runs support `pm.execution.setNextRequest()` for jumps and early termination.

The text report streams each step as it finishes, with ✓ passed in green, ✗ failed in red and − skipped in yellow. Colors are only used on a terminal (and never with `NO_COLOR`).

### CI and reports

```
./postOffice run collection.json \
  --folder smoke \
  -e ci.json \
  --output junit \
  --report results.xml
```

Options:

```
--output text|json|junit
--report <file>
--iterations <n>
--data <file.csv|json>
--bail
--persist
-e, --environment <file>
```

JSON includes request results, tests, errors, timings, and console output.

JUnit produces test suites and test cases.

`--persist` writes script-modified variables back to files. OpenAPI files cannot be persisted.

Exit codes:

```
0    All passed
1    Request, test, or script failure
2    Invalid arguments or input
```

## Listing

Inspect loaded collections and environments.

```
./postOffice ls
./postOffice ls var
./postOffice ls request

./postOffice ls var -c collection.json -e dev.json
./postOffice ls r -c collection.json
```

Loaded files are tracked in:

```
~/.postoffice_collections.json
```

## Merging

Merge collections or environments without opening the TUI.

```
# Second file wins
./postOffice merge a.json b.json --forceMerge

# First file wins
./postOffice merge a.json b.json --keep-original

# Write to new file
./postOffice merge a.json b.json \
  --forceMerge -o merged.json

# Keep collections grouped
./postOffice merge a.json b.json \
  --forceMerge --group-by-collection
```

Rules:

- Both inputs must be the same type.
- Folders merge recursively.
- Conflicts follow the selected strategy.
- Unique items are preserved.
- The first file is overwritten unless `-o` is used.
- Grouping keeps each collection under its own folder and avoids double-wrapping existing groups.

`-m` is an alias for `merge`.

## CLI Aliases

Save frequently used commands.

```
./postOffice alias add smoke \
  run collection.json --folder smoke -e ci.json

./postOffice alias list
./postOffice alias run smoke
./postOffice alias remove smoke
```

Aliases support `run`, `workflow`, `ls`, and `merge`.

Stored in:

```
~/.postoffice_aliases.json
```

Paths resolve from the current working directory. Exit codes are preserved.

## Development

```
go fmt ./...
go vet ./...
go test ./...
```

## Debugging

```
./postOffice --log debug.log
```

Logs file reads, writes, and errors.
