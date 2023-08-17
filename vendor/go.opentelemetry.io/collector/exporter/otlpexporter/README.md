# OTLP gRPC Exporter

| Status                   |                       |
| ------------------------ | --------------------- |
| Stability                | traces [stable]       |
|                          | metrics [stable]      |
|                          | logs [beta]           |
| Supported pipeline types | traces, metrics, logs |
| Distributions            | [core], [contrib]     |

Export data via gRPC using [OTLP](
https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/otlp.md)
format. By default, this exporter requires TLS and offers queued retry capabilities.

## Getting Started

The following settings are required:

- `endpoint` (no default): host:port to which the exporter is going to send OTLP trace data,
using the gRPC protocol. The valid syntax is described
[here](https://github.com/grpc/grpc/blob/master/doc/naming.md).
If a scheme of `https` is used then client transport security is enabled and overrides the `insecure` setting.
- `tls`: see [TLS Configuration Settings](../../config/configtls/README.md) for the full set of available options.

Example:

```yaml
exporters:
  otlp:
    endpoint: otelcol2:4317
    tls:
      cert_file: file.cert
      key_file: file.key
  otlp/2:
    endpoint: otelcol2:4317
    tls:
      insecure: true
```

By default, `gzip` compression is enabled. See [compression comparison](../../config/configgrpc/README.md#compression-comparison) for details benchmark information. To disable, configure as follows:

```yaml
exporters:
  otlp:
    ...
    compression: none
```

## Advanced Configuration

Several helper files are leveraged to provide additional capabilities automatically:

- [gRPC settings](https://github.com/open-telemetry/opentelemetry-collector/blob/main/config/configgrpc/README.md)
- [TLS and mTLS settings](https://github.com/open-telemetry/opentelemetry-collector/blob/main/config/configtls/README.md)
- [Queuing, retry and timeout settings](https://github.com/open-telemetry/opentelemetry-collector/blob/main/exporter/exporterhelper/README.md)

[beta]: https://github.com/open-telemetry/opentelemetry-collector#beta
[contrib]: https://github.com/open-telemetry/opentelemetry-collector-releases/tree/main/distributions/otelcol-contrib
[core]: https://github.com/open-telemetry/opentelemetry-collector-releases/tree/main/distributions/otelcol
[stable]: https://github.com/open-telemetry/opentelemetry-collector#stable
