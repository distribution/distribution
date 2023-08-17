# Memory Ballast

| Status                   |                   |
| ------------------------ | ----------------- |
| Stability                | [beta]            |
| Distributions            | [core], [contrib] |

Memory Ballast extension enables applications to configure memory ballast for the process. For more details see:
- [Go memory ballast blogpost](https://web.archive.org/web/20210929130001/https://blog.twitch.tv/en/2019/04/10/go-memory-ballast-how-i-learnt-to-stop-worrying-and-love-the-heap-26c2462549a2/)
- [Golang issue related to this](https://github.com/golang/go/issues/23044)

The following settings can be configured:

- `size_mib` (default = 0, disabled): Is the memory ballast size, in MiB. 
  Takes higher priority than `size_in_percentage` if both are specified at the same time.
- `size_in_percentage` (default = 0, disabled): Set the memory ballast based on the 
  total memory in percentage, value range is `1-100`. 
  It is supported in both containerized(eg, docker, k8s) and physical host environments.
  
**How ballast size is calculated with percentage configuration**
When `size_in_percentage` is enabled with the value(1-100), the absolute `ballast_size` will be calculated by
`size_in_percentage * totalMemory / 100`. The `totalMemory` can be retrieved for hosts and containers(in docker, k8s, etc) by the following steps,
1. Look up Memory Cgroup subsystem on the target host or container, find out if there is any total memory limitation has been set for the running collector process.
   Check the value in `memory.limit_in_bytes` file under cgroup memory files (eg, `/sys/fs/cgroup/memory/memory.limit_in_bytes`).

2. If `memory.limit_in_bytes` is positive value other than `9223372036854771712`(`0x7FFFFFFFFFFFF000`). The `ballest_size`
   will be calculated by `memory.limit_in_bytes * size_in_percentage / 100`.
   If `memory.limit_in_bytes` value is `9223372036854771712`(`0x7FFFFFFFFFFFF000`), it indicates there is no memory limit has
   been set for the collector process or the running container in cgroup. Then the `totalMemory` will be determined in next step.
   
3. if there is no memory limit set in cgroup for the collector process or container where the collector is running. The total memory will be
   calculated by `github.com/shirou/gopsutil/v3/mem`[[link]](https://github.com/shirou/gopsutil/) on `mem.VirtualMemory().total` which is supported in multiple OS systems.


Example:
Config that uses 64 Mib of memory for the ballast:
```yaml
extensions:
  memory_ballast:
    size_mib: 64
```

Config that uses 20% of the total memory for the ballast:
```yaml
extensions:
  memory_ballast:
    size_in_percentage: 20
```

[beta]: https://github.com/open-telemetry/opentelemetry-collector-contrib#beta
[contrib]: https://github.com/open-telemetry/opentelemetry-collector-releases/tree/main/distributions/otelcol-contrib
[core]: https://github.com/open-telemetry/opentelemetry-collector-releases/tree/main/distributions/otelcol
