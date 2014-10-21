package main

import (
	"encoding/json"
	"os"

	"github.com/docker/docker-registry/storagedriver/filesystem"
	"github.com/docker/docker-registry/storagedriver/ipc"
)

func main() {
	parametersBytes := []byte(os.Args[1])
	var parameters map[string]interface{}
	err := json.Unmarshal(parametersBytes, &parameters)
	if err != nil {
		panic(err)
	}
	rootDirectory := "/tmp/registry"
	if parameters != nil {
		rootDirParam, ok := parameters["RootDirectory"].(string)
		if ok && rootDirParam != "" {
			rootDirectory = rootDirParam
		}
	}
	ipc.Server(filesystem.NewDriver(rootDirectory))
}
