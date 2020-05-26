package handlers

import (
	"crypto/sha256"
	"encoding/hex"
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
		getBlob <- blob
	}()

	emgr := th.EncodeManager
	recipe, _ := emgr.GetRecipeFromDB(th.Digest)
	declaration, _ := emgr.GetAvailableBlocksFromNode(nodeID, recipe, th.Digest)
	encode.PerfLog(fmt.Sprintf("Handled request to request for layer: %s in time %s", th.Digest, time.Since(start)))

	blob := <-getBlob

	encode.PerfLog(fmt.Sprintf("Handled request to get blob for layer: %s in time %s", th.Digest, time.Since(start)))

	blockResponse := encode.AssembleBlockResponse(declaration, recipe, blob)
	data, headerLength := encode.ConvertBlockResponseToByteStream(blockResponse)
	checksum := sha256.Sum256(blob)
	w.Header().Set("header-length", strconv.Itoa(headerLength))
	w.Header().Set("block-length", strconv.Itoa(len(blob)))
	w.Header().Set("hash-length", hex.EncodeToString(checksum[:]))
	if encode.Debug == true {
		fmt.Println("Blob", blob)
	}
	fmt.Printf("serverless==> Sending blob for layer %s with size %d. Header length: %d.\n", th.Digest, len(data), headerLength)
	w.Write(data)
	encode.PerfLog(fmt.Sprintf("Handled request to fetch layer: %s in time %s", th.Digest, time.Since(start)))
}
