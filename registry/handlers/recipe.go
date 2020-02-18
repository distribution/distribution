package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	//"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/encode"

	//"github.com/docker/distribution/registry/api/errcode"
	//"github.com/docker/distribution/registry/api/v2"
	"github.com/gorilla/handlers"
	"github.com/opencontainers/go-digest"
)

// recipeDispatcher responds to a request to fetch the recipe of a
// image.
func recipeDispatcher(ctx *Context, r *http.Request) http.Handler {
	dgst, _ := getDigest(ctx)
	recipeHandler := &recipeHandler{
		Context: ctx,
		Digest:  dgst,
	}

	mhandler := handlers.MethodHandler{
		"GET": http.HandlerFunc(recipeHandler.GetRecipe),
	}

	return mhandler
}

// recipeHandler serves http blob requests.
type recipeHandler struct {
	*Context

	Digest digest.Digest
}

//GetRecipe returns the recipe for the given digest
func (rh *recipeHandler) GetRecipe(w http.ResponseWriter, r *http.Request) {
	context.GetLogger(rh).Debug("GetRecipe")
	blobStore := rh.Repository.Blobs(rh)
	blob, _ := blobStore.Get(rh, rh.Digest)
	recipePayload, _ := encode.GetRecipeForImage(blob)
	fmt.Print(len(blob))

	//Add code to fetch and generate the handler
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(recipePayload)
}
