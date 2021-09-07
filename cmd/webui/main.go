package main

import (
	"context"
	"flag"
	"net/http"
	"strconv"
	"time"

	"github.com/gadget-inc/dateilager/pkg/client"
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
	log, _ := zap.NewDevelopment()
	defer log.Sync()

	args := parseArgs()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Second)
	defer cancel()

	c, err := client.NewClient(ctx, args.server)
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
