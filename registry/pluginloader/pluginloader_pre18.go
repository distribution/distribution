// +build !go1.8

package pluginloader

import (
	"fmt"

	"github.com/docker/distribution/context"
)

func LoadPlugins(ctx context.Context, paths []string) error {
	return fmt.Errorf("only golang >= 1.8 supports dynamic plugins")
}
