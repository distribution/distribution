# Memory Limiter Processor

| Status                   |                       |
| ------------------------ | --------------------- |
| Stability                | [beta]                |
| Supported pipeline types | traces, metrics, logs |
| Distributions            | [core], [contrib]     |

The memory limiter processor is used to prevent out of memory situations on
the collector. Given that the amount and type of data the collector processes is
environment specific and resource utilization of the collector is also dependent
on the configured processors, it is important to put checks in place regarding
memory usage.
 
The memory_limiter processor allows to perform periodic checks of memory
usage if it exceeds defined limits will begin dropping data and forcing GC to reduce
memory consumption.

The memory_limiter uses soft and hard memory limits. Hard limit is always above or equal
the soft limit.

When the memory usage exceeds the soft limit the processor will start dropping the data and
return errors to the preceding component it in the pipeline (which should be normally a
receiver).

When the memory usage is above the hard limit in addition to dropping the data the
processor will forcedly perform garbage collection in order to try to free memory.

When the memory usage drop below the soft limit, the normal operation is resumed (data
will not longer be dropped and no forced garbage collection will be performed).

The difference between the soft limit and hard limits is defined via `spike_limit_mib`
configuration option. The value of this option should be selected in a way that ensures
that between the memory check intervals the memory usage cannot increase by more than this
value (otherwise memory usage may exceed the hard limit - even if temporarily).
A good starting point for `spike_limit_mib` is 20% of the hard limit. Bigger
`spike_limit_mib` values may be necessary for spiky traffic or for longer check intervals.

Note that while the processor can help mitigate out of memory situations,
it is not a replacement for properly sizing and configuring the
collector. Keep in mind that if the soft limit is crossed, the collector will
return errors to all receive operations until enough memory is freed. This will
result in dropped data.

It is highly recommended to configure `ballastextension` as well as the
`memory_limiter` processor on every collector. The ballast should be configured to
be 1/3 to 1/2 of the memory allocated to the collector. The memory_limiter
processor should be the first processor defined in the pipeline (immediately after
the receivers). This is to ensure that backpressure can be sent to applicable
receivers and minimize the likelihood of dropped data when the memory_limiter gets
triggered.

Please refer to [config.go](./config.go) for the config spec.

The following configuration options **must be changed**:
- `check_interval` (default = 0s): Time between measurements of memory
usage. The recommended value is 1 second.
If the expected traffic to the Collector is very spiky then decrease the `check_interval`
or increase `spike_limit_mib` to avoid memory usage going over the hard limit.
- `limit_mib` (default = 0): Maximum amount of memory, in MiB, targeted to be
allocated by the process heap. Note that typically the total memory usage of
process will be about 50MiB higher than this value.  This defines the hard limit.
- `spike_limit_mib` (default = 20% of `limit_mib`): Maximum spike expected between the
measurements of memory usage. The value must be less than `limit_mib`. The soft limit
value will be equal to (limit_mib - spike_limit_mib).
The recommended value for `spike_limit_mib` is about 20% `limit_mib`.
- `limit_percentage` (default = 0): Maximum amount of total memory targeted to be
allocated by the process heap. This configuration is supported on Linux systems with cgroups
and it's intended to be used in dynamic platforms like docker.
This option is used to calculate `memory_limit` from the total available memory.
For instance setting of 75% with the total memory of 1GiB will result in the limit of 750 MiB.
The fixed memory setting (`limit_mib`) takes precedence
over the percentage configuration.
- `spike_limit_percentage` (default = 0): Maximum spike expected between the
measurements of memory usage. The value must be less than `limit_percentage`.
This option is used to calculate `spike_limit_mib` from the total available memory.
For instance setting of 25% with the total memory of 1GiB will result in the spike limit of 250MiB.
This option is intended to be used only with `limit_percentage`.

Examples:

```yaml
processors:
  memory_limiter:
    check_interval: 1s
    limit_mib: 4000
    spike_limit_mib: 800
```

```yaml
processors:
  memory_limiter:
    check_interval: 1s
    limit_percentage: 50
    spike_limit_percentage: 30
```

Refer to [config.yaml](./testdata/config.yaml) for detailed
examples on using the processor.

[beta]: https://github.com/open-telemetry/opentelemetry-collector#beta
[contrib]: https://github.com/open-telemetry/opentelemetry-collector-releases/tree/main/distributions/otelcol-contrib
[core]: https://github.com/open-telemetry/opentelemetry-collector-releases/tree/main/distributions/otelcol
