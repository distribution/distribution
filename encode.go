package distribution

import (
	"context"

	"github.com/docker/distribution/encode"
	"github.com/opencontainers/go-digest"
)

//RecipeService fetches the recipe from the server
type RecipeService interface {
	Get(ctx context.Context, tag digest.Digest) (encode.Recipe, error)
}
