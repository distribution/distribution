package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/docker/distribution/context"
	"github.com/gorilla/handlers"
	"github.com/opencontainers/go-digest"
)

// blocksDispatcher responds to posting a request containing
func blocksDispatcher(ctx *Context, r *http.Request) http.Handler {
	dgst, _ := getDigest(ctx)
	blocksHandler := &blocksHandler{
		Context: ctx,
		Digest:  dgst,
	}

	mhandler := handlers.MethodHandler{
		"POST": http.HandlerFunc(blocksHandler.RequestBlocks),
	}

	return mhandler
}

// blocksHandler serves http blob requests.
type blocksHandler struct {
	*Context
	Digest digest.Digest
}

//PostDeclaration returns the recipe for the given digest
func (th *blocksHandler) RequestBlocks(w http.ResponseWriter, r *http.Request) {
	context.GetLogger(th).Debug("RequestBlocks")

	var data [1024]byte

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
