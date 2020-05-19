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

// SizeOfWindow give the size of the sliding window
const SizeOfWindow = 512

// ShiftOfWindow give the size of the sliding window
const ShiftOfWindow = 512

// ScaleFactor gives the factor by which the size and shift of window is sclaed
const ScaleFactor = SizeOfWindow / ShiftOfWindow

//BlockIndices returns the start and end index of the block
func BlockIndices(i int, blobLength int) (int, int) {
	startIndex := i * ShiftOfWindow
	endIndex := startIndex + SizeOfWindow
	if endIndex > blobLength {
		endIndex = blobLength
	}
	return startIndex, endIndex

}

//RecipeManager of the image to be generated for comparision
type RecipeManager struct {
	redisPool *redis.Pool
}

//Recipe of the recipe structure
type Recipe struct {
	digest digest.Digest
	Keys   []string
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
		beginIndex = 0
	)

	dataLength := len(data)

	recipeLength := (dataLength / ShiftOfWindow)
	if dataLength%ShiftOfWindow != 0 {
		//For the last block which may be smaller than shiftOfWindow size
		recipeLength = recipeLength + 1
	}
	recipeKeys := make([]string, recipeLength)

	for i := beginIndex; i < dataLength; i = i + ShiftOfWindow {

		limit := i + SizeOfWindow
		if limit >= dataLength {
			limit = dataLength
		}
		chunk := data[i:limit]
		hashOfChunk := sha256.Sum256(chunk)

		recipeKeys[i/ShiftOfWindow] = hex.EncodeToString(hashOfChunk[:])
	}

	return Recipe{
		digest: digest,
		Keys:   recipeKeys,
	}, nil
}

//InsertRecipeInDB will insert the recipe in the db
func (rg *RecipeManager) InsertRecipeInDB(recipe Recipe) error {
	conn := rg.redisPool.Get()
	defer conn.Close()

	serialized, _ := json.Marshal(recipe)
	i, err := conn.Do("SET", rg.generateKeyForLayer(recipe.digest), serialized)
	if Debug == true {
		fmt.Println(i)
		fmt.Println(err)
	}
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

//GetRecipesFromDB will get a map of recipes from the db
func (rg *RecipeManager) GetRecipesFromDB(digests []digest.Digest) (map[digest.Digest]Recipe, error) {
	conn := rg.redisPool.Get()
	defer conn.Close()

	keys := make([]interface{}, len(digests))
	for i, digest := range digests {
		keys[i] = rg.generateKeyForLayer(digest)
	}
	serializedValues, err := redis.Values(conn.Do("MGET", keys...))
	if err != nil {
		return map[digest.Digest]Recipe{}, err
	}

	if serializedValues == nil {
		return map[digest.Digest]Recipe{}, errors.New("Key not found")
	}

	recipes := make(map[digest.Digest]Recipe)
	for i, digest := range digests {
		var r Recipe
		json.Unmarshal(serializedValues[i].([]byte), &r)
		recipes[digest] = r
	}
	return recipes, err
}

func (rg *RecipeManager) generateKeyForLayer(digest digest.Digest) string {
	return "recipe:blob:" + string(digest)
}
