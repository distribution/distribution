package dcontext

import "context"

type registryHostKey struct{}

func (registryHostKey) String() string { return "registryHost" }

func WithRegistryHost(ctx context.Context, host string) context.Context {
	return context.WithValue(ctx, registryHostKey{}, host)
}

func GetRegistryHost(ctx context.Context) string {
	return GetStringValue(ctx, registryHostKey{})
}
