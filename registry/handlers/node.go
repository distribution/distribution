package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/docker/distribution/context"
	"github.com/gorilla/handlers"
)

// recipeDispatcher responds to a request to fetch the recipe of a
// image.
func nodeDispatcher(ctx *Context, r *http.Request) http.Handler {
	nodeHandler := &nodeHandler{
		Context: ctx,
	}

	mhandler := handlers.MethodHandler{
		"POST": http.HandlerFunc(nodeHandler.UpdateNodeState),
	}

	return mhandler
}

// recipeHandler serves http blob requests.
type nodeHandler struct {
	*Context
}

//GetRecipe returns the recipe for the given digest
func (nh *nodeHandler) UpdateNodeState(w http.ResponseWriter, r *http.Request) {
	context.GetLogger(nh).Debug("UpdateState")
	nodeID := getName(nh.Context)

	decoder := json.NewDecoder(r.Body)
	var blockKeys []string
	err := decoder.Decode(&blockKeys)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}

	encodeManager := nh.EncodeManager
	encodeManager.InsertNodeAsSet(nodeID, blockKeys)
	w.WriteHeader(http.StatusOK)
}
