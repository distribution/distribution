# Collector Feature Gates

This package provides a mechanism that allows operators to enable and disable
experimental or transitional features at deployment time. These flags should
be able to govern the behavior of the application starting as early as possible
and should be available to every component such that decisions may be made
based on flags at the component level.

## Usage

Feature gates must be defined and registered with the global registry in
an `init()` function.  This makes the `Gate` available to be configured and 
queried with a default value of its `Enabled` property.

```go
const myFeatureGateID = "namespaced.uniqueIdentifier"

func init() {
	featuregate.Register(featuregate.Gate{
		ID:          fancyNewFeatureGate,
		Description: "A brief description of what the gate controls",
		Enabled:     false,
	})
}
```

The status of the gate may later be checked by interrogating the global 
feature gate registry:

```go
if featuregate.IsEnabled(myFeatureGateID) {
	setupNewFeature()
}
```

Note that querying the registry takes a read lock and accesses a map, so it 
should be done once and the result cached for local use if repeated checks 
are required.  Avoid querying the registry in a loop.

## Controlling Gates

Feature gates can be enabled or disabled via the CLI, with the 
`--feature-gates` flag. When using the CLI flag, gate 
identifiers must be presented as a comma-delimited list. Gate identifiers
prefixed with `-` will disable the gate and prefixing with `+` or with no
prefix will enable the gate.

```shell
otelcol --config=config.yaml --feature-gates=gate1,-gate2,+gate3
```

This will enable `gate1` and `gate3` and disable `gate2`.

## Feature Lifecycle

Features controlled by a `Gate` should follow a three-stage lifecycle, 
modeled after the [system used by Kubernetes](https://kubernetes.io/docs/reference/command-line-tools-reference/feature-gates/#feature-stages):

1. An `alpha` stage where the feature is disabled by default and must be enabled 
   through a `Gate`.
2. A `beta` stage where the feature has been well tested and is enabled by 
   default but can be disabled through a `Gate`.
3. A generally available stage where the feature is permanently enabled and 
   the `Gate` is no longer operative.

Features that prove unworkable in the `alpha` stage may be discontinued 
without proceeding to the `beta` stage.  Features that make it to the `beta` 
stage will not be dropped and will eventually reach general availability 
where the `Gate` that allowed them to be disabled during the `beta` stage 
will be removed.
