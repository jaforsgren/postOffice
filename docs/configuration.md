# Configuration

PostOffice has no config file to edit. It keeps its state in JSON files in your home directory:

| File | Contents |
|---|---|
| `~/.postoffice_collections.json` | Paths of loaded collections and environments, reloaded on startup |
| `~/.postoffice_session.json` | The last view (`:session`) and the CLI selection (`postOffice use`) |
| `~/.postoffice_history.json` | [Request history](history.md), the 200 most recent entries, owner-only (`0600`) |
| `~/.postoffice_saved_responses.json` | Responses saved with `s` |
| `~/.postoffice_favorites.json` | Favorite requests (`f`) |
| `~/.postoffice_aliases.json` | [CLI aliases](cli.md#aliases) |

Next to each collection:

| File | Contents |
|---|---|
| `<collection>.dependencies.yaml` | [Request dependencies](dependencies.md) |
| `workflows/<collection>/*.yaml` | [Workflows](workflows.md) |

## Environment variables

| Variable | Used for |
|---|---|
| `VISUAL`, `EDITOR` | Editing requests, scripts and dependency files, and opening reports (falls back to `nvim`, then `vim`) |
| `NO_COLOR` | Disables colors in headless text reports |

`GRPC_PROTOSET`, set as a collection or environment variable, points [gRPC](grpc.md) at a descriptor set when server reflection is unavailable.

## Debug logging

```bash
postOffice --log debug.log
```

Logs file reads, writes, and errors.
