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

Press `e` on a gRPC request. Fields include endpoint, service/method, metadata, JSON message, and TLS.

| Key | Action |
|---|---|
| `j` / `k` | Select field |
| `enter` | Edit / toggle TLS |
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
