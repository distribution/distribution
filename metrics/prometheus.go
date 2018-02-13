package metrics

import "github.com/docker/go-metrics"

const (
	// NamespacePrefix is the namespace of prometheus metrics
	NamespacePrefix = "registry"
)

var (
	// StorageNamespace is the prometheus namespace of blob/cache related operations
	StorageNamespace = metrics.NewNamespace(NamespacePrefix, "storage", nil)

	// MiddlewareNamespace is the prometheus namespace of middleware related operations
	MiddlewareNamespace = metrics.NewNamespace(NamespacePrefix, "middleware", nil)
)
