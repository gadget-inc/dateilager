package main

import (
	"context"
	"flag"
	"net/http"
	"strconv"

	"github.com/gadget-inc/dateilager/pkg/server"
	"github.com/gadget-inc/dateilager/pkg/web"
	"go.uber.org/zap"
)

type WebUIArgs struct {
	port  int
	dbUri string
}

func parseArgs() WebUIArgs {
	port := flag.Int("port", 5051, "web server port")
	dbUri := flag.String("dburi", "postgres://postgres@127.0.0.1:5432/dl", "Postgres URI")

	flag.Parse()

	return WebUIArgs{
		port:  *port,
		dbUri: *dbUri,
	}
}

func main() {
	ctx := context.Background()
	log, _ := zap.NewDevelopment()
	defer log.Sync()

	args := parseArgs()

	pool, err := server.NewDbPoolConnector(ctx, args.dbUri)
	if err != nil {
		log.Fatal("cannot connect to DB", zap.String("dburi", args.dbUri))
	}
	defer pool.Close()

	handler := web.NewWebServer(log, pool)

	log.Info("start webui", zap.Int("port", args.port))
	http.ListenAndServe(":"+strconv.Itoa(args.port), handler)
}
