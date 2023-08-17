# OpenTelemetry Collector Service

## How to provide configuration

The `--config` flag accepts either a file path or values in the form of a config URI `"<scheme>:<opaque_data>"`. Currently, the
OpenTelemetry Collector supports the following providers `scheme`:
- [file](../confmap/provider/fileprovider/provider.go) - Reads configuration from a file. E.g. `file:path/to/config.yaml`.
- [env](../confmap/provider/envprovider/provider.go) - Reads configuration from an environment variable. E.g. `env:MY_CONFIG_IN_AN_ENVVAR`.
- [yaml](../confmap/provider/yamlprovider/provider.go) - Reads configuration from yaml bytes. E.g. `yaml:exporters::logging::loglevel: debug`.

For more technical details about how configuration is resolved you can read the [configuration resolving design](../confmap/README.md#configuration-resolving).

### Single Config Source

1. Simple local file:

    `./otelcorecol --config=examples/local/otel-config.yaml`

2. Simple local file using the new URI format:

    `./otelcorecol --config=file:examples/local/otel-config.yaml`

3. Config provided via an environment variable:

    `./otelcorecol --config=env:MY_CONFIG_IN_AN_ENVVAR`


### Multiple Config Sources

1. Merge a `otel-config.yaml` file with the content of an environment variable `MY_OTHER_CONFIG` and use the merged result as the config:
     
    `./otelcorecol --config=file:examples/local/otel-config.yaml --config=env:MY_OTHER_CONFIG`

2. Merge a `config.yaml` file with the content of a yaml bytes configuration (overwrites the `exporters::logging::loglevel` config) and use the content as the config:

    `./otelcorecol --config=file:examples/local/otel-config.yaml --config="yaml:exporters::logging::loglevel: info"`
