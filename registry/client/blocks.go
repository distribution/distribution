package client

import (
	"context"
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

func (b *blocksClient) Exchange(ctx context.Context, tag digest.Digest, d encode.Declaration) (encode.BlockResponse, error) {
	ref, _ := reference.WithDigest(b.name, tag)
	url, _ := b.ub.BuildBlocksURL(ref)

	httpResponse, _ := b.client.Post(url, "application/text", strings.NewReader(d.String()))
	length, _ := strconv.Atoi(httpResponse.Header.Get("length"))
	var byteStream []byte
	_, _ = httpResponse.Body.Read(byteStream) //Qn: ? Is it
	return encode.GetBlockResponseFromByteStream(length, byteStream), nil
}
