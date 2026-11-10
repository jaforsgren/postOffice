# Workflows

Workflows enable multi-request orchestration directly in PostOffice. A workflow is a single YAML file containing a step list and an inline JavaScript function that controls execution.

Workflows are intended for:

- Authentication flows (login, token refresh, token propagation)
- Provisioning sequences (create user → assign role → configure settings)
- Data migrations and batch operations
- Test scenarios requiring ordered request execution
- Any operation that spans multiple requests with conditional logic

---

## File Structure

Each workflow is a single `.yaml` file stored in a `workflows/` directory alongside the collection file.

```
my-collection.json
workflows/
  login-flow.yaml
  user-provisioning.yaml
```

### Minimal example

```yaml
id: login-flow
name: Login Flow
steps: []
script: |
  export default async function (wf, pm) {
    // start here
  }
```

### Full example

```yaml
id: login-and-fetch-users
name: Login and Fetch Users
description: Authenticate, extract token, fetch users
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
      wf.fail("Login failed with status " + loginResult.response.code);
      return;
    }

    const data = loginResult.response.json();
    pm.collectionVariables.set("authToken", "token-" + data.id);

    const listResult = await wf.run("list-users");
    const users = listResult.response.json();
    console.log("Found " + users.length + " users");
  }
```

---

## Fields

### id

Stable identifier used for execution references, CLI commands, and future automation.

```yaml
id: login-and-fetch-users
```

### name

Human-readable name shown in the TUI workflow list.

```yaml
name: Login and Fetch Users
```

### description

Optional description shown beneath the workflow name in the TUI.

```yaml
description: Authenticate and then fetch users
```

### version

Optional integer. Reserved for future migration support.

```yaml
version: 1
```

### steps

Declares the requests the workflow may execute. Used by the TUI for visualization and status tracking — the script controls which steps run and in what order.

```yaml
steps:
  - id: login
    request: Auth/Login
  - id: list-users
    request: Users/List Users
```

Each step has:

| Field | Description |
|-------|-------------|
| `id` | Stable step identifier used in `wf.run("id")` |
| `request` | Path to a request within the collection, e.g. `Auth/Login` or `Users/List Users` |

Request paths are resolved by traversing the collection item tree using `/` as a folder separator. They are never absolute URLs.

### script

The JavaScript function that drives execution. Must be an `export default async function`.

```yaml
script: |
  export default async function (wf, pm) {
    await wf.run("login");
  }
```

The script is the sole source of truth for execution logic. All branching, retries, loops, variable manipulation, and sequencing happen here.

---

## Runtime API

The script receives two arguments: `wf` and `pm`.

### wf

Workflow control API.

```ts
wf.run(stepId: string, options?: RunOptions): Promise<StepResult>
wf.fail(reason?: string): void
wf.skip(reason?: string): void
wf.stop(reason?: string): void
wf.context: object
wf.status: string
```

#### wf.run(stepId, options?)

Executes the step identified by `stepId`. Triggers pre-request and post-request scripts attached to the request, identical to executing the request manually in the TUI.

```js
const result = await wf.run("login");
```

**Options:**

```ts
{
  repeat?: number;
  until?: (response: Response) => boolean;
}
```

`repeat` runs the step N times. `until` runs the step repeatedly until the predicate returns `true` or the repeat limit is reached.

```js
// Run exactly three times
await wf.run("warm-cache", { repeat: 3 });

// Poll until condition is met (up to 10 attempts)
await wf.run("wait-for-token", {
  repeat: 10,
  until: (res) => res.code === 200,
});
```

#### wf.fail(reason?)

Marks the workflow as failed and halts execution.

```js
wf.fail("Login failed");
```

#### wf.skip(reason?)

Marks the current step as skipped. Execution continues.

```js
wf.skip("Feature not available in this environment");
```

#### wf.stop(reason?)

Stops workflow execution without marking it as failed.

```js
wf.stop("User chose not to continue");
```

#### wf.context

A plain object for storing transient runtime state across steps. Not persisted after the workflow completes.

```js
wf.context.userId = user.id;
// later in the same run:
console.log(wf.context.userId);
```

