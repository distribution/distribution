package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/docker/distribution/registry/api/errcode"
)

func createCancelledRequest(shouldCancel bool) (*http.Request, error) {
	ctx, cancel := context.WithCancel(context.Background())
	testRequest, err := http.NewRequest("GET", "/", strings.NewReader(""))
	if err != nil {
		return nil, err
	}
	cancelledRequest := testRequest.WithContext(ctx)
	if shouldCancel {
		cancel()
	}
	return cancelledRequest, nil
}

func TestCheckForClientDisconnection(t *testing.T) {
	r, err := createCancelledRequest(true)
	w := httptest.NewRecorder()
	if err != nil {
		t.Fatal(err)
	}
	disconnected := checkForClientDisconnection(w, r)
	if disconnected == nil {
		t.Fatal("expected client disconnected error got nil")
	}
	if !strings.Contains(disconnected.Error(), "client disconnected") {
		t.Fatalf("expected error to signify client disconnection got %s", disconnected.Error())
	}
	if w.Result().StatusCode != 499 {
		t.Fatalf("expected status code 499 got %d", w.Result().StatusCode)
	}
}

func TestClientNotDisconnected(t *testing.T) {
	r, err := createCancelledRequest(false)
	w := httptest.NewRecorder()
	if err != nil {
		t.Fatal(err)
	}
	disconnected := checkForClientDisconnection(w, r)
	if disconnected != nil {
		t.Fatal("expected no error got client disconnected")
	}
	if w.Result().StatusCode == 499 {
		t.Fatal("unexpected status code 499")
	}
}

func TestHandleDisconnectionEvent(t *testing.T) {
	ctx := Context{
		Errors: errcode.Errors{},
	}
	r, err := createCancelledRequest(true)
	w := httptest.NewRecorder()
	if err != nil {
		t.Fatal(err)
	}
	var result bool
	ctx.Errors, result = handleDisconnectionEvent(&ctx, w, r)
	if !result {
		t.Fatal("disconnection event not handled correctly")
	}
	found := false
	for _, err := range ctx.Errors {
		if err.Error() == "clientdisconnected: client disconnected" {
			found = true
		}
	}
	if !found {
		t.Fatal("request context does not contain a disconnection event")
	}
}

func TestHandleNonDisconnectionEvent(t *testing.T) {
	ctx := Context{
		Errors: errcode.Errors{},
	}
	r, err := createCancelledRequest(false)
	w := httptest.NewRecorder()
	if err != nil {
		t.Fatal(err)
	}
	var result bool
	ctx.Errors, result = handleDisconnectionEvent(&ctx, w, r)
	if result {
		t.Fatal("disconnection event not handled correctly")
	}
	if ctx.Errors.Len() > 0 {
		t.Fatalf("unexpected context errors received %v", ctx.Errors)
	}
}
