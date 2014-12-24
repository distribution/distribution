package registry

import (
	"github.com/Sirupsen/logrus"
	"github.com/docker/distribution/api/v2"
)

// Context should contain the request specific context for use in across
// handlers. Resources that don't need to be shared across handlers should not
// be on this object.
type Context struct {
	// App points to the application structure that created this context.
	*App

	// Name is the prefix for the current request. Corresponds to the
	// namespace/repository associated with the image.
	Name string

	// Errors is a collection of errors encountered during the request to be
	// returned to the client API. If errors are added to the collection, the
	// handler *must not* start the response via http.ResponseWriter.
	Errors v2.Errors

	// vars contains the extracted gorilla/mux variables that can be used for
	// assignment.
	vars map[string]string

	// log provides a context specific logger.
	log *logrus.Entry

	urlBuilder *v2.URLBuilder
}
