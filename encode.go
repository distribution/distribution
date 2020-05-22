package distribution

import (
	"context"

	"github.com/docker/distribution/encode"
	"github.com/opencontainers/go-digest"
)

//RecipeService fetches the recipe from the server
type RecipeService interface {
	Get(ctx context.Context, tag digest.Digest) (encode.Recipe, error)
	MGet(ctx context.Context, tags []digest.Digest) (map[digest.Digest]encode.Recipe, error)
}

//BlockService fetches the blocks from the service
type BlockService interface {
	Exchange(ctx context.Context, tag digest.Digest) (encode.BlockResponse, []string, int, string, error)
}
