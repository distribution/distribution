package handlers

import (
	"fmt"
	"net"
	"net/http"
	"os"

	"code.google.com/p/go-uuid/uuid"
	"github.com/docker/distribution"
	"github.com/docker/distribution/configuration"
	ctxu "github.com/docker/distribution/context"
	"github.com/docker/distribution/notifications"
	"github.com/docker/distribution/registry/api/v2"
	"github.com/docker/distribution/registry/auth"
	registrymiddleware "github.com/docker/distribution/registry/middleware/registry"
	repositorymiddleware "github.com/docker/distribution/registry/middleware/repository"
	"github.com/docker/distribution/registry/storage"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/factory"
	storagemiddleware "github.com/docker/distribution/registry/storage/driver/middleware"
	"github.com/gorilla/mux"
	"golang.org/x/net/context"
)

// App is a global registry application object. Shared resources can be placed
// on this object that will be accessible from all requests. Any writable
// fields should be protected.
type App struct {
	context.Context
	Config configuration.Configuration

	// InstanceID is a unique id assigned to the application on each creation.
	// Provides information in the logs and context to identify restarts.
	InstanceID string

	router           *mux.Router                 // main application router, configured with dispatchers
	driver           storagedriver.StorageDriver // driver maintains the app global storage driver instance.
	registry         distribution.Registry       // registry is the primary registry backend for the app instance.
	accessController auth.AccessController       // main access controller for application

	// events contains notification related configuration.
	events struct {
		sink   notifications.Sink
		source notifications.SourceRecord
	}
}

// Value intercepts calls context.Context.Value, returning the current app id,
// if requested.
func (app *App) Value(key interface{}) interface{} {
	switch key {
	case "app.id":
		return app.InstanceID
	}

	return app.Context.Value(key)
}

