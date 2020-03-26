package handlers

import (
	"crypto/sha256"
	"encoding/json"
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

// RequestBlocks returns the recipe for the given digest
func (th *blocksHandler) RequestBlocks(w http.ResponseWriter, r *http.Request) {
	context.GetLogger(th).Debug("RequestBlocks")

	rawDeclaration, _ := ioutil.ReadAll(r.Body)
	declaration := encode.NewDeclarationFromString(string(rawDeclaration))

	blobStore := th.Repository.Blobs(th)
	blob, _ := blobStore.Get(th, th.Digest)
	checksum := sha256.Sum256(blob)

	blockResponse := encode.AssembleBlockResponse(declaration, blob)
	data, headerLength := encode.ConvertBlockResponseToByteStream(blockResponse)

	w.Header().Set("header-length", string(headerLength))
	w.Header().Set("block-length", string(len(blob)))
	w.Header().Set("hash-length", string(checksum[:]))
	json.NewEncoder(w).Encode(data)
}
