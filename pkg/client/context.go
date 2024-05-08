package client

import (
	"context"
)

type clientCtxKey struct{}
type cachedCtxKey struct{}

func FromContext(ctx context.Context) *Client {
	client, ok := ctx.Value(clientCtxKey{}).(*Client)
	if !ok {
		return nil
	}
	return client
}

func IntoContext(ctx context.Context, client *Client) context.Context {
	return context.WithValue(ctx, clientCtxKey{}, client)
}

func CachedFromContext(ctx context.Context) *CachedClient {
	client, ok := ctx.Value(cachedCtxKey{}).(*CachedClient)
	if !ok {
		return nil
	}
	return client
}

func CachedIntoContext(ctx context.Context, client *CachedClient) context.Context {
	return context.WithValue(ctx, cachedCtxKey{}, client)
}
