# PostOffice

A terminal UI for browsing and executing Postman collections with vim-style navigation.

## Features

- Browse Postman collections in a terminal interface
- Execute HTTP requests and view responses
- Vim-style keyboard navigation
- Persistent session state
- Environment variable support
- Request and collection editing
- gRPC request editing with live server reflection
- Workflow automation with inline JavaScript orchestration
- Path parameter editing (`:id` style) per request
- Variable autocomplete with resolved value preview when typing `{{`

## Installation

### Build from source

```bash
go build -o postOffice
```

## Usage

### Basic Usage

```bash
# Run the application
./postOffice

# Run with file operation logging (for debugging)
./postOffice --log debug.log
```

### Navigation

**Lists (Collections / Requests / Environments / Variables):**
- `j/k` or `↓/↑` — navigate items
- `g/G` — jump to top / bottom
- `enter` — select item (open folder, execute request)
- `e` — edit selected request
- `E` — edit scripts for selected request
- `d` — delete selected request
- `D` — duplicate selected request
- `i` — show info for selected item
- `J` — show raw JSON for selected item
- `H` — view saved responses for selected request
- `r` — refresh current view
- `/` — search / filter items
- `?` — show help
- `h/esc/backspace` — go back / up one folder level
- `q` or `ctrl+c` — quit

**Overlay views (Response / Info / JSON / Log):**
- `j/k` or `↓/↑` — scroll
- `g/G` or `Home/End` — jump to top / bottom
- `d/u` or `Page Down/Up` — scroll half page
- `q` or `esc/h/backspace` — close view

**Request execution:**
- `enter` — execute selected request
- `ctrl+e` — re-execute selected request
- `ctrl+r` — view last response for selected request (without re-executing)
- `y` — copy response body (in response view)
- `Y` — copy full response (in response view)
- `s` — save response (in response view)

**Workflow detail view:**
- `j/k` — navigate steps
- `g/G` — jump to first / last step
- `a` — add step
- `e` — edit step request
- `E` — edit step scripts
- `u` — run workflow up to this step
- `f` — run workflow from this step
- `r` — run this step only
- `R` — run full workflow
- `ctrl+r` — view last response for this step
- `ctrl+d` — delete step
- `i` — inspect step request info

**Command Mode:**

Press `:` to enter command mode:

- `:load <path>` or `:l <path>` — load a Postman collection
- `:loadenv <path>` or `:le <path>` — load an environment file
- `:collections` or `:c` — switch to collections view
- `:requests` or `:r` — switch to requests view
- `:environments` or `:env` — switch to environments view
- `:variables` or `:var` — show all variables
- `:info` or `:i` — display item info
- `:edit` — edit selected request
- `:wf` — browse workflows
- `:wf run <id>` — run a workflow by ID
- `:wf new <id>` — create a new workflow
- `:w` — save changes to file
- `:wq` — save changes and quit
- `:changes` or `:ch` — show unsaved changes
- `:help` or `:h` or `?` — show help
- `:quit` or `:q` — exit

**Search Mode:**

Press `/` to enter search mode:
- type to filter items (results update as you type)
- `enter` — confirm search
- `esc` — cancel search

### File Paths

Collections and environments are automatically saved to `~/.postoffice_collections.json` and restored on next startup.

When loading files, `~/` is expanded to your home directory:

```
:load ~/postman/my-collection.json
```

## Request Execution

1. Navigate to a request using `j/k`
2. Press `enter` to execute
3. View the response — use `j/k` to scroll, `d/u` for half-page scrolling
4. Press `q` or `esc` to close
5. Press `ctrl+r` to view the last response without re-executing

## Editing Requests

1. Navigate to a request and press `e` or use `:edit`
2. Use `j/k` to navigate between fields (Name, Method, URL, path params, Headers, Body)
3. Press `enter` to edit a field
4. **For single-line fields (Name, Method, URL, path params):**
   - Type your changes
   - Press `enter` to save
   - Press `esc` to cancel
5. **For multi-line fields (Headers, Body):**
   - Type your changes
   - Press `enter` for newlines
   - Press `ctrl+s` to save
   - Press `esc` to cancel
6. Press `esc` to exit edit mode (changes saved to memory)
7. Use `:w` to write changes to file
8. Use `:wq` to write changes and quit

**Path Parameters:**

Requests with URL path parameters (e.g. `/api/v1/claims/:id`) show each `:param` as a dedicated editable field below the URL. Set a value and it will be substituted into the URL before the request is sent. Values persist for the session — executing the request from the list will reuse the last-set values.

