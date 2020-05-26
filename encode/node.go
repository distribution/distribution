package encode

import (
	"fmt"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/opencontainers/go-digest"
)

//InsertNodeAsSet removes the set if already exists and inserts the new set
func (emngr *EncodeManager) InsertNodeAsSet(nodeID string, keys []string) {
	conn := emngr.redisPool.Get()
	defer conn.Close()

	//TODO: Add code to delete previous values
	BatchSize := 5000

	for i := 0; i < len(keys); i = i + BatchSize {
		startIndex := i
		endIndex := i + BatchSize
		if endIndex > len(keys) {
			endIndex = len(keys)
		}

		batchKeys := keys[startIndex:endIndex]
		values := make([]interface{}, 2*len(batchKeys))
		for j := range batchKeys {
			values[2*j] = getNodeIdentifier(nodeID, batchKeys[j])
			values[2*j+1] = ""
		}

		result, _ := conn.Do("MSET", values...)
		fmt.Println("Result:", result)
	}

}

//GetAvailableBlocksFromNode will get the instersection of the node and the recipe
func (emngr *EncodeManager) GetAvailableBlocksFromNode(nodeID string, recipe Recipe, digest digest.Digest) (Declaration, error) {
	conn := emngr.redisPool.Get()
	defer conn.Close()

	dbKeys := make([]interface{}, len(recipe.Keys))
	for i := range dbKeys {
		dbKeys[i] = getNodeIdentifier(nodeID, recipe.Keys[i])
	}

	var d Declaration
	d.Encodings = make([]bool, len(recipe.Keys))

	startFetching := time.Now()
	BatchSize := 5000
	for i := 0; i < len(recipe.Keys); i = i + BatchSize {
		startIndex := i
		endIndex := i + BatchSize
		if endIndex > len(recipe.Keys) {
			endIndex = len(recipe.Keys)
		}

		batchKeys := recipe.Keys[startIndex:endIndex]
		dbKeys := make([]interface{}, len(batchKeys))
		for i := range dbKeys {
			dbKeys[i] = getNodeIdentifier(nodeID, recipe.Keys[i])
		}
		values, _ := redis.Values(conn.Do("MGET", dbKeys...))
		if len(values) != len(dbKeys) {
			panic("serverles--->STOP 😒")
		}
		if Debug {
			fmt.Println("serverless---> MGET Values:", len(values))
		}
		for j, v := range values {
			d.Encodings[i+j] = (v != nil)
		}
	}
	PerfLog(fmt.Sprintf("Time to get retreive encodings for layer %s is %s", digest, time.Since(startFetching)))
	return d, nil
}

func getNodeIdentifier(nodeID string, hashID string) string {
	return "node-set:" + nodeID + ":" + hashID
}
