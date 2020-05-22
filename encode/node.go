package encode

import (
	"fmt"

	"github.com/garyburd/redigo/redis"
	"github.com/golang-collections/collections/set"
	"github.com/opencontainers/go-digest"
)

//InsertNodeAsSet removes the set if already exists and inserts the new set
func (emngr *EncodeManager) InsertNodeAsSet(nodeID string, keys []string) {
	conn := emngr.redisPool.Get()
	defer conn.Close()

	nodeSetKey := getNodeSetKey(nodeID)
	conn.Do("DEL", nodeSetKey)

	nodeSetArgs := make([]interface{}, len(keys)+1)
	nodeSetArgs[0] = nodeSetKey
	for i, v := range keys {
		nodeSetArgs[i+1] = v
	}
	conn.Do("SADD", nodeSetArgs...)
}

//GetAvailableBlocksFromNode will get the instersection of the node and the recipe
func (emngr *EncodeManager) GetAvailableBlocksFromNode(nodeID string, digest digest.Digest) (set.Set, error) {
	conn := emngr.redisPool.Get()
	defer conn.Close()

	nodeSetKey := getNodeSetKey(nodeID)
	if exists, _ := redis.Bool(conn.Do("EXISTS", nodeSetKey)); !exists {
		fmt.Println("Node info not available in server stash")
		return set.Set{}, fmt.Errorf("Node info not available in server stash")
	}

	recipeSetKey := getRecipeSetKey(digest)

	values, _ := redis.Values(conn.Do("SINTER", nodeSetKey, recipeSetKey))
	return *set.New(values), nil
}

func getNodeSetKey(nodeID string) string {
	return "node-set:" + nodeID
}
