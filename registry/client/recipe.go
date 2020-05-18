package client

import (
	"bytes"
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

//RecipeClient is a client object to fetch the recipe
//from distribution
type recipeClient struct {
	name   reference.Named
	ub     *v2.URLBuilder
	client *http.Client
}

//Get will fetch the recipe for the tag which provides the digest value
func (r *recipeClient) Get(ctx context.Context, tag digest.Digest) (encode.Recipe, error) {
	ref, _ := reference.WithDigest(r.name, tag)
	url, _ := r.ub.BuildRecipeURL(ref)

	httpResponse, err := r.client.Get(url)
	if err != nil {
		return encode.Recipe{}, err
	}
	defer httpResponse.Body.Close()

	if httpResponse.StatusCode != http.StatusOK {
		fmt.Printf("Did not receive OK response from Http Server to fetch recipe for digest :%v. Received: %v\n", tag, httpResponse.StatusCode)
		return encode.Recipe{}, nil
	}

	rawRecipeByteStream, _ := ioutil.ReadAll(httpResponse.Body)

	var rcp encode.Recipe
	json.Unmarshal(rawRecipeByteStream, &rcp)

	return rcp, nil
}

//Get will fetch the recipe for the tag which provides the digest value
func (r *recipeClient) MGet(ctx context.Context, tags []digest.Digest) (map[digest.Digest]encode.Recipe, error) {
	url, _ := r.ub.BuildRecipesURL()

	body, _ := json.Marshal(tags)

	httpResponse, err := r.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return map[digest.Digest]encode.Recipe{}, err
	}
	defer httpResponse.Body.Close()

	if httpResponse.StatusCode != http.StatusOK {
		fmt.Println("Did not receive OK response from Http Server to fetch recipes", httpResponse.StatusCode)
		return map[digest.Digest]encode.Recipe{}, nil
	}

	rawRecipeByteStream, _ := ioutil.ReadAll(httpResponse.Body)

	var rcp map[digest.Digest]encode.Recipe
	json.Unmarshal(rawRecipeByteStream, &rcp)

	return rcp, nil
}
