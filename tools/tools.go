package tools

//nolint // This package imports things required by build scripts, to force `go mod` to see them as dependencies
import (
	_ "github.com/bojand/ghz/cmd/ghz"
	_ "github.com/golang-migrate/migrate/v4/cmd/migrate"
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"
	_ "github.com/grpc-ecosystem/grpc-health-probe"
)
