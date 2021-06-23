package main

import (
	"context"
	"flag"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/angelini/dateilager/pkg/client"

	"go.uber.org/zap"
)

type Command interface {
	run(context.Context, *zap.Logger, *client.Client)
	serverAddr() string
}

type getArgs struct {
	server  string
	project int32
	vrange  client.VersionRange
	prefix  string
}

func parseGetArgs(log *zap.Logger, args []string) *getArgs {
	set := flag.NewFlagSet("get", flag.ExitOnError)

	server := set.String("server", "", "Server GRPC address")
	project := set.Int("project", -1, "Project ID (required)")
	from := set.Int64("from", -1, "From version ID (optional)")
	to := set.Int64("to", -1, "To version ID (optional)")
	prefix := set.String("prefix", "", "Search prefix")

	set.Parse(args)

	if *project == -1 {
		log.Fatal("--project required")
	}

	if *from == -1 {
		from = nil
	}
	if *to == -1 {
		to = nil
	}

	return &getArgs{
		server:  *server,
		project: int32(*project),
		vrange:  client.VersionRange{From: from, To: to},
		prefix:  *prefix,
	}
}

func (a *getArgs) serverAddr() string {
	return a.server
}

func (a *getArgs) run(ctx context.Context, log *zap.Logger, c *client.Client) {
	objects, err := c.Get(ctx, a.project, a.prefix, a.vrange)
	if err != nil {
		log.Fatal("could not fetch data", zap.Error(err))
	}

	log.Info("listing objects in project", zap.Int32("project", a.project), zap.Int("count", len(objects)))
	for _, object := range objects {
		log.Info("object", zap.String("path", object.Path), zap.String("content", string(object.Content)))
	}
}

type rebuildArgs struct {
	server  string
	project int32
	vrange  client.VersionRange
	prefix  string
	output  string
}

func parseRebuildArgs(log *zap.Logger, args []string) *rebuildArgs {
	set := flag.NewFlagSet("rebuild", flag.ExitOnError)

	server := set.String("server", "", "Server GRPC address")
	project := set.Int("project", -1, "Project ID (required)")
	from := set.Int64("from", -1, "From version ID (optional)")
	to := set.Int64("to", -1, "To version ID (optional)")
	prefix := set.String("prefix", "", "Search prefix")
	output := set.String("output", "", "Output directory")

	set.Parse(args)

	if *project == -1 {
		log.Fatal("--project required")
	}

	if *from == -1 {
		from = nil
	}
	if *to == -1 {
		to = nil
	}

	return &rebuildArgs{
		server:  *server,
		project: int32(*project),
		vrange:  client.VersionRange{From: from, To: to},
		prefix:  *prefix,
		output:  *output,
	}
}

func (a *rebuildArgs) serverAddr() string {
	return a.server
}

func (a *rebuildArgs) run(ctx context.Context, log *zap.Logger, c *client.Client) {
	err := c.Rebuild(ctx, a.project, a.prefix, a.vrange, a.output)
	if err != nil {
		log.Fatal("could not fetch data", zap.Error(err))
	}

	log.Info("wrote files", zap.Int32("project", a.project), zap.String("output", a.output))
}

type updateArgs struct {
	server    string
	project   int32
	diff      string
	directory string
}

func parseUpdateArgs(log *zap.Logger, args []string) *updateArgs {
	set := flag.NewFlagSet("update", flag.ExitOnError)

	server := set.String("server", "", "Server GRPC address")
	project := set.Int("project", -1, "Project ID (required)")
	diff := set.String("diff", "", "Diff file listing changed file names")
	directory := set.String("directory", "", "Directory containing updated files")

	set.Parse(args)

	if *project == -1 {
		log.Fatal("--project required")
	}

	return &updateArgs{
		server:    *server,
		project:   int32(*project),
		diff:      *diff,
		directory: *directory,
	}
}

func (a *updateArgs) serverAddr() string {
	return a.server
}

func (a *updateArgs) run(ctx context.Context, log *zap.Logger, c *client.Client) {
	content, err := ioutil.ReadFile(a.diff)
	if err != nil {
		log.Fatal("cannot read update file", zap.String("diff", a.diff), zap.Error(err))
	}

	filePaths := strings.Split(string(content), "\n")
	version, err := c.Update(ctx, a.project, filePaths, a.directory)
	if err != nil {
		log.Fatal("update objects", zap.Error(err))
	}

	log.Info("updated objects", zap.Int32("project", a.project), zap.Int64("version", version), zap.Int("count", len(filePaths)))
}

type packArgs struct {
	server  string
	project int32
	path    string
}

func parsePackArgs(log *zap.Logger, args []string) *packArgs {
	set := flag.NewFlagSet("update", flag.ExitOnError)

	server := set.String("server", "", "Server GRPC address")
	project := set.Int("project", -1, "Project ID (required)")
	path := set.String("path", "", "Root of the object path to pack")

	set.Parse(args)

	if *project == -1 {
		log.Fatal("--project required")
	}

	return &packArgs{
		server:  *server,
		project: int32(*project),
		path:    *path,
	}
}

func (a *packArgs) serverAddr() string {
	return a.server
}

func (a *packArgs) run(ctx context.Context, log *zap.Logger, c *client.Client) {
	version, err := c.Pack(ctx, a.project, a.path)
	if err != nil {
		log.Fatal("pack objects", zap.Int32("project", a.project), zap.String("path", a.path), zap.Error(err))
	}

	log.Info("packed objects", zap.Int32("project", a.project), zap.String("path", a.path), zap.Int64("version", version))
}

func main() {
	log, _ := zap.NewDevelopment()
	defer log.Sync()

	if len(os.Args) < 2 {
		log.Fatal("requires a subcommand: [get, rebuild, update]")
	}

	var cmd Command

	switch os.Args[1] {
	case "get":
		cmd = parseGetArgs(log, os.Args[2:])
	case "rebuild":
		cmd = parseRebuildArgs(log, os.Args[2:])
	case "update":
		cmd = parseUpdateArgs(log, os.Args[2:])
	case "pack":
		cmd = parsePackArgs(log, os.Args[2:])
	default:
		log.Fatal("requires a subcommand: [get, rebuild, update]")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, err := client.NewClient(ctx, cmd.serverAddr())
	if err != nil {
		log.Fatal("could not connect to server", zap.String("server", cmd.serverAddr()))
	}
	defer c.Close()

	cmd.run(ctx, log, c)
}
