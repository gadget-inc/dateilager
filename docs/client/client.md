## client

DateiLager client

### Options

```
  -h, --help                  help for client
      --log-encoding string   Log encoding (console | json) (default "console")
      --log-level Level       Log level (default debug)
      --otel-context string   Open Telemetry context
      --server string         Server GRPC address
      --tracing               Whether tracing is enabled
```

### Environment variables

You can make Dateilager always use reflink support from the underlying filesystem with the `FORCE_REFLINKS=true` environment variable. If you want to disable support for reflinks even if a filesystem supports them, you can also set `FORCE_REFLINKS=never`.

### SEE ALSO

- [client get](client_get.md) -
- [client inspect](client_inspect.md) -
- [client new](client_new.md) -
- [client rebuild](client_rebuild.md) -
- [client reset](client_reset.md) -
- [client snapshot](client_snapshot.md) -
- [client update](client_update.md) -
