package registry

import (
	"net/http"

	"github.com/docker/docker-registry/configuration"

	log "github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
)

// App is a global registry application object. Shared resources can be placed
// on this object that will be accessible from all requests. Any writable
// fields should be protected.
type App struct {
	Config configuration.Configuration

	router *mux.Router
}

// NewApp takes a configuration and returns a configured app, ready to serve
// requests. The app only implements ServeHTTP and can be wrapped in other
// handlers accordingly.
func NewApp(configuration configuration.Configuration) *App {
	app := &App{
		Config: configuration,
		router: v2APIRouter(),
	}

	// Register the handler dispatchers.
	app.register(routeNameImageManifest, imageManifestDispatcher)
	app.register(routeNameBlob, layerDispatcher)
	app.register(routeNameTags, tagsDispatcher)
	app.register(routeNameBlobUpload, layerUploadDispatcher)
	app.register(routeNameBlobUploadResume, layerUploadDispatcher)

	return app
}

func (app *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	app.router.ServeHTTP(w, r)
}

// register a handler with the application, by route name. The handler will be
// passed through the application filters and context will be constructed at
// request time.
func (app *App) register(routeName string, dispatch dispatchFunc) {

	// TODO(stevvooe): This odd dispatcher/route registration is by-product of
	// some limitations in the gorilla/mux router. We are using it to keep
	// routing consistent between the client and server, but we may want to
	// replace it with manual routing and structure-based dispatch for better
	// control over the request execution.

	app.router.GetRoute(routeName).Handler(app.dispatcher(dispatch))
}

// dispatchFunc takes a context and request and returns a constructed handler
// for the route. The dispatcher will use this to dynamically create request
// specific handlers for each endpoint without creating a new router for each
// request.
type dispatchFunc func(ctx *Context, r *http.Request) http.Handler

// TODO(stevvooe): dispatchers should probably have some validation error
// chain with proper error reporting.

// dispatcher returns a handler that constructs a request specific context and
// handler, using the dispatch factory function.
func (app *App) dispatcher(dispatch dispatchFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		context := &Context{
			App:  app,
			Name: vars["name"],
		}

		// Store vars for underlying handlers.
		context.vars = vars

		context.log = log.WithField("name", context.Name)
		handler := dispatch(context, r)

		context.log.Infoln("handler", resolveHandlerName(r.Method, handler))
		handler.ServeHTTP(w, r)

		// Automated error response handling here. Handlers may return their
		// own errors if they need different behavior (such as range errors
		// for layer upload).
		if len(context.Errors.Errors) > 0 {
			w.WriteHeader(http.StatusBadRequest)
			serveJSON(w, context.Errors)
		}
	})
}
