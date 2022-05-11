package v2

import (
	"fmt"

	"github.com/distribution/distribution/v3/reference"
)

// ExtendRoute extends the routes using the template.
// The Name and Path in the template will be re-generated according to
// - Namespace (ns)
// - Extension Name (ext)
// - Component name (component)
// Returns the full route descriptor with Name and Path populated.
// Returns true if the route is successfully extended, or false if route exists.
func ExtendRoute(ns, ext, component string, template RouteDescriptor, nameRequired bool) (RouteDescriptor, bool) {
	name := RouteNameExtensionsRegistry
	path := routeDescriptorsMap[RouteNameBase].Path
	if nameRequired {
		name = RouteNameExtensionsRepository
		path += "{name:" + reference.NameRegexp.String() + "}/"
	}
	name = fmt.Sprintf("%s-%s-%s-%s", name, ns, ext, component)
	path = fmt.Sprintf("%s_%s/%s/%s", path, ns, ext, component)

	desc := template
	desc.Name = name
	desc.Path = path

	if _, exists := routeDescriptorsMap[desc.Name]; exists {
		return desc, false
	}

	routeDescriptors = append(routeDescriptors, desc)
	routeDescriptorsMap[desc.Name] = desc
	APIDescriptor.RouteDescriptors = routeDescriptors

	return desc, true
}
