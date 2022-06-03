package client

import (
	"context"
)

type clientCtxKey struct{}

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
