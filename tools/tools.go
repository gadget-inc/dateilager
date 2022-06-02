package tools

// This package imports things required by build scripts, to force `go mod` to see them as dependencies
import (
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"
)
