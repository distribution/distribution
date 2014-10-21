package main

import (
	"github.com/docker/docker-registry/storagedriver/inmemory"
	"github.com/docker/docker-registry/storagedriver/ipc"
)

func main() {
	ipc.Server(inmemory.NewDriver())
}
