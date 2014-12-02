package main

import (
	"encoding/json"
	"os"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/docker-registry/storagedriver/azure"
	"github.com/docker/docker-registry/storagedriver/ipc"
)

// An out-of-process Azure Storage driver, intended to be run by ipc.NewDriverClient
func main() {
	parametersBytes := []byte(os.Args[1])
	var parameters map[string]string
	err := json.Unmarshal(parametersBytes, &parameters)
	if err != nil {
		panic(err)
	}

	driver, err := azure.FromParameters(parameters)
	if err != nil {
		panic(err)
	}

	if err := ipc.StorageDriverServer(driver); err != nil {
		log.Fatalln("driver error:", err)
	}
}
