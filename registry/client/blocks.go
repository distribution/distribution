package client

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	encode "github.com/docker/distribution/encode"
	"github.com/docker/distribution/reference"
	v2 "github.com/docker/distribution/registry/api/v2"
	"github.com/opencontainers/go-digest"
)

type blocksClient struct {
	name   reference.Named
	ub     *v2.URLBuilder
	client *http.Client
}

func (b *blocksClient) Exchange(ctx context.Context, tag digest.Digest) (encode.BlockResponse, []string, int, error) {
	ref, _ := reference.WithDigest(b.name, tag)
	url, _ := b.ub.BuildBlocksURL(ref)

	r, _ := http.NewRequest("POST", url, strings.NewReader("")) // URL-encoded payload
	r.Header.Add("Content-Type", "application/text")
	r.Header.Add("node-id", "node-x")

	httpResponse, _ := b.client.Do(r)
	headerLength, _ := strconv.Atoi(httpResponse.Header.Get("header-length"))
	blockLength, _ := strconv.Atoi(httpResponse.Header.Get("block-length"))
	byteStream, _ := ioutil.ReadAll(httpResponse.Body)

	if encode.Debug == true {
		fmt.Println("Header-length: ", headerLength)
		fmt.Println("Amount of bytes received: ", len(byteStream))
		fmt.Println("Block-length: ", blockLength)
	}

	blockResponse, blockKeys := encode.GetBlockResponseFromByteStream(headerLength, byteStream)
	return blockResponse, blockKeys, blockLength, nil
}
