package device

import (
	"runtime"
)

// Cached runtime versions that can be updated globally by framework
// integrations through AddVersion.
var versions *RuntimeVersions

// RuntimeVersions define the various versions of Go and any framework that may
// be in use.
// As a user of the notifier you're unlikely to need to modify this struct.
// As such, the authors reserve the right to introduce breaking changes to the
// properties in this struct. In particular the framework versions are liable
// to change in new versions of the notifier in minor/patch versions.
type RuntimeVersions struct {
	Go string `json:"go"`

	Gin     string `json:"gin,omitempty"`
	Martini string `json:"martini,omitempty"`
	Negroni string `json:"negroni,omitempty"`
	Revel   string `json:"revel,omitempty"`
}

// GetRuntimeVersions retrieves the recorded runtime versions in a goroutine-safe manner.
func GetRuntimeVersions() *RuntimeVersions {
	if versions == nil {
		versions = &RuntimeVersions{Go: runtime.Version()}
	}
	return versions
}

// AddVersion permits a framework to register its version, assuming it's one of
// the officially supported frameworks.
func AddVersion(framework, version string) {
	if versions == nil {
		versions = &RuntimeVersions{Go: runtime.Version()}
	}
	switch framework {
	case "Martini":
		versions.Martini = version
	case "Gin":
		versions.Gin = version
	case "Negroni":
		versions.Negroni = version
	case "Revel":
		versions.Revel = version
	}
}
