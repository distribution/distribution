// +build ignore

package main

import (
	"encoding/json"
	"os"

	"github.com/Sirupsen/logrus"

	"github.com/docker/distribution/registry/storage/driver/ipc"
	"github.com/docker/distribution/registry/storage/driver/swift"
)

// An out-of-process Swift driver, intended to be run by ipc.NewDriverClient
func main() {
	parametersBytes := []byte(os.Args[1])
	var parameters map[string]string
	err := json.Unmarshal(parametersBytes, &parameters)
	if err != nil {
		panic(err)
	}

	driver, err := swift.FromParameters(parameters)
	if err != nil {
		panic(err)
	}

	if err := ipc.StorageDriverServer(driver); err != nil {
		logrus.Fatalln(err)
	}
}
