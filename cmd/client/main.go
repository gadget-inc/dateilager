package main

import (
	"context"
	"flag"
	"io/ioutil"
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

func getLatest(ctx context.Context, log *zap.Logger, c *client.Client, project int32) {
	objects, err := c.GetLatestRoot(ctx, project)
	if err != nil {
		log.Fatal("could not fetch data", zap.Error(err))
	}

	log.Info("listing objects in project", zap.Int32("project", project))
	for _, object := range objects {
		log.Info("object", zap.String("path", object.Path), zap.String("contents", string(object.Contents)))
	}
}

func updateProject(ctx context.Context, log *zap.Logger, c *client.Client, project int32, diff, prefix string) {
	content, err := ioutil.ReadFile(diff)
	if err != nil {
		log.Fatal("cannot read update file", zap.String("diff", diff), zap.Error(err))
	}

	filePaths := strings.Split(string(content), "\n")
	version, err := c.UpdateObjects(ctx, project, filePaths, prefix)
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
	case "get":
		getLatest(ctx, log, c, args.project)
	case "update":
		if len(args.args) != 2 {
			log.Fatal("invalid update args", zap.Any("args", args.args))
		}

		updateProject(ctx, log, c, args.project, args.args[0], args.args[1])
	default:
		log.Fatal("unknown command", zap.String("command", args.cmd))
	}
}
