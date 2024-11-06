# Experimental Features

The Logs SDK contains features that have not yet stabilized.
These features are added to the OpenTelemetry Go Logs SDK prior to stabilization so that users can start experimenting with them and provide feedback.

These feature may change in backwards incompatible ways as feedback is applied.
See the [Compatibility and Stability](#compatibility-and-stability) section for more information.

## Features

- [Filter Processors](#filter-processor)

### Filter Processor

Users of logging libraries often want to know if a log `Record` will be processed or dropped before they perform complex operations to construct the `Record`.
The [`Logger`] in the Logs Bridge API provides the `Enabled` method for just this use-case.
In order for the Logs Bridge SDK to effectively implement this API, it needs to be known if the registered [`Processor`]s are enabled for the `Record` within a context.
A [`Processor`] that knows, and can identify, what `Record` it will process or drop when it is passed to `OnEmit` can communicate this to the SDK `Logger` by implementing the `FilterProcessor`.

By default, the SDK `Logger.Enabled` will return true when called.
Only if all the registered [`Processor`]s implement `FilterProcessor` and they all return `false` will `Logger.Enabled` return `false`.

See the [`minsev`] [`Processor`] for an example use-case.
It is used to filter `Record`s out that a have a `Severity` below a threshold.

[`Logger`]: https://pkg.go.dev/go.opentelemetry.io/otel/log#Logger
[`Processor`]: https://pkg.go.dev/go.opentelemetry.io/otel/sdk/log#Processor
[`minsev`]: https://pkg.go.dev/go.opentelemetry.io/contrib/processors/minsev

## Compatibility and Stability

Experimental features do not fall within the scope of the OpenTelemetry Go versioning and stability [policy](../../../../VERSIONING.md).
These features may be removed or modified in successive version releases, including patch versions.

When an experimental feature is promoted to a stable feature, a migration path will be included in the changelog entry of the release.
