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

func (b *blocksClient) Exchange(ctx context.Context, tag digest.Digest, d encode.Declaration) (encode.BlockResponse, int, string, error) {
	ref, _ := reference.WithDigest(b.name, tag)
	url, _ := b.ub.BuildBlocksURL(ref)

	httpResponse, _ := b.client.Post(url, "application/text", strings.NewReader(d.String()))
	headerLength, _ := strconv.Atoi(httpResponse.Header.Get("header-length"))
	blockLength, _ := strconv.Atoi(httpResponse.Header.Get("block-length"))
	checksum := httpResponse.Header.Get("hash-length")

	fmt.Println("Header-length: ", headerLength)
	fmt.Println("block-length: ", blockLength)
	fmt.Println("block-checksum: ", checksum)

	byteStream, _ := ioutil.ReadAll(httpResponse.Body)
	return encode.GetBlockResponseFromByteStream(headerLength, byteStream), blockLength, checksum, nil
}