#### wf.status

The current workflow status string: `pending`, `running`, `success`, `failed`, `skipped`, or `stopped`.

---

### pm

The full PostOffice Postman-compatible scripting API, identical to the API available in request pre/post scripts.

```js
pm.environment.get("token");
pm.environment.set("token", value);
pm.collectionVariables.get("baseUrl");
pm.collectionVariables.set("authToken", token);
pm.variables.get("userId");
```

Variable mutations made in the workflow script are immediately visible to subsequent `wf.run()` calls, including variable substitution in URLs and request bodies.

---

## Run Result

`wf.run()` returns a `StepResult`:

```ts
{
  status: "success" | "failed" | "skipped";
  response: Response;  // alias of last
  last: Response;      // final execution result
  responses: Response[]; // all results when repeat > 1
}
```

### Step status

A step is **failed** when its request could not be sent (connection error, timeout, unknown request), when one of its Postman scripts throws, when any `pm.test` fails, or when its own pre/post script fails. A pre-request script calling `pm.execution.skipRequest()` makes it **skipped**. Otherwise it is **success**; an HTTP error status alone (e.g. 404) does not fail a step unless a test checks for it.

With `repeat`/`until`, the last attempt decides the status, so a poll that fails a few times before succeeding ends as success.

A failed step does not stop the workflow script, so it can react to `result.status`. The workflow itself ends as failed ("one or more steps failed") if any step failed.

```js
const result = await wf.run("create-claim");
if (result.status === "failed") {
  wf.fail("create-claim failed, see the run report");
}
```

### Response object

```ts
{
  code: number;
  headers: { [key: string]: string[] };
  body: string;
  time: number; // milliseconds
  json(): any;
  text(): string;
}
```

```js
const result = await wf.run("get-user");
const code = result.response.code;
const body = result.response.json();
const raw = result.response.text();
```

---

## Variables

Workflows share the same variable system as requests. No separate workflow variable scope exists.

Variable access and mutation work identically to request pre/post scripts:

```js
// Read a collection variable
const baseUrl = pm.collectionVariables.get("baseUrl");

// Write an environment variable (visible to all subsequent steps)
pm.environment.set("authToken", token);

// Read any variable (collection → environment → global precedence)
const userId = pm.variables.get("userId");
```

---

## Pre/Post Scripts

When `wf.run("stepId")` executes a request, any pre-request and post-request scripts attached to that request in the collection are executed automatically, in the same order as manual execution.

This means existing scripts (e.g. scripts that extract tokens from responses and set environment variables) work without modification in workflows.

---

## gRPC Steps

Workflows support gRPC requests. If the request at the given path has a `grpc://` or `grpcs://` URL scheme, the workflow runtime invokes it via gRPC using server reflection.

```yaml
steps:
  - id: list-services
    request: gRPC/ListServices
```

The result object is the same as HTTP: `response.code`, `response.body`, `response.json()`.

---

## TUI Commands

### List workflows

```
:wf
```

Opens the workflow browser. Navigate with `j/k`. Press `enter` or `ctrl+r` to run the selected workflow.

### Run a workflow by ID

```
:wf run <id>
```

Runs the workflow with the given ID immediately.

### Create a new workflow

```
:wf new <id>
```

Creates a new workflow file at `workflows/<id>.yaml` with a skeleton script.

### Run selected workflow

```
ctrl+r
```

In the workflow list view, runs the currently highlighted workflow.

### Run a folder as a workflow

```
ctrl+e   (on a folder, in the requests view)
```

Asks `Run folder 'policies/TEST' as workflow? (y/n)` in the command bar. `y` confirms. Any other key cancels. On a request, `ctrl+e` still executes the request.

On confirm, every request in the folder runs one after another in collection order, depth-first through subfolders, and the workflow run view shows progress. Each step goes through the normal executor, so request pre-request and test scripts run and variables carry over between steps, the same as in any other workflow.

The first run saves the folder as a workflow in `workflows/` next to the collection:

| Field | Value |
|-------|-------|
| `name` | Full folder path, e.g. `policies/TEST` or `a/b/c` |
| `id` / file name | Slug of that path, e.g. `policies-test` → `workflows/policies-test.yaml` |
| `steps` | One per request, `request` set to the request's full collection path; step ids are slugs of the request names (`-2`, `-3`, … on clashes) |
| `script` | `await wf.run("<step>")` for every step, in order |

It shows up in the `:wf` list like any other workflow. In the requests list, a folder with a saved workflow gets a `⚡` marker after its name. The marker shows up as soon as the workflow is saved, and on startup for workflows already on disk.

Pressing `ctrl+e` on that folder again runs the saved workflow instead of creating a new one:

- **Folder unchanged** (same requests in the same order): the saved workflow runs unchanged, including any edits you made to it.
- **Folder changed, workflow never edited** (the script is still the generated sequence and no step has `pre_script`/`post_script`): the workflow is regenerated, saved over the old one and run.
- **Folder changed, workflow hand-edited**: the saved workflow runs unchanged so your edits are not overwritten, and the status bar says so. Delete the file to regenerate it.

Limitations:

- A folder or request whose name contains `/` cannot be a step, because step paths are split on `/`. Such a folder is rejected with an error.
- Collection-level and folder-level scripts do not run. Only each request's own scripts do. Postman runner flow control (`pm.execution.setNextRequest`, `pm.execution.skipRequest`) is not supported, so requests always run strictly in order. Steps that need flow control should be scripted in the workflow `script` with `wf.run`, `wf.skip` and `wf.stop`.
- The whole workflow script shares the runtime's 30-second timeout, so long folders may time out.

---

## Workflow Discovery

Workflows are loaded from the `workflows/` directory relative to the collection file currently active in the TUI. Switch collections to switch the available workflow set.

---

## TUI Run View

While a workflow runs, the TUI shows:

- Workflow name and overall status
- Each step with its status icon and label
- Repeat progress for steps using `repeat`
- Execution logs from `console.log()` calls
- Error message if the workflow fails

Step status icons:

| Icon | Status |
|------|--------|
| `○` | Pending |
| `▶` | Running |
| `✓` | Success |
| `✗` | Failed |
| `−` | Skipped |
| `■` | Stopped |

---

## Examples

### Conditional branching

```js
export default async function (wf, pm) {
  const result = await wf.run("get-user");

  if (result.response.code === 404) {
    await wf.run("create-user");
  } else {
    await wf.run("update-user");
  }
}
```

### Polling until ready

```js
export default async function (wf, pm) {
  const result = await wf.run("wait-for-job", {
    repeat: 20,
    until: (res) => res.json().status === "complete",
  });

  if (result.last.json().status !== "complete") {
    wf.fail("Job did not complete in time");
  }
}
```

### Token propagation

```js
export default async function (wf, pm) {
  const loginResult = await wf.run("login");
  const token = loginResult.response.json().token;
  pm.environment.set("authToken", token);

  // All subsequent steps use {{authToken}} automatically
  await wf.run("get-profile");
  await wf.run("list-orders");
}
```

### Batch operations

```js
export default async function (wf, pm) {
  const listResult = await wf.run("list-users");
  const users = listResult.response.json();

  for (const user of users) {
    pm.collectionVariables.set("userId", String(user.id));
    await wf.run("delete-user");
    console.log("Deleted user " + user.id);
  }
}
```

### Error handling

```js
export default async function (wf, pm) {
  const result = await wf.run("provision-resource");

  if (result.response.code >= 400) {
    const err = result.response.json();
    console.log("Provisioning error: " + err.message);
    wf.fail(err.message);
    return;
  }

  const resourceId = result.response.json().id;
  pm.collectionVariables.set("resourceId", String(resourceId));

  await wf.run("configure-resource");
  await wf.run("verify-resource");
}
```

---

## Design Notes

- One workflow = one YAML file. Easy to diff, review, and version-control.
- JavaScript provides unlimited flexibility. No workflow DSL is provided.
- The `steps` list is declarative metadata for the TUI. Execution order is determined by the script.
- Runtime status is ephemeral. No execution state is written to files.
- Variable mutations in the script propagate immediately to subsequent steps.
