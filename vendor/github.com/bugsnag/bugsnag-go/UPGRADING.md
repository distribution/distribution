# Upgrading guide

## v1 to v2

The v2 release adds support for Go modules, removes web framework
integrations from the main repository, and supports library configuration
through environment variables. The following breaking changes occurred as a part
of this release:

### Importing the package

```diff+go
- import "github.com/bugsnag/bugsnag-go"
+ import "github.com/bugsnag/bugsnag-go/v2"
```

### Removed `Configuration.Endpoint`

The `Endpoint` configuration option was deprecated as a part of the v1.4.0
release in November 2018. It was replaced with `Endpoints`, which includes
options for configuring both event and session delivery.

```diff+go
- config.Endpoint = "https://notify.myserver.example.com"
+ config.Endpoints = {
+ 	Notify: "https://notify.myserver.example.com",
+ 	Sessions: "https://sessions.myserver.example.com"
+ }
```

### Moved web framework integrations into separate repositories

Integrations with Negroni, Revel, and Gin now live in separate repositories, to
prevent implicit dependencies on every framework and to improve the ease of
updating each integration independently.

```diff+go
- import "github.com/bugsnag/bugsnag-go/negroni"
+ import "github.com/bugsnag/bugsnag-go-negroni"
```

```diff+go
- import "github.com/bugsnag/bugsnag-go/revel"
+ import "github.com/bugsnag/bugsnag-go-revel"
```

```diff+go
- import "github.com/bugsnag/bugsnag-go/gin"
+ import "github.com/bugsnag/bugsnag-go-gin"
```

### Renamed constants for platform consistency

```diff+go
- bugsnag.VERSION
+ bugsnag.Version
```
