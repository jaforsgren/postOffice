# Bash scripts

A `BASH` request runs its body as a bash script. Add one with the request type picker (`Bash`), or write it in the collection:

```json
{
  "name": "Confirm deploy",
  "request": {
    "method": "BASH",
    "url": { "raw": "bash://~/dev/{{app}}" },
    "header": [{ "key": "TARGET", "value": "{{environment}}" }],
    "body": {
      "mode": "raw",
      "raw": "read -r -p \"Deploy {{app}} to $TARGET? [y/n] \" answer\n[[ \"$answer\" == [yY]* ]] || exit 1\n./deploy.sh"
    }
  }
}
```

| Part | Meaning |
| --- | --- |
| Method `BASH` (or a `bash://` URL) | Marks the request as a script |
| URL `bash://<dir>` | Working directory; `~/` expands to home, empty runs in PostOffice's current directory |
| Headers | Exported to the script as environment variables |
| Body | The script, run with `bash -c` |

`{{variables}}` resolve in the URL, headers and body from the environment, collection and folder scopes, including values set by pre-request scripts.

## Prompts

Scripts can prompt (`read -p "Continue? [y/n] "`):

- **TUI** — the TUI steps aside like it does for `$EDITOR`, the script runs in the terminal, and the TUI returns once it exits. `ctrl+c` stops only the script.
- **Headless** (`postOffice run`) — with stdin a terminal the script reads it; its output is echoed on stderr so stdout keeps the report.
- **No terminal** (CI, pipes) — stdin is empty, so a prompt reads EOF and the script decides what that means.

Scripts that run at the same time take turns at the terminal.

## Response

The response body is everything the script printed (stdout and stderr). Exit code `0` answers `200 Exit 0`, any other code `500 Exit <code>`; the code is also in the `Exit-Code` header. `ctrl+x` (or a cancelled run) kills the script.

## Pre-request and test scripts

Bash requests run collection → folder → request pre-request and test scripts like HTTP requests:

```js
pm.test('exits 0', () => pm.response.to.have.status(200));
pm.collectionVariables.set('version', pm.response.text().trim());
```

See [examples/bash-scripts.json](../examples/bash-scripts.json).
