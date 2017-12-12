# Examples of working with bugsnag-go

This library has extensions for several use cases and web frameworks, and this
directory includes many examples. Most examples can be run by fetching
dependencies, inserting your API key, and using `go run`:

```
$ go get [example]/main.go
$ go run [example]/main.go
```

Other examples (such as App Engine) include separate instructions with
additional steps.

## Use cases

* [Capturing panics within goroutines](using-goroutines). Goroutines require
  special care to avoid crashing the app entirely or cleaning up before an error
  report can be sent. This is an example of a panic within a goroutine which is
  sent to Bugsnag.
* [Deploying to Google App Engine](appengine)
* [Using Gin](gin) (web framework)
* [Using net/http](http)
* [Using Negroni](negroni) (web framework)
* [Using Revel](revel) (web framework)
