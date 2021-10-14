package distribution

import "context"

// ExtensionService provices access to extensions.
type ExtensionService interface {
	// Get returns the extended service identified by its project name.
	// The project name in the format of `<ns>/<ext>`.
	// The caller should cast the returned service into proper interface
	// according to the extension provider.
	Get(ctx context.Context, name string) (interface{}, error)

	// All returns all available extensions with their components.
	// The returned list consists of extension components named by
	// `_<ns>/<ext>/<component>`
	All(ctx context.Context) ([]string, error)
}
