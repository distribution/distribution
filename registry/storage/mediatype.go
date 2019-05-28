package storage

import (
	"github.com/docker/distribution"
)

// validateMediaType ensures that a descriptor's mediaType is allowed
// based on configuration. If the first argument is 0, the descriptor
// is treated as a manifest config, otherwise is assumed a manifest layer
func validateMediaType(descIndex int, descMediaType string, configMediaTypes manifestConfigMediaTypes, layerMediaTypes manifestLayerMediaTypes) error {
	if descIndex == 0 {
		// index 0 is manifest config
		if (configMediaTypes.allow != nil && !configMediaTypes.allow.MatchString(descMediaType)) ||
			(configMediaTypes.deny != nil && configMediaTypes.deny.MatchString(descMediaType)) {
			return distribution.ErrManifestConfigMediaTypeForbidden{
				ConfigMediaType: descMediaType,
			}
		}
	} else {
		// index > 0 is a layer
		if (layerMediaTypes.allow != nil && !layerMediaTypes.allow.MatchString(descMediaType)) ||
			(layerMediaTypes.deny != nil && layerMediaTypes.deny.MatchString(descMediaType)) {
			return distribution.ErrManifestLayerMediaTypeForbidden{
				LayerIndex:     descIndex - 1,
				LayerMediaType: descMediaType,
			}
		}
	}
	return nil
}
