package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

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

	log, _ := zap.NewDevelopment()
	defer log.Sync()

	args := parseArgs()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Second)
	defer cancel()

	token := os.Getenv("DL_TOKEN")
	if token == "" {
		log.Fatal("missing token: set the DL_TOKEN environment variable")
	}

	c, err := client.NewClient(ctx, args.server, token)
	if err != nil {
		log.Fatal("could not connect to server", zap.String("server", args.server))
	}
	defer c.Close()

	handler, err := web.NewWebServer(log, c, args.assetsDir)
	if err != nil {
		log.Fatal("cannot setup web server", zap.Error(err))
	}

	log.Info("start webui", zap.Int("port", args.port), zap.String("assets", args.assetsDir))
	http.ListenAndServe(":"+strconv.Itoa(args.port), handler)
}
