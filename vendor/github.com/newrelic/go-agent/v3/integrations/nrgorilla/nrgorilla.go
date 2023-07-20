// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Package nrgorilla instruments https://github.com/gorilla/mux applications.
//
// Use this package to instrument inbound requests handled by a gorilla
// mux.Router.  Use the nrgorilla.Middleware as the first middleware registered
// with your router.
//
// Complete example:
// https://github.com/newrelic/go-agent/tree/master/v3/integrations/nrgorilla/example/main.go
package nrgorilla

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/newrelic/go-agent/v3/internal"
	newrelic "github.com/newrelic/go-agent/v3/newrelic"
)

func init() { internal.TrackUsage("integration", "framework", "gorilla", "v1") }

type instrumentedHandler struct {
	app  *newrelic.Application
	orig http.Handler
}

func (h instrumentedHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if newrelic.FromContext(r.Context()) == nil {
		name := routeName(r)
		txn := h.app.StartTransaction(name)
		txn.SetWebRequestHTTP(r)
		w = txn.SetWebResponse(w)
		defer txn.End()
		r = newrelic.RequestWithTransactionContext(r, txn)
	}

	h.orig.ServeHTTP(w, r)
}

func instrumentRoute(h http.Handler, app *newrelic.Application) http.Handler {
	if _, ok := h.(instrumentedHandler); ok {
		return h
	}
	return instrumentedHandler{
		orig: h,
		app:  app,
	}
}

func routeName(r *http.Request) string {
	route := mux.CurrentRoute(r)
	if nil == route {
		return "NotFoundHandler"
	}
	if n := route.GetName(); n != "" {
		return n
	}
	if n, _ := route.GetPathTemplate(); n != "" {
		return r.Method + " " + n
	}
	n, _ := route.GetHostTemplate()
	return r.Method + " " + n
}

// InstrumentRoutes instruments requests through the provided mux.Router.  Use
// this after the routes have been added to the router.
//
// Deprecated: Use the newer and more complete Middleware method instead.
func InstrumentRoutes(r *mux.Router, app *newrelic.Application) *mux.Router {
	if app != nil {
		r.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
			h := instrumentRoute(route.GetHandler(), app)
			route.Handler(h)
			return nil
		})
		if nil != r.NotFoundHandler {
			r.NotFoundHandler = instrumentRoute(r.NotFoundHandler, app)
		}
	}
	return r
}

// Middleware creates a new mux.MiddlewareFunc.  When used, this middleware
// will create a transaction for each inbound request.  The transaction will be
// available in the Request's context throughout the call chain, including in
// any other middlewares that are registered after this one.  For this reason,
// it is important for this middleware to be registered first.
//
// Note that mux.MiddlewareFuncs are not called for the NotFoundHandler or
// MethodNotAllowedHandler.  To instrument these handlers, use
// newrelic.WrapHandle
// (https://godoc.org/github.com/newrelic/go-agent/v3/newrelic#WrapHandle).
//
// Note that if you are moving from the now deprecated InstrumentRoutes to this
// Middleware, the reported time of your transactions may increase.  This is
// expected and nothing to worry about.  This method includes in the
// transaction total time request time that is spent in other custom
// middlewares whereas InstrumentRoutes does not.
func Middleware(app *newrelic.Application) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			name := routeName(r)
			txn := app.StartTransaction(name)
			defer txn.End()
			txn.SetWebRequestHTTP(r)
			w = txn.SetWebResponse(w)
			r = newrelic.RequestWithTransactionContext(r, txn)
			next.ServeHTTP(w, r)
		})
	}
}