// NewApp takes a configuration and returns a configured app, ready to serve
// requests. The app only implements ServeHTTP and can be wrapped in other
// handlers accordingly.
func NewApp(ctx context.Context, configuration configuration.Configuration) *App {
	app := &App{
		Config:     configuration,
		Context:    ctx,
		InstanceID: uuid.New(),
		router:     v2.RouterWithPrefix(configuration.HTTP.Prefix),
	}

	app.Context = ctxu.WithLogger(app.Context, ctxu.GetLogger(app, "app.id"))

	// Register the handler dispatchers.
	app.register(v2.RouteNameBase, func(ctx *Context, r *http.Request) http.Handler {
		return http.HandlerFunc(apiBase)
	})
	app.register(v2.RouteNameManifest, imageManifestDispatcher)
	app.register(v2.RouteNameTags, tagsDispatcher)
	app.register(v2.RouteNameBlob, layerDispatcher)
	app.register(v2.RouteNameBlobUpload, layerUploadDispatcher)
	app.register(v2.RouteNameBlobUploadChunk, layerUploadDispatcher)

	var err error
	app.driver, err = factory.Create(configuration.Storage.Type(), configuration.Storage.Parameters())

	if err != nil {
		// TODO(stevvooe): Move the creation of a service into a protected
		// method, where this is created lazily. Its status can be queried via
		// a health check.
		panic(err)
	}
	app.driver, err = applyStorageMiddleware(app.driver, configuration.Middleware["storage"])
	if err != nil {
		panic(err)
	}

	app.configureEvents(&configuration)

	app.registry = storage.NewRegistryWithDriver(app.driver)
	app.registry, err = applyRegistryMiddleware(app.registry, configuration.Middleware["registry"])
	if err != nil {
		panic(err)
	}

	authType := configuration.Auth.Type()

	if authType != "" {
		accessController, err := auth.GetAccessController(configuration.Auth.Type(), configuration.Auth.Parameters())
		if err != nil {
			panic(fmt.Sprintf("unable to configure authorization (%s): %v", authType, err))
		}
		app.accessController = accessController
	}

	return app
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

// configureEvents prepares the event sink for action.
func (app *App) configureEvents(configuration *configuration.Configuration) {
	// Configure all of the endpoint sinks.
	var sinks []notifications.Sink
	for _, endpoint := range configuration.Notifications.Endpoints {
		if endpoint.Disabled {
			ctxu.GetLogger(app).Infof("endpoint %s disabled, skipping", endpoint.Name)
			continue
		}

		ctxu.GetLogger(app).Infof("configuring endpoint %v (%v), timeout=%s, headers=%v", endpoint.Name, endpoint.URL, endpoint.Timeout, endpoint.Headers)
		endpoint := notifications.NewEndpoint(endpoint.Name, endpoint.URL, notifications.EndpointConfig{
			Timeout:   endpoint.Timeout,
			Threshold: endpoint.Threshold,
			Backoff:   endpoint.Backoff,
			Headers:   endpoint.Headers,
		})

		sinks = append(sinks, endpoint)
	}

	// NOTE(stevvooe): Moving to a new queueing implementation is as easy as
	// replacing broadcaster with a rabbitmq implementation. It's recommended
	// that the registry instances also act as the workers to keep deployment
	// simple.
	app.events.sink = notifications.NewBroadcaster(sinks...)

	// Populate registry event source
	hostname, err := os.Hostname()
	if err != nil {
		hostname = configuration.HTTP.Addr
	} else {
		// try to pick the port off the config
		_, port, err := net.SplitHostPort(configuration.HTTP.Addr)
		if err == nil {
			hostname = net.JoinHostPort(hostname, port)
		}
	}

	app.events.source = notifications.SourceRecord{
		Addr:       hostname,
		InstanceID: app.InstanceID,
	}
}

func (app *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close() // ensure that request body is always closed.

	// Set a header with the Docker Distribution API Version for all responses.
	w.Header().Add("Docker-Distribution-API-Version", "registry/2.0")
	app.router.ServeHTTP(w, r)
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

func (ssrw *singleStatusResponseWriter) Flush() {
	if flusher, ok := ssrw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// dispatcher returns a handler that constructs a request specific context and
// handler, using the dispatch factory function.
func (app *App) dispatcher(dispatch dispatchFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		context := app.context(w, r)

		defer func() {
			ctxu.GetResponseLogger(context).Infof("response completed")
		}()

		if err := app.authorized(w, r, context); err != nil {
			ctxu.GetLogger(context).Errorf("error authorizing context: %v", err)
			return
		}

		if app.nameRequired(r) {
			repository, err := app.registry.Repository(context, getName(context))

			if err != nil {
				ctxu.GetLogger(context).Errorf("error resolving repository: %v", err)

				switch err := err.(type) {
				case distribution.ErrRepositoryUnknown:
					context.Errors.Push(v2.ErrorCodeNameUnknown, err)
				case distribution.ErrRepositoryNameInvalid:
					context.Errors.Push(v2.ErrorCodeNameInvalid, err)
				}

				w.WriteHeader(http.StatusBadRequest)
				serveJSON(w, context.Errors)
				return
			}

			// assign and decorate the authorized repository with an event bridge.
			context.Repository = notifications.Listen(
				repository,
				app.eventBridge(context, r))

			context.Repository, err = applyRepoMiddleware(context.Repository, app.Config.Middleware["repository"])
			if err != nil {
				ctxu.GetLogger(context).Errorf("error initializing repository middleware: %v", err)
				context.Errors.Push(v2.ErrorCodeUnknown, err)
				w.WriteHeader(http.StatusInternalServerError)
				serveJSON(w, context.Errors)
				return
			}
		}

		handler := dispatch(context, r)

		ssrw := &singleStatusResponseWriter{ResponseWriter: w}
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
func (app *App) context(w http.ResponseWriter, r *http.Request) *Context {
	ctx := ctxu.WithRequest(app, r)
	ctx, w = ctxu.WithResponseWriter(ctx, w)
	ctx = ctxu.WithVars(ctx, r)
	ctx = ctxu.WithLogger(ctx, ctxu.GetRequestLogger(ctx))
	ctx = ctxu.WithLogger(ctx, ctxu.GetLogger(ctx,
		"vars.name",
		"vars.reference",
		"vars.digest",
		"vars.uuid"))

	context := &Context{
		App:        app,
		Context:    ctx,
		urlBuilder: v2.NewURLBuilderFromRequest(r),
	}

	return context
}

// authorized checks if the request can proceed with access to the requested
// repository. If it succeeds, the context may access the requested
// repository. An error will be returned if access is not available.
func (app *App) authorized(w http.ResponseWriter, r *http.Request, context *Context) error {
	ctxu.GetLogger(context).Debug("authorizing request")
	repo := getName(context)

	if app.accessController == nil {
		return nil // access controller is not enabled.
	}

	var accessRecords []auth.Access

	if repo != "" {
		accessRecords = appendAccessRecords(accessRecords, r.Method, repo)
	} else {
		// Only allow the name not to be set on the base route.
		if app.nameRequired(r) {
			// For this to be properly secured, repo must always be set for a
			// resource that may make a modification. The only condition under
			// which name is not set and we still allow access is when the
			// base route is accessed. This section prevents us from making
			// that mistake elsewhere in the code, allowing any operation to
			// proceed.
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusForbidden)

			var errs v2.Errors
			errs.Push(v2.ErrorCodeUnauthorized)
			serveJSON(w, errs)
			return fmt.Errorf("forbidden: no repository name")
		}
	}

	ctx, err := app.accessController.Authorized(context.Context, accessRecords...)
	if err != nil {
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
			ctxu.GetLogger(context).Errorf("error checking authorization: %v", err)
			w.WriteHeader(http.StatusBadRequest)
		}

		return err
	}

	// TODO(stevvooe): This pattern needs to be cleaned up a bit. One context
	// should be replaced by another, rather than replacing the context on a
	// mutable object.
	context.Context = ctx

	return nil
}

// eventBridge returns a bridge for the current request, configured with the
// correct actor and source.
func (app *App) eventBridge(ctx *Context, r *http.Request) notifications.Listener {
	actor := notifications.ActorRecord{
		Name: getUserName(ctx, r),
	}
	request := notifications.NewRequestRecord(ctxu.GetRequestID(ctx), r)

	return notifications.NewBridge(ctx.urlBuilder, app.events.source, actor, request, app.events.sink)
}

// nameRequired returns true if the route requires a name.
func (app *App) nameRequired(r *http.Request) bool {
	route := mux.CurrentRoute(r)
	return route == nil || route.GetName() != v2.RouteNameBase
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

// appendAccessRecords checks the method and adds the appropriate Access records to the records list.
func appendAccessRecords(records []auth.Access, method string, repo string) []auth.Access {
	resource := auth.Resource{
		Type: "repository",
		Name: repo,
	}

	switch method {
	case "GET", "HEAD":
		records = append(records,
			auth.Access{
				Resource: resource,
				Action:   "pull",
			})
	case "POST", "PUT", "PATCH":
		records = append(records,
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
		records = append(records,
			auth.Access{
				Resource: resource,
				Action:   "*",
			})
	}
	return records
}

// applyRegistryMiddleware wraps a registry instance with the configured middlewares
func applyRegistryMiddleware(registry distribution.Registry, middlewares []configuration.Middleware) (distribution.Registry, error) {
	for _, mw := range middlewares {
		rmw, err := registrymiddleware.Get(mw.Name, mw.Options, registry)
		if err != nil {
			return nil, fmt.Errorf("unable to configure registry middleware (%s): %s", mw.Name, err)
		}
		registry = rmw
	}
	return registry, nil

}

// applyRepoMiddleware wraps a repository with the configured middlewares
func applyRepoMiddleware(repository distribution.Repository, middlewares []configuration.Middleware) (distribution.Repository, error) {
	for _, mw := range middlewares {
		rmw, err := repositorymiddleware.Get(mw.Name, mw.Options, repository)
		if err != nil {
			return nil, err
		}
		repository = rmw
	}
	return repository, nil
}

// applyStorageMiddleware wraps a storage driver with the configured middlewares
func applyStorageMiddleware(driver storagedriver.StorageDriver, middlewares []configuration.Middleware) (storagedriver.StorageDriver, error) {
	for _, mw := range middlewares {
		smw, err := storagemiddleware.Get(mw.Name, mw.Options, driver)
		if err != nil {
			return nil, fmt.Errorf("unable to configure storage middleware (%s): %v", mw.Name, err)
		}
		driver = smw
	}
	return driver, nil
}
