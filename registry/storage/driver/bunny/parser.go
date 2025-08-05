package bunny

import (
	"fmt"
	"net/url"
)

type DriverParameters struct {
	// StorageZone is the name of the storage zone to connect to.
	StorageZone string

	// Your Bunny Storage API key (also called "Password" in the Bunny dashboard).
	apiKey string

	// Hostname is the hostname of the Bunny Object Storage endpoint.
	Hostname url.URL

	// Pullzone is the URL of an associated Pull Zone where the data will be accessible.
	Pullzone url.URL
}

func NewParameters(parameters map[string]interface{}) (*DriverParameters, error) {
	if parameters == nil {
		return nil, fmt.Errorf("missing parameters for bunny driver")
	}
	params := &DriverParameters{}
	if storageZone, ok := parameters["storage_zone"].(string); ok {
		params.StorageZone = storageZone
	} else {
		return nil, fmt.Errorf("missing or invalid 'storage_zone' parameter for bunny driver")
	}
	if apiKey, ok := parameters["api_key"].(string); ok {
		params.apiKey = apiKey
	} else {
		return nil, fmt.Errorf("missing or invalid 'api_key' parameter for bunny driver")
	}
	if hostname, ok := parameters["hostname"].(string); ok {
		parsedURL, err := url.Parse(hostname)
		if err != nil {
			return nil, fmt.Errorf("invalid 'hostname' parameter for bunny driver: %v", err)
		}
		params.Hostname = *parsedURL
		if params.Hostname.Scheme == "" {
			params.Hostname.Scheme = "https"
		}
		if params.Hostname.Host == "" {
			return nil, fmt.Errorf("hostname must include a host component for bunny driver")
		}
	} else {
		return nil, fmt.Errorf("missing or invalid 'hostname' parameter for bunny driver")
	}
	if pullzone, ok := parameters["pullzone"].(string); ok {
		parsedURL, err := url.Parse(pullzone)
		if err != nil {
			return nil, fmt.Errorf("invalid 'pullzone' parameter for bunny driver: %v", err)
		}
		params.Pullzone = *parsedURL
		if params.Pullzone.Scheme == "" {
			params.Pullzone.Scheme = "https"
		}
		if params.Pullzone.Host == "" {
			return nil, fmt.Errorf("pullzone must include a host component for bunny driver")
		}
	} else {
		return nil, fmt.Errorf("missing or invalid 'pullzone' parameter for bunny driver")
	}
	return params, nil
}
