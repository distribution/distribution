package main

import (
	_ "net/http/pprof"

	"github.com/goharbor/distribution/registry"
	_ "github.com/goharbor/distribution/registry/auth/htpasswd"
	_ "github.com/goharbor/distribution/registry/auth/silly"
	_ "github.com/goharbor/distribution/registry/auth/token"
	_ "github.com/goharbor/distribution/registry/proxy"
	_ "github.com/goharbor/distribution/registry/storage/driver/azure"
	_ "github.com/goharbor/distribution/registry/storage/driver/filesystem"
	_ "github.com/goharbor/distribution/registry/storage/driver/gcs"
	_ "github.com/goharbor/distribution/registry/storage/driver/inmemory"
	_ "github.com/goharbor/distribution/registry/storage/driver/middleware/cloudfront"
	_ "github.com/goharbor/distribution/registry/storage/driver/middleware/redirect"
	_ "github.com/goharbor/distribution/registry/storage/driver/oss"
	_ "github.com/goharbor/distribution/registry/storage/driver/s3-aws"
	_ "github.com/goharbor/distribution/registry/storage/driver/swift"
)

func main() {
	registry.RootCmd.Execute()
}
