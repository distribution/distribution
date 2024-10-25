package proxy

import "context"

type upstreamAuthUserKey struct{}

func WithUpstreamAuthUser(ctx context.Context, username string) context.Context {
	return context.WithValue(ctx, upstreamAuthUserKey{}, username)
}

func GetUpstreamAuthUser(ctx context.Context) string {
	return ctx.Value(upstreamAuthUserKey{}).(string)
}

type UpstreamAuthPasswordKey struct{}

func WithUpstreamAuthPassword(ctx context.Context, password string) context.Context {
	return context.WithValue(ctx, UpstreamAuthPasswordKey{}, password)
}

func GetUpstreamAuthPassword(ctx context.Context) string {
	return ctx.Value(UpstreamAuthPasswordKey{}).(string)
}
