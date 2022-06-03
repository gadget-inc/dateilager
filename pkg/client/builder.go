package client

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

type ClientBuilder struct {
	server string
}

func (b *ClientBuilder) AddPersistentFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVar(&b.server, "server", "", "Server GRPC address")
}

func (b *ClientBuilder) Build(ctx context.Context) (*Client, error) {
	token := os.Getenv("DL_TOKEN")
	if token == "" {
		tokenFile := os.Getenv("DL_TOKEN_FILE")
		if tokenFile == "" {
			return nil, errors.New("missing token: set the DL_TOKEN or DL_TOKEN_FILE environment variable")
		}

		bytes, err := os.ReadFile(tokenFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read contents of DL_TOKEN_FILE: %w", err)
		}

		token = string(bytes)
	}

	client, err := NewClient(ctx, b.server, token)
	if err != nil {
		return nil, fmt.Errorf("could not connect to server %v: %w", b.server, err)
	}

	return client, err
}
