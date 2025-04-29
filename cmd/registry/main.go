package main

import (
	_ "net/http/pprof"

	"github.com/2DFS/2dfs-registry/v3/registry"
	_ "github.com/2DFS/2dfs-registry/v3/registry/auth/htpasswd"
	_ "github.com/2DFS/2dfs-registry/v3/registry/auth/silly"
	_ "github.com/2DFS/2dfs-registry/v3/registry/auth/token"
	_ "github.com/2DFS/2dfs-registry/v3/registry/proxy"
	_ "github.com/2DFS/2dfs-registry/v3/registry/storage/driver/azure"
	_ "github.com/2DFS/2dfs-registry/v3/registry/storage/driver/filesystem"
	_ "github.com/2DFS/2dfs-registry/v3/registry/storage/driver/gcs"
	_ "github.com/2DFS/2dfs-registry/v3/registry/storage/driver/inmemory"
	_ "github.com/2DFS/2dfs-registry/v3/registry/storage/driver/middleware/cloudfront"
	_ "github.com/2DFS/2dfs-registry/v3/registry/storage/driver/middleware/redirect"
	_ "github.com/2DFS/2dfs-registry/v3/registry/storage/driver/middleware/rewrite"
	_ "github.com/2DFS/2dfs-registry/v3/registry/storage/driver/s3-aws"
)

func main() {
	// NOTE(milosgajdos): if the only two commands registered
	// with registry.RootCmd fail they will halt the program
	// execution and  exit the program with non-zero exit code.
	// nolint:errcheck
	registry.RootCmd.Execute()
}
