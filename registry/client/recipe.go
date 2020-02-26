package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/docker/distribution/encode"
	"github.com/docker/distribution/reference"
	v2 "github.com/docker/distribution/registry/api/v2"
	"github.com/opencontainers/go-digest"
)

// RecipeFetcher holds the interface to the additional methods in relation
// to the communication of the recipe
type RecipeFetcher interface {
	//Get will fetch the image from the tag
	Get(ctx context.Context, tag string)
}

//RecipeClient is a client object to fetch the recipe
//from distribution
type RecipeClient struct {
	name   reference.Named
	ub     *v2.URLBuilder
	client *http.Client
}

//Get will fetch the recipe for the tag which provides the digest value
func (r *RecipeClient) Get(ctx context.Context, tag digest.Digest) (encode.Recipe, error) {
	ref, _ := reference.WithDigest(r.name, tag)
	url, _ := r.ub.BuildRecipeURL(ref)

	httpResponse, err := r.client.Get(url)
	if err != nil {
		return encode.Recipe{}, err
	}
	defer httpResponse.Body.Close()

	if httpResponse.StatusCode != http.StatusOK {
		fmt.Println("Did not receive OK response from Http Server.")
		return encode.Recipe{}, nil
	}

	rawRecipeByteStream, _ := ioutil.ReadAll(httpResponse.Body)

	var rcp encode.Recipe
	json.Unmarshal(rawRecipeByteStream, &rcp)

	return rcp, nil
}
