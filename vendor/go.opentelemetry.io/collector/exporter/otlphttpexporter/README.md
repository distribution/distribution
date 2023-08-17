# OTLP/HTTP Exporter

| Status                   |                       |
| ------------------------ | --------------------- |
| Stability                | traces [stable]       |
|                          | metrics [stable]      |
|                          | logs [beta]           |
| Supported pipeline types | traces, metrics, logs |
| Distributions            | [core], [contrib]     |

Export traces and/or metrics via HTTP using [OTLP](
https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/otlp.md)
format.

The following settings are required:

- `endpoint` (no default): The target base URL to send data to (e.g.: https://example.com:4318).
  To send each signal a corresponding path will be added to this base URL, i.e. for traces
  "/v1/traces" will appended, for metrics "/v1/metrics" will be appended, for logs
  "/v1/logs" will be appended. 

The following settings can be optionally configured:

- `traces_endpoint` (no default): The target URL to send trace data to (e.g.: https://example.com:4318/v1/traces).
   If this setting is present the `endpoint` setting is ignored for traces.
- `metrics_endpoint` (no default): The target URL to send metric data to (e.g.: https://example.com:4318/v1/metrics).
   If this setting is present the `endpoint` setting is ignored for metrics.
- `logs_endpoint` (no default): The target URL to send log data to (e.g.: https://example.com:4318/v1/logs).
   If this setting is present the `endpoint` setting is ignored logs.
- `tls`: see [TLS Configuration Settings](../../config/configtls/README.md) for the full set of available options.
- `timeout` (default = 30s): HTTP request time limit. For details see https://golang.org/pkg/net/http/#Client
- `read_buffer_size` (default = 0): ReadBufferSize for HTTP client.
- `write_buffer_size` (default = 512 * 1024): WriteBufferSize for HTTP client.

Example:

```yaml
exporters:
  otlphttp:
    endpoint: https://example.com:4318/v1/traces
```

By default `gzip` compression is enabled. See [compression comparison](../../config/configgrpc/README.md#compression-comparison) for details benchmark information. To disable, configure as follows:

```yaml
exporters:
  otlphttp:
    ...
    compression: none
```

The full list of settings exposed for this exporter are documented [here](./config.go)
with detailed sample configurations [here](./testdata/config.yaml).

[beta]: https://github.com/open-telemetry/opentelemetry-collector#beta
[contrib]: https://github.com/open-telemetry/opentelemetry-collector-releases/tree/main/distributions/otelcol-contrib
[core]: https://github.com/open-telemetry/opentelemetry-collector-releases/tree/main/distributions/otelcol
[stable]: https://github.com/open-telemetry/opentelemetry-collector#stable
