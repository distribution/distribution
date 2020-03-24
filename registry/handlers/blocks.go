package handlers

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/docker/distribution/context"
	"github.com/docker/distribution/encode"
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

	rawDeclaration, _ := ioutil.ReadAll(r.Body)
	declaration := encode.NewDeclarationFromString(string(rawDeclaration))
	fmt.Println(declaration)

	blobStore := th.Repository.Blobs(th)
	blob, _ := blobStore.Get(th, th.Digest)

	recipeManager := th.RecipeManager
	recipe, err := recipeManager.GetRecipeFromDB(th.Digest)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	fmt.Println(recipe)
	fmt.Println(blob)

	blockResponse := GetBlockResponse()
	encode.
		w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
