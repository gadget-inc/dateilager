package client

import (
	"errors"
	"fmt"
	"os"
)

func getToken() (string, error) {
	token := os.Getenv("DL_TOKEN")
	if token == "" {
		tokenFile := os.Getenv("DL_TOKEN_FILE")
		if tokenFile == "" {
			return "", errors.New("missing token: set the DL_TOKEN or DL_TOKEN_FILE environment variable")
		}

		bytes, err := os.ReadFile(tokenFile)
		if err != nil {
			return "", fmt.Errorf("failed to read contents of DL_TOKEN_FILE: %w", err)
		}

		token = string(bytes)
	}

	return token, nil
}
