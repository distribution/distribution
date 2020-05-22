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

//BlockIndices returns the start and end index of the block
func BlockIndices(i int, blobLength int) (int, int) {
	startIndex := i * SizeOfWindow
	endIndex := startIndex + SizeOfWindow
	if endIndex > blobLength {
		endIndex = blobLength
	}
	return startIndex, endIndex

}

//EncodeManager of the image to be generated for comparision
type EncodeManager struct {
	redisPool *redis.Pool
}

//Recipe of the recipe structure
type Recipe struct {
	digest digest.Digest
	Keys   []string
}

//NewEncodeManager generates the RecipeGenerator struct
func NewEncodeManager(redis *redis.Pool) EncodeManager {
	return EncodeManager{
		redisPool: redis,
	}
}

//GetRecipeForLayer generates a recipe and returns as Payload
func (emngr *EncodeManager) GetRecipeForLayer(digest digest.Digest, data []byte) (Recipe, error) {

	const (
		beginIndex = 0
	)

	dataLength := len(data)

	recipeLength := (dataLength / SizeOfWindow)
	if dataLength%SizeOfWindow != 0 {
		//For the last block which may be smaller than shiftOfWindow size
		recipeLength = recipeLength + 1
	}
	recipeKeys := make([]string, recipeLength)

	for i := beginIndex; i < dataLength; i = i + SizeOfWindow {

		limit := i + SizeOfWindow
		if limit >= dataLength {
			limit = dataLength
		}
		chunk := data[i:limit]
		hashOfChunk := sha256.Sum256(chunk)

		recipeKeys[i/SizeOfWindow] = hex.EncodeToString(hashOfChunk[:])
	}

	return Recipe{
		digest: digest,
		Keys:   recipeKeys,
	}, nil
}

//InsertRecipeInDB will insert the recipe in the db
func (emngr *EncodeManager) InsertRecipeInDB(recipe Recipe) error {
	conn := emngr.redisPool.Get()
	defer conn.Close()

	serialized, _ := json.Marshal(recipe)
	i, err := conn.Do("SET", generateKeyForLayer(recipe.digest), serialized)
	if Debug == true {
		fmt.Println(i)
		fmt.Println(err)

	}

	recipeSetArgs := make([]interface{}, len(recipe.Keys)+1)
	recipeSetArgs[0] = getRecipeSetKey(recipe.digest)
	for i, v := range recipe.Keys {
		recipeSetArgs[i+1] = v
	}

	conn.Do("SADD", recipeSetArgs...)
	return nil
}

//GetRecipeFromDB will insert the recipe in the db
func (emngr *EncodeManager) GetRecipeFromDB(digest digest.Digest) (Recipe, error) {
	conn := emngr.redisPool.Get()
	defer conn.Close()

	serialized, err := conn.Do("GET", generateKeyForLayer(digest))
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
func (emngr *EncodeManager) GetRecipesFromDB(digests []digest.Digest) (map[digest.Digest]Recipe, error) {
	conn := emngr.redisPool.Get()
	defer conn.Close()

	keys := make([]interface{}, len(digests))
	for i, digest := range digests {
		keys[i] = generateKeyForLayer(digest)
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

func generateKeyForLayer(digest digest.Digest) string {
	return "recipe:blob:" + string(digest)
}

func getRecipeSetKey(digest digest.Digest) string {
	return "recipe-set:" + string(digest)
}
