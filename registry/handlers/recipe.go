package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/docker/distribution/context"
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
	// blobStore := rh.Repository.Blobs(rh)
	// blob, _ := blobStore.Get(rh, rh.Digest)

	encodeManager := rh.EncodeManager
	// recipe, _ := encodeManager.GetRecipeForLayer(rh.Digest, blob)

	// encodeManager.InsertRecipeInDB(recipe)
	recipe, err := encodeManager.GetRecipeFromDB(rh.Digest)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	//Add code to fetch and generate the handler
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(recipe)
}
