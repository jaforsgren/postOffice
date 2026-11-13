# Scripts and tests

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
