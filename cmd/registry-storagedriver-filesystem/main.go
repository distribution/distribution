// +build ignore

package main

import (
	"encoding/json"
	"os"

	"github.com/Sirupsen/logrus"

	"github.com/docker/distribution/registry/storage/driver/filesystem"
	"github.com/docker/distribution/registry/storage/driver/ipc"
)

// An out-of-process filesystem driver, intended to be run by ipc.NewDriverClient
func main() {
	parametersBytes := []byte(os.Args[1])
	var parameters map[string]string
	err := json.Unmarshal(parametersBytes, &parameters)
	if err != nil {
		panic(err)
	}

	if err := ipc.StorageDriverServer(filesystem.FromParameters(parameters)); err != nil {
		logrus.Fatalln(err)
	}
}
