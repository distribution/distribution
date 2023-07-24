package middleware

import (
	"fmt"

	httpmiddleware "github.com/distribution/distribution/v3/registry/middleware/http"
	"github.com/gorilla/mux"
	"github.com/newrelic/go-agent/v3/integrations/nrgorilla"
	"github.com/newrelic/go-agent/v3/integrations/nrlogrus"
	"github.com/newrelic/go-agent/v3/newrelic"
)

type newRelicHttpMiddleware struct{}

var _ = &newRelicHttpMiddleware{}

func newNewRelicHttpMiddleware(options map[string]interface{}) (mux.MiddlewareFunc, error) {
	name, ok := options["name"]
	if !ok {
		return nil, fmt.Errorf("no New Relic app name provided")
	}
	nameStr, ok := name.(string)
	if !ok {
		return nil, fmt.Errorf("the New Relic app name must be a string")
	}
	licenseKey, ok := options["licenseKey"]
	if !ok {
		return nil, fmt.Errorf("no New Relic license key provided")
	}
	licenseKeyStr, ok := licenseKey.(string)
	if !ok {
		return nil, fmt.Errorf("the New Relic license key must be a string")
	}
	newrelicApp, err := newrelic.NewApplication(
		newrelic.ConfigAppName(nameStr),
		newrelic.ConfigLicense(licenseKeyStr),
		nrlogrus.ConfigStandardLogger(),
	)
	if err != nil {
		return nil, err
	}
	return nrgorilla.Middleware(newrelicApp), nil
}

func init() {
	httpmiddleware.Register("newrelic", newNewRelicHttpMiddleware)
}
