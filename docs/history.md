# Request history and timing

## History

Every execution started from the TUI (HTTP, gRPC, Service Bus and AMQP, including replays) is recorded with its resolved URL/target, headers and body, environment name, status, duration, timestamp and response size. Workflow runs are not recorded.

- Open it with `ctrl+y` or `:history` (`:hist`, `:hi`).
- `enter` replays the entry with exactly the recorded resolved inputs. Variables are not re-resolved and pre-request/test scripts do not run.
- `=` shows a line diff (`-` history entry, `+` current response) of status, sorted headers and pretty-printed body.

History is persisted to `~/.postoffice_history.json` (capped at the 200 most recent entries). Because entries contain resolved secrets such as auth headers and connection strings, the file is written with owner-only permissions (`0600`).

## Timing breakdown

Press `t` (or run `:timing`) in the response view to open a waterfall popup of the request's phases:

- **HTTP**: DNS lookup, TCP connect, TLS handshake, TTFB and content download. On a reused connection, DNS/TCP/TLS show as `reused`; phases that don't apply (DNS for an IP address, TLS for plain HTTP) show as `n/a`.
- **gRPC** (workflow steps): connect + reflection, invoke.
- **AMQP**: dial (DNS + TCP), handshake (TLS + AMQP), open channel, publish.
- **Azure Service Bus** (sends and peeks): total only.

Phases the request never reached because it failed are left out. Press `t`, `q` or `esc` to close.
