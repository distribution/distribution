package encode

import (
	"crypto/sha256"
	"encoding/hex"
)

//Recipe of the image to be generated for comparision
type Recipe struct {
}

//RecipePayload of the recipe structure
type RecipePayload struct {
	Recipe []string
}

//GenerateRecipe generates a new recipe from the image address provided
func GenerateRecipe(image string) (Recipe, error) {
	return Recipe{}, nil
}

//GetRecipeForImage generates a recipe and returns as Payload
func GetRecipeForImage(data []byte) (RecipePayload, error) {

	const (
		beginIndex    = 0
		sizeOfWindow  = 4096
		shiftOfWindow = 1024
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
	return RecipePayload{
		Recipe: recipe,
	}, nil
}
