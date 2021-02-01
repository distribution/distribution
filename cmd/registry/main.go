package main

import (
	_ "net/http/pprof"

	"github.com/distribution/distribution/registry"
	_ "github.com/distribution/distribution/registry/auth/htpasswd"
	_ "github.com/distribution/distribution/registry/auth/silly"
	_ "github.com/distribution/distribution/registry/auth/token"
	_ "github.com/distribution/distribution/registry/proxy"
	_ "github.com/distribution/distribution/registry/storage/driver/azure"
	_ "github.com/distribution/distribution/registry/storage/driver/filesystem"
	_ "github.com/distribution/distribution/registry/storage/driver/gcs"
	_ "github.com/distribution/distribution/registry/storage/driver/inmemory"
	_ "github.com/distribution/distribution/registry/storage/driver/middleware/alicdn"
	_ "github.com/distribution/distribution/registry/storage/driver/middleware/cloudfront"
	_ "github.com/distribution/distribution/registry/storage/driver/middleware/redirect"
	_ "github.com/distribution/distribution/registry/storage/driver/oss"
	_ "github.com/distribution/distribution/registry/storage/driver/s3-aws"
	_ "github.com/distribution/distribution/registry/storage/driver/swift"
)

func main() {
	registry.RootCmd.Execute()
}
