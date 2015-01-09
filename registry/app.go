package registry

import (
	"fmt"
	"net/http"

	"github.com/docker/distribution/api/v2"
	"github.com/docker/distribution/auth"
	"github.com/docker/distribution/configuration"
	"github.com/docker/distribution/storage"
	"github.com/docker/distribution/storagedriver"
	"github.com/docker/distribution/storagedriver/factory"

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

	layerHandler storage.LayerHandler

	accessController auth.AccessController
}

// NewApp takes a configuration and returns a configured app, ready to serve
// requests. The app only implements ServeHTTP and can be wrapped in other
// handlers accordingly.
func NewApp(configuration configuration.Configuration) *App {
	app := &App{
		Config: configuration,
		router: v2.Router(),
	}

	// Register the handler dispatchers.
	app.register(v2.RouteNameBase, func(ctx *Context, r *http.Request) http.Handler {
		return http.HandlerFunc(apiBase)
	})
	app.register(v2.RouteNameManifest, imageManifestDispatcher)
	app.register(v2.RouteNameTags, tagsDispatcher)
	app.register(v2.RouteNameBlob, layerDispatcher)
	app.register(v2.RouteNameBlobUpload, layerUploadDispatcher)
	app.register(v2.RouteNameBlobUploadChunk, layerUploadDispatcher)

	driver, err := factory.Create(configuration.Storage.Type(), configuration.Storage.Parameters())

	if err != nil {
		// TODO(stevvooe): Move the creation of a service into a protected
		// method, where this is created lazily. Its status can be queried via
		// a health check.
		panic(err)
	}

	app.driver = driver
	app.services = storage.NewServices(app.driver)
	authType := configuration.Auth.Type()

	if authType != "" {
		accessController, err := auth.GetAccessController(configuration.Auth.Type(), configuration.Auth.Parameters())
		if err != nil {
			panic(fmt.Sprintf("unable to configure authorization (%s): %v", authType, err))
		}
		app.accessController = accessController
	}

	layerHandlerType := configuration.LayerHandler.Type()

	if layerHandlerType != "" {
		lh, err := storage.GetLayerHandler(layerHandlerType, configuration.LayerHandler.Parameters(), driver)
		if err != nil {
			panic(fmt.Sprintf("unable to configure layer handler (%s): %v", layerHandlerType, err))
		}
		app.layerHandler = lh
	}

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
		context := app.context(r)

		if err := app.authorized(w, r, context); err != nil {
			return
		}

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

// context constructs the context object for the application. This only be
// called once per request.
func (app *App) context(r *http.Request) *Context {
	vars := mux.Vars(r)
	context := &Context{
		App:        app,
		Name:       vars["name"],
		urlBuilder: v2.NewURLBuilderFromRequest(r),
	}

	// Store vars for underlying handlers.
	context.vars = vars

	return context
}

// authorized checks if the request can proceed with with request access-
// level. If it cannot, the method will return an error.
func (app *App) authorized(w http.ResponseWriter, r *http.Request, context *Context) error {
	if app.accessController == nil {
		return nil // access controller is not enabled.
	}

	var accessRecords []auth.Access

	if context.Name != "" {
		resource := auth.Resource{
			Type: "repository",
			Name: context.Name,
		}

		switch r.Method {
		case "GET", "HEAD":
			accessRecords = append(accessRecords,
				auth.Access{
					Resource: resource,
					Action:   "pull",
				})
		case "POST", "PUT", "PATCH":
			accessRecords = append(accessRecords,
				auth.Access{
					Resource: resource,
					Action:   "pull",
				},
				auth.Access{
					Resource: resource,
					Action:   "push",
				})
		case "DELETE":
			// DELETE access requires full admin rights, which is represented
			// as "*". This may not be ideal.
			accessRecords = append(accessRecords,
				auth.Access{
					Resource: resource,
					Action:   "*",
				})
		}
	} else {
		// Only allow the name not to be set on the base route.
		route := mux.CurrentRoute(r)

		if route == nil || route.GetName() != v2.RouteNameBase {
			// For this to be properly secured, context.Name must always be set
			// for a resource that may make a modification. The only condition
			// under which name is not set and we still allow access is when the
			// base route is accessed. This section prevents us from making that
			// mistake elsewhere in the code, allowing any operation to proceed.
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusForbidden)

			var errs v2.Errors
			errs.Push(v2.ErrorCodeUnauthorized)
			serveJSON(w, errs)
		}
	}

	if err := app.accessController.Authorized(r, accessRecords...); err != nil {
		switch err := err.(type) {
		case auth.Challenge:
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			err.ServeHTTP(w, r)

			var errs v2.Errors
			errs.Push(v2.ErrorCodeUnauthorized, accessRecords)
			serveJSON(w, errs)
		default:
			// This condition is a potential security problem either in
			// the configuration or whatever is backing the access
			// controller. Just return a bad request with no information
			// to avoid exposure. The request should not proceed.
			context.log.Errorf("error checking authorization: %v", err)
			w.WriteHeader(http.StatusBadRequest)
		}

		return err
	}

	return nil
}

// apiBase implements a simple yes-man for doing overall checks against the
// api. This can support auth roundtrips to support docker login.
func apiBase(w http.ResponseWriter, r *http.Request) {
	const emptyJSON = "{}"
	// Provide a simple /v2/ 200 OK response with empty json response.
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Length", fmt.Sprint(len(emptyJSON)))

	fmt.Fprint(w, emptyJSON)
}
