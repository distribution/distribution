package registry

import (
	"net/http"

	"github.com/docker/docker-registry/storagedriver"
	"github.com/docker/docker-registry/storagedriver/factory"

	"github.com/docker/docker-registry/configuration"
	"github.com/docker/docker-registry/storage"

	log "github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
)

// App is a global registry application object. Shared resources can be placed
// on this object that will be accessible from all requests. Any writable
// fields should be protected.
type App struct {
	Config configuration.Configuration

	router *mux.Router

	// driver maintains the app global storage driver instance.
	driver storagedriver.StorageDriver

	// services contains the main services instance for the application.
	services *storage.Services
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
	app.register(routeNameTags, tagsDispatcher)
	app.register(routeNameBlob, layerDispatcher)
	app.register(routeNameBlobUpload, layerUploadDispatcher)
	app.register(routeNameBlobUploadResume, layerUploadDispatcher)

	driver, err := factory.Create(configuration.Storage.Type(), configuration.Storage.Parameters())

	if err != nil {
		// TODO(stevvooe): Move the creation of a service into a protected
		// method, where this is created lazily. Its status can be queried via
		// a health check.
		panic(err)
	}

	app.driver = driver
	app.services = storage.NewServices(app.driver)

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

// singleStatusResponseWriter only allows the first status to be written to be
// the valid request status. The current use case of this class should be
// factored out.
type singleStatusResponseWriter struct {
	http.ResponseWriter
	status int
}

func (ssrw *singleStatusResponseWriter) WriteHeader(status int) {
	if ssrw.status != 0 {
		return
	}
	ssrw.status = status
	ssrw.ResponseWriter.WriteHeader(status)
}

// dispatcher returns a handler that constructs a request specific context and
// handler, using the dispatch factory function.
func (app *App) dispatcher(dispatch dispatchFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		context := &Context{
			App:        app,
			Name:       vars["name"],
			urlBuilder: newURLBuilderFromRequest(r),
		}

		// Store vars for underlying handlers.
		context.vars = vars

		context.log = log.WithField("name", context.Name)
		handler := dispatch(context, r)

		ssrw := &singleStatusResponseWriter{ResponseWriter: w}
		context.log.Infoln("handler", resolveHandlerName(r.Method, handler))
		handler.ServeHTTP(ssrw, r)

		// Automated error response handling here. Handlers may return their
		// own errors if they need different behavior (such as range errors
		// for layer upload).
		if context.Errors.Len() > 0 {
			if ssrw.status == 0 {
				w.WriteHeader(http.StatusBadRequest)
			}
			serveJSON(w, context.Errors)
		}
	})
}
