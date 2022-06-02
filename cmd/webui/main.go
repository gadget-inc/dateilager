package main

import (
	"context"
	"flag"
	"fmt"
	stdlog "log"
	"net/http"
	"os"
	"strconv"

	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/gadget-inc/dateilager/pkg/version"
	"github.com/gadget-inc/dateilager/pkg/web"
	"go.uber.org/zap"
)

type WebUIArgs struct {
	port      int
	server    string
	assetsDir string
}

func parseArgs() WebUIArgs {
	port := flag.Int("port", 3333, "web server port")
	server := flag.String("server", "localhost:5051", "GRPC server address and port")
	assetsDir := flag.String("assets", "assets", "Assets directory")

	flag.Parse()

	return WebUIArgs{
		port:      *port,
		server:    *server,
		assetsDir: *assetsDir,
	}
}

func main() {
	if os.Args[1] == "version" {
		fmt.Println(version.Version)
		return
	}

	err := logger.Init(zap.NewDevelopmentConfig())
	if err != nil {
		stdlog.Fatal(err.Error())
	}
	defer logger.Sync() //nolint:errcheck

	args := parseArgs()

	ctx := context.Background()

	token := os.Getenv("DL_TOKEN")
	if token == "" {
		tokenFile := os.Getenv("DL_TOKEN_FILE")
		if tokenFile == "" {
			logger.Fatal(ctx, "missing token: set the DL_TOKEN or DL_TOKEN_FILE environment variable")
		}

		bytes, err := os.ReadFile(tokenFile)
		if err != nil {
			logger.Fatal(ctx, "failed to read contents of DL_TOKEN_FILE", zap.Error(err))
		}

		token = string(bytes)
		logger.Fatal(ctx, "missing token: set the DL_TOKEN environment variable")
	}

	c, err := client.NewClient(ctx, args.server, token)
	if err != nil {
		logger.Fatal(ctx, "could not connect to server", zap.String("server", args.server))
	}
	defer c.Close()

	handler, err := web.NewWebServer(ctx, c, args.assetsDir)
	if err != nil {
		logger.Fatal(ctx, "cannot setup web server", zap.Error(err))
	}

	logger.Info(ctx, "start webui", zap.Int("port", args.port), zap.String("assets", args.assetsDir))
	err = http.ListenAndServe(":"+strconv.Itoa(args.port), handler)
	if err != nil {
		logger.Fatal(ctx, "starting server", zap.Error(err))
	}
}
