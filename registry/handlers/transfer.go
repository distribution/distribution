package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/docker/distribution/context"
	"github.com/gorilla/handlers"
	"github.com/opencontainers/go-digest"
)

// transferDispatcher responds to posting a request containing
func transferDispatcher(ctx *Context, r *http.Request) http.Handler {
	dgst, _ := getDigest(ctx)
	transferHandler := &transferHandler{
		Context: ctx,
		Digest:  dgst,
	}

	mhandler := handlers.MethodHandler{
		"POST": http.HandlerFunc(transferHandler.RequestBlocks),
	}

	return mhandler
}

// transferHandler serves http blob requests.
type transferHandler struct {
	*Context
	Digest digest.Digest
}

//PostDeclaration returns the recipe for the given digest
func (th *transferHandler) RequestBlocks(w http.ResponseWriter, r *http.Request) {
	context.GetLogger(th).Debug("RequestBlocks")

	var data [1024]byte

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
