package main

import (
	"github.com/docker/distribution/pruner"
	_ "github.com/docker/distribution/registry"
	_ "github.com/docker/distribution/registry/auth/silly"
	_ "github.com/docker/distribution/registry/storage/driver/filesystem"
)

func main() {
	pruner.Cmd.Execute()
}
