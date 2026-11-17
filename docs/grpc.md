# gRPC

Execute unary gRPC requests directly from Postman collections.

Supports:

- Server reflection.
- Protobuf descriptor sets.
- TLS and insecure connections.
- Metadata and binary headers.
- Variable substitution.
- Pre-request scripts and tests.
- Workflow execution.

## Editing

Press `e` on a gRPC request. Fields include endpoint, service/method, metadata, JSON message, TLS and inject defaults.

**Inject defaults** (off by default, `enter` toggles it) makes the response JSON list fields left at their default value (`0`, `""`, `false`, empty lists, the first enum value), which protobuf JSON leaves out otherwise, like grpcurl's `-emit-defaults`. It is saved on the request as `"injectDefaults": true`.

| Key | Action |
|---|---|
| `j` / `k` | Select field |
| `enter` | Edit / toggle TLS or inject defaults |
| `ctrl+s` | Save a multi-line field |
| `ctrl+f` | Browse server reflection |
| `esc` | Back |

Reflection lists services and methods. Selecting a method fills its path and generates a JSON message template.

Reflection browsing uses an insecure connection. TLS services require manual method entry.

## Without reflection

Generate a descriptor set:

```bash
protoc -I protos --include_imports \
  --descriptor_set_out=services.protoset \
  $(find protos -name '*.proto')
```

Set `GRPC_PROTOSET` in your collection or environment. PostOffice uses it when reflection is unavailable.
