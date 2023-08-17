# Logging Exporter

| Status                   |                         |
| ------------------------ | ----------------------- |
| Stability                | [In development]        |
| Supported pipeline types | traces, metrics, logs   |
| Distributions            | [core], [contrib]       |

Exports data to the console via zap.Logger.

Supported pipeline types: traces, metrics, logs

## Getting Started

The following settings are optional:

- `loglevel` (default = `info`): the log level of the logging export
  (debug|info|warn|error). When set to `debug`, pipeline data is verbosely
  logged.
- `sampling_initial` (default = `2`): number of messages initially logged each
  second.
- `sampling_thereafter` (default = `500`): sampling rate after the initial
  messages are logged (every Mth message is logged). Refer to [Zap
  docs](https://godoc.org/go.uber.org/zap/zapcore#NewSampler) for more details.
  on how sampling parameters impact number of messages.

Example:

```yaml
exporters:
  logging:
    loglevel: debug
    sampling_initial: 5
    sampling_thereafter: 200
```

[contrib]: https://github.com/open-telemetry/opentelemetry-collector-releases/tree/main/distributions/otelcol-contrib
[core]: https://github.com/open-telemetry/opentelemetry-collector-releases/tree/main/distributions/otelcol
[In development]: https://github.com/open-telemetry/opentelemetry-collector#in-development
