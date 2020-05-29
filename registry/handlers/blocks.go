package handlers

import (
	"fmt"
	"time"

	"net/http"
	"strconv"

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
	start := time.Now()
	context.GetLogger(th).Debug("RequestBlocks")
	nodeID := r.Header.Get("node-id")

	getBlob := make(chan []byte)
	go func() {
		blobStore := th.Repository.Blobs(th)
		blob, _ := blobStore.Get(th, th.Digest)
		encode.PerfLog(fmt.Sprintf("Fetched blob from filesystem for layer: %s in time %s", th.Digest, time.Since(start)))

		getBlob <- blob
	}()

	emgr := th.EncodeManager
	recipe, _ := emgr.GetRecipeFromDB(th.Digest)
	declaration, _ := emgr.GetAvailableBlocksFromNode(nodeID, recipe, th.Digest)
	encode.PerfLog(fmt.Sprintf("Generated declaration for layer: %s in time %s", th.Digest, time.Since(start)))

	blob := <-getBlob
	encode.PerfLog(fmt.Sprintf("Point of synchronization for declaration generation and recipe generation for layer: %s in time %s", th.Digest, time.Since(start)))

	blockResponse := encode.AssembleBlockResponse(declaration, recipe, blob)
	encode.PerfLog(fmt.Sprintf("Time to assemble block response for layer: %s in time %s", th.Digest, time.Since(start)))

	data, headerLength := encode.ConvertBlockResponseToByteStream(blockResponse)
	encode.PerfLog(fmt.Sprintf("Generated response for layer: %s in time %s", th.Digest, time.Since(start)))
	w.Header().Set("header-length", strconv.Itoa(headerLength))
	w.Header().Set("block-length", strconv.Itoa(len(blob)))
	w.Header().Set("hash-length", "") //TODO: Remove
	if encode.Debug == true {
		fmt.Println("Blob", blob)
	}

	w.Write(data)
	encode.PerfLog(fmt.Sprintf("Sending Data for layer %s with size %d. Header length: %d. Time taken to handle request: %s\n", th.Digest, len(data), headerLength, time.Since(start)))
}
