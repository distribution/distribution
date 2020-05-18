package handlers

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/docker/distribution/context"
	"github.com/docker/distribution/encode"
	"github.com/gorilla/handlers"
	"github.com/opencontainers/go-digest"
)

// recipesDispatcher responds to a request to fetch the recipe of a
// image.
func recipesDispatcher(ctx *Context, r *http.Request) http.Handler {
	recipesHandler := &recipesHandler{
		Context: ctx,
	}

	mhandler := handlers.MethodHandler{
		"POST": http.HandlerFunc(recipesHandler.GetRecipes),
	}

	return mhandler
}

// recipeHandler serves http blob requests.
type recipesHandler struct {
	*Context
}

//GetRecipe returns the recipe for the given digest
func (rh *recipesHandler) GetRecipes(w http.ResponseWriter, r *http.Request) {
	context.GetLogger(rh).Debug("GetRecipes")
	recipeManager := rh.RecipeManager

	var listOfDigests []digest.Digest
	rawListOfDigests, _ := ioutil.ReadAll(r.Body)
	json.Unmarshal(rawListOfDigests, &listOfDigests)

	recipes := make(map[digest.Digest]encode.Recipe)
	for _, digest := range listOfDigests {
		recipe, err := recipeManager.GetRecipeFromDB(digest)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			continue
		}
		recipes[digest] = recipe
	}
	// recipeManager.InsertRecipeInDB(recipe)

	//Add code to fetch and generate the handler
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(recipes)
}
