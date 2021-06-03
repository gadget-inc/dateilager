package main

import (
	"context"
	"flag"
	"io/ioutil"
	"strconv"
	"strings"
	"time"

	"github.com/angelini/dateilager/pkg/client"

	"go.uber.org/zap"
)

type ClientArgs struct {
	server  string
	project int32
	cmd     string
	args    []string
}

func parseArgs(log *zap.Logger) ClientArgs {
	server := flag.String("server", "", "GRPC server address")
	project := flag.Int("project", -1, "Project ID")

	flag.Parse()

	if *project == -1 {
		log.Fatal("invalid project", zap.Int("project", *project))
	}

	return ClientArgs{
		server:  *server,
		project: int32(*project),
		cmd:     flag.Arg(0),
		args:    flag.Args()[1:],
	}
}

func get(ctx context.Context, log *zap.Logger, c *client.Client, project int32, prefix string, version *int64) {
	objects, err := c.Get(ctx, project, prefix, version)
	if err != nil {
		log.Fatal("could not fetch data", zap.Error(err))
	}

	log.Info("listing objects in project", zap.Int32("project", project))
	for _, object := range objects {
		log.Info("object", zap.String("path", object.Path), zap.String("contents", string(object.Contents)))
	}
}

func update(ctx context.Context, log *zap.Logger, c *client.Client, project int32, diff, prefix string) {
	content, err := ioutil.ReadFile(diff)
	if err != nil {
		log.Fatal("cannot read update file", zap.String("diff", diff), zap.Error(err))
	}

	filePaths := strings.Split(string(content), "\n")
	version, err := c.Update(ctx, project, filePaths, prefix)
	if err != nil {
		log.Fatal("update objects", zap.Error(err))
	}

	log.Info("updated objects", zap.Int("count", len(filePaths)), zap.Int64("version", version))
}

func main() {
	log, _ := zap.NewDevelopment()
	defer log.Sync()

	args := parseArgs(log)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, err := client.NewClient(ctx, args.server)
	if err != nil {
		log.Fatal("could not connect to server", zap.String("server", args.server))
	}
	defer c.Close()

	switch args.cmd {
	case "get-latest":
		prefix := ""
		if len(args.args) == 1 {
			prefix = args.args[0]
		}
		get(ctx, log, c, args.project, prefix, nil)

	case "get-version":
		if len(args.args) == 0 || len(args.args) > 2 {
			log.Fatal("invalid get-version args", zap.Any("args", args.args))
		}

		version, err := strconv.ParseInt(args.args[0], 10, 64)
		if err != nil {
			log.Fatal("invalid version", zap.String("version", args.args[0]))
		}

		prefix := ""
		if len(args.args) == 2 {
			prefix = args.args[1]
		}

		get(ctx, log, c, args.project, prefix, &version)

	case "update":
		if len(args.args) != 2 {
			log.Fatal("invalid update args", zap.Any("args", args.args))
		}
		update(ctx, log, c, args.project, args.args[0], args.args[1])

	default:
		log.Fatal("unknown command", zap.String("command", args.cmd))
	}
}