**Variable Autocomplete:**

Type `{{` in any field (URL, path params, headers, body) to trigger variable autocomplete. Matching variables from the active environment and collection are shown with their resolved values. Press `Tab` to insert the highlighted suggestion; press `Tab` again to cycle through alternatives. The URL field also shows a resolved preview (`→ full-url`) combining variable substitution and path param values.

**Managing Unsaved Changes:**

- `:changes` - View all unsaved changes
- In changes view:
  - `d` - Discard selected change
  - `ctrl+d` - Discard all changes
  - `esc` - Close changes view

## Workflows

Workflows automate multi-request sequences with inline JavaScript. Each workflow is a single YAML file stored in a `workflows/` directory next to the collection file.

```yaml
id: login-and-fetch-users
name: Login and Fetch Users
description: Authenticate and fetch users
version: 1

steps:
  - id: login
    request: Auth/Login
  - id: list-users
    request: Users/List Users

script: |
  export default async function (wf, pm) {
    const loginResult = await wf.run("login");

    if (loginResult.response.code !== 201) {
      wf.fail("Login failed");
      return;
    }

    const data = loginResult.response.json();
    pm.collectionVariables.set("authToken", "token-" + data.id);

    const listResult = await wf.run("list-users");
    const users = listResult.response.json();
    console.log("Found " + users.length + " users");
  }
```

**TUI commands:**
- `:wf` — browse available workflows
- `:wf run <id>` — run a workflow by ID
- `:wf new <id>` — create a new workflow skeleton

**Workflow list keys:**
- `enter` — open workflow detail
- `ctrl+r` — run selected workflow

**Workflow detail keys:**
- `R` — run full workflow
- `u` — run up to selected step
- `f` — run from selected step
- `r` — run selected step only
- `ctrl+r` — view last response for selected step

The workflow script receives `wf` (workflow control API) and `pm` (full Postman-compatible scripting API). Pre/post request scripts attached to collection requests are triggered automatically on each step.

See [docs/workflows.md](docs/workflows.md) for full documentation.

## gRPC Requests

PostOffice supports gRPC requests stored in Postman collections (method `GRPC` or URL scheme `grpc://`). A dedicated edit form replaces the standard HTTP edit fields.

### Editing a gRPC Request

1. Navigate to a gRPC request and press `e` or use `:edit`
2. Use `j/k` to navigate between fields:
   - **Name** — request name in the collection
   - **Endpoint** — server address, e.g. `localhost:50051`
   - **Service/Method** — fully-qualified method path, e.g. `helloworld.Greeter/SayHello`
   - **Metadata** — gRPC metadata headers, one `Key: Value` per line
   - **Message** — JSON request body sent as the protobuf message
   - **TLS** — `Enabled` / `Disabled (insecure)` — press `Enter` to toggle

TLS state is stored in the URL scheme: `grpc://` for insecure, `grpcs://` for TLS with system certificate roots.

3. Press `enter` to edit a field (single-line fields: `enter` to save, `esc` to cancel)
4. For multi-line fields (Metadata, Message): press `ctrl+s` to save, `esc` to cancel
5. Press `esc` to exit edit mode, then `:w` to write to file

### Server Reflection

When the cursor is on the **Service/Method** field, press `Ctrl+R` to browse the server's available services and methods via gRPC reflection:

1. PostOffice connects to the endpoint and lists all services
2. Navigate services with `j/k`, press `enter` to expand a service's methods
3. Navigate methods with `j/k`, press `enter` to select a method:
   - **Service/Method** is filled with the selected method's full path
   - **Message** is pre-filled with a JSON template generated from the method's protobuf input type
4. Press `esc` to return to the method list, press `esc` again to return to the edit form without selecting

The reflection connection is insecure (no TLS). For TLS-enabled servers, edit the **Service/Method** field manually.

## Environment Variables

1. Press `v` to open variable management
2. Load an environment file with `:load <env-path>`
3. Edit variable values directly in the UI
4. Variables are automatically applied to requests

## Debugging

Enable file operation logging to troubleshoot issues:

```bash
./postOffice --log debug.log
```

The log file will contain:
- `[FILE_OPEN]` - File read attempts
- `[FILE_WRITE]` - File write attempts
- `[ERROR]` - Any errors encountered

## Development

```bash
# Format code
go fmt ./...

# Vet code
go vet ./...

# Run tests
go test ./...
```

## License

MIT
