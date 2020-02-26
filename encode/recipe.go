package encode

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/garyburd/redigo/redis"
	"github.com/opencontainers/go-digest"
)

//RecipeManager of the image to be generated for comparision
type RecipeManager struct {
	redisPool *redis.Pool
}

//Recipe of the recipe structure
type Recipe struct {
	digest digest.Digest
	Recipe []string
}

//NewRecipeManager generates the RecipeGenerator struct
func NewRecipeManager(redis *redis.Pool) RecipeManager {
	return RecipeManager{
		redisPool: redis,
	}
}

//GetRecipeForLayer generates a recipe and returns as Payload
func (rg *RecipeManager) GetRecipeForLayer(digest digest.Digest, data []byte) (Recipe, error) {

	const (
		beginIndex    = 0
		sizeOfWindow  = 1024
		shiftOfWindow = 256
	)

	dataLength := len(data)

	recipeLength := (dataLength / shiftOfWindow)
	if dataLength%shiftOfWindow != 0 {
		//For the last block which may be smaller than shiftOfWindow size
		recipeLength = recipeLength + 1
	}
	recipe := make([]string, recipeLength)

	for i := beginIndex; i < dataLength; i = i + shiftOfWindow {

		limit := i + sizeOfWindow
		if limit >= dataLength {
			limit = dataLength - 1
		}
		chunk := data[i:limit]
		hashOfChunk := sha256.Sum256(chunk)

		recipe[i/shiftOfWindow] = hex.EncodeToString(hashOfChunk[:])
	}

	return Recipe{
		digest: digest,
		Recipe: recipe,
	}, nil
}

//InsertRecipeInDB will insert the recipe in the db
func (rg *RecipeManager) InsertRecipeInDB(recipe Recipe) error {
	conn := rg.redisPool.Get()
	defer conn.Close()

	serialized, _ := json.Marshal(recipe)
	i, err := conn.Do("SET", rg.generateKeyForLayer(recipe.digest), serialized)
	fmt.Println(i)
	fmt.Println(err)

	return nil
}

//GetRecipeFromDB will insert the recipe in the db
func (rg *RecipeManager) GetRecipeFromDB(digest digest.Digest) (Recipe, error) {
	conn := rg.redisPool.Get()
	defer conn.Close()

	serialized, err := conn.Do("GET", rg.generateKeyForLayer(digest))
	if err != nil {
		return Recipe{}, err
	}

	if serialized == nil {
		return Recipe{}, errors.New("Key not found")
	}

	var r Recipe
	json.Unmarshal(serialized.([]byte), &r)
	return r, err
}

func (rg *RecipeManager) generateKeyForLayer(digest digest.Digest) string {
	return "recipe:blob:" + string(digest)
}
