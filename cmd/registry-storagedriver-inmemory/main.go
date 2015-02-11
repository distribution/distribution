// +build ignore

package main

import (
	"github.com/Sirupsen/logrus"
	"github.com/docker/distribution/registry/storage/driver/inmemory"
	"github.com/docker/distribution/registry/storage/driver/ipc"
)

// An out-of-process inmemory driver, intended to be run by ipc.NewDriverClient
// This exists primarily for example and testing purposes
func main() {
	if err := ipc.StorageDriverServer(inmemory.New()); err != nil {
		logrus.Fatalln(err)
	}
}
