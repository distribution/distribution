package bugsnag

import (
	"fmt"
	"strings"
)

const metadataPrefix string = "BUGSNAG_METADATA_"
const metadataPrefixLen int = len(metadataPrefix)
const metadataDefaultTab string = "custom"

type envMetadata struct {
	tab   string
	key   string
	value string
}

func loadEnvMetadata(environ []string) []envMetadata {
	metadata := make([]envMetadata, 0)
	for _, value := range environ {
		key, value, err := parseEnvironmentPair(value)
		if err != nil {
			continue
		}
		if keypath, err := parseMetadataKeypath(key); err == nil {
			tab, key := splitTabKeyValues(keypath)
			metadata = append(metadata, envMetadata{tab, key, value})
		}
	}
	return metadata
}

func splitTabKeyValues(keypath string) (string, string) {
	key_components := strings.SplitN(keypath, "_", 2)
	if len(key_components) > 1 {
		return key_components[0], key_components[1]
	}
	return metadataDefaultTab, keypath
}

func parseMetadataKeypath(key string) (string, error) {
	if strings.HasPrefix(key, metadataPrefix) && len(key) > metadataPrefixLen {
		return strings.TrimPrefix(key, metadataPrefix), nil
	}
	return "", fmt.Errorf("No metadata prefix found")
}
