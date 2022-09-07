package proxy

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/configuration"
	"github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
)

func TestNewRegistryPullThroughCache(t *testing.T) {
	tts := []struct {
		name string
		args struct {
			ctx      context.Context
			registry distribution.Namespace
			driver   driver.StorageDriver
			config   configuration.Proxy
		}
		want struct {
			err error
		}
	}{
		{
			name: "CacheTTL is nil",
			args: struct {
				ctx      context.Context
				registry distribution.Namespace
				driver   driver.StorageDriver
				config   configuration.Proxy
			}{ctx: context.Background(), registry: nil, driver: inmemory.New(), config: configuration.Proxy{RemoteURL: "https://registry.k8s.io"}},
			want: struct {
				err error
			}{err: nil},
		},
		{
			name: "CacheTTL is valid",
			args: struct {
				ctx      context.Context
				registry distribution.Namespace
				driver   driver.StorageDriver
				config   configuration.Proxy
			}{ctx: context.Background(), registry: nil, driver: inmemory.New(), config: configuration.Proxy{RemoteURL: "https://registry.k8s.io", CacheTTL: "48h"}},
			want: struct {
				err error
			}{err: nil},
		},
		{
			name: "CacheTTL is invalid",
			args: struct {
				ctx      context.Context
				registry distribution.Namespace
				driver   driver.StorageDriver
				config   configuration.Proxy
			}{ctx: context.Background(), registry: nil, driver: inmemory.New(), config: configuration.Proxy{RemoteURL: "https://registry.k8s.io", CacheTTL: "1d"}},
			want: struct {
				err error
			}{err: errors.New("time: unknown unit \"d\" in duration \"1d\"")},
		},
	}

	for _, tt := range tts {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewRegistryPullThroughCache(tt.args.ctx, tt.args.registry, tt.args.driver, tt.args.config)
			fmt.Println(err, tt.want.err)
			if err != tt.want.err && err.Error() != tt.want.err.Error() {
				t.Fatal(err)
			}
		})
	}
}
