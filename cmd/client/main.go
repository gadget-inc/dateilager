package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/gadget-inc/dateilager/pkg/client"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Command interface {
	run(context.Context, *zap.Logger, *client.Client)
}

type sharedArgs struct {
	server   *string
	level    *zapcore.Level
	encoding *string
}

func parseSharedArgs(set *flag.FlagSet) *sharedArgs {
	level := zapcore.DebugLevel

	server := set.String("server", "", "Server GRPC address")
	set.Var(&level, "log", "Log level")
	encoding := set.String("encoding", "console", "Log encoding (console | json)")

	return &sharedArgs{
		server:   server,
		level:    &level,
		encoding: encoding,
	}
}

type newArgs struct {
	id       int64
	template *int64
	patterns string
}

func parseNewArgs(args []string) (*sharedArgs, *newArgs, error) {
	set := flag.NewFlagSet("new", flag.ExitOnError)

	shared := parseSharedArgs(set)
	id := set.Int64("id", -1, "Project ID (required)")
	template := set.Int64("template", -1, "Template ID")
	patterns := set.String("patterns", "", "Comma separated pack patterns")

	set.Parse(args)

	if *id == -1 {
		return nil, nil, errors.New("required arg: -id")
	}

	if *template == -1 {
		template = nil
	}

	return shared, &newArgs{
		id:       *id,
		template: template,
		patterns: *patterns,
	}, nil
}

func (a *newArgs) run(ctx context.Context, log *zap.Logger, c *client.Client) {
	err := c.NewProject(ctx, a.id, a.template, a.patterns)
	if err != nil {
		log.Fatal("could not create new project", zap.Error(err))
	}

	log.Info("created new project", zap.Int64("id", a.id))
}

type getArgs struct {
	project int64
	vrange  client.VersionRange
	prefix  string
}

func parseGetArgs(args []string) (*sharedArgs, *getArgs, error) {
	set := flag.NewFlagSet("get", flag.ExitOnError)

	shared := parseSharedArgs(set)
	project := set.Int64("project", -1, "Project ID (required)")
	from := set.Int64("from", -1, "From version ID (optional)")
	to := set.Int64("to", -1, "To version ID (optional)")
	prefix := set.String("prefix", "", "Search prefix")

	set.Parse(args)

	if *project == -1 {
		return nil, nil, errors.New("required arg: -project")
	}

	if *from == -1 {
		from = nil
	}
	if *to == -1 {
		to = nil
	}

	return shared, &getArgs{
		project: *project,
		vrange:  client.VersionRange{From: from, To: to},
		prefix:  *prefix,
	}, nil
}

func (a *getArgs) run(ctx context.Context, log *zap.Logger, c *client.Client) {
	objects, err := c.Get(ctx, a.project, a.prefix, a.vrange)
	if err != nil {
		log.Fatal("could not fetch data", zap.Error(err))
	}

	log.Info("listing objects in project", zap.Int64("project", a.project), zap.Int("count", len(objects)))
	for _, object := range objects {
		log.Info("object", zap.String("path", object.Path), zap.String("content", string(object.Content)))
	}
}

type rebuildArgs struct {
	project int64
	vrange  client.VersionRange
	prefix  string
	output  string
}

func parseRebuildArgs(args []string) (*sharedArgs, *rebuildArgs, error) {
	set := flag.NewFlagSet("rebuild", flag.ExitOnError)

	shared := parseSharedArgs(set)
	project := set.Int64("project", -1, "Project ID (required)")
	from := set.Int64("from", -1, "From version ID (optional)")
	to := set.Int64("to", -1, "To version ID (optional)")
	prefix := set.String("prefix", "", "Search prefix")
	output := set.String("output", "", "Output directory")

	set.Parse(args)

	if *project == -1 {
		return nil, nil, errors.New("required arg: -project")
	}

	if *from == -1 {
		from = nil
	}
	if *to == -1 {
		to = nil
	}

	return shared, &rebuildArgs{
		project: *project,
		vrange:  client.VersionRange{From: from, To: to},
		prefix:  *prefix,
		output:  *output,
	}, nil
}

func (a *rebuildArgs) run(ctx context.Context, log *zap.Logger, c *client.Client) {
	version, count, err := c.Rebuild(ctx, a.project, a.prefix, a.vrange, a.output)
	if err != nil {
		log.Fatal("could not fetch data", zap.Error(err))
	}

	log.Info("wrote files", zap.Int64("project", a.project), zap.String("output", a.output), zap.Int("diff_count", count))
	fmt.Println(version)
}

type updateArgs struct {
	project   int64
	diff      string
	directory string
}

func parseUpdateArgs(args []string) (*sharedArgs, *updateArgs, error) {
	set := flag.NewFlagSet("update", flag.ExitOnError)

	shared := parseSharedArgs(set)
	project := set.Int64("project", -1, "Project ID (required)")
	diff := set.String("diff", "", "Diff file listing changed file names")
	directory := set.String("directory", "", "Directory containing updated files")

	set.Parse(args)

	if *project == -1 {
		return nil, nil, errors.New("required arg: -project")
	}

	return shared, &updateArgs{
		project:   *project,
		diff:      *diff,
		directory: *directory,
	}, nil
}

func (a *updateArgs) run(ctx context.Context, log *zap.Logger, c *client.Client) {
	version, count, err := c.Update(ctx, a.project, a.diff, a.directory)
	if err != nil {
		log.Fatal("update objects", zap.Error(err))
	}

	log.Info("updated objects", zap.Int64("project", a.project), zap.Int64("version", version), zap.Int("count", count))
	fmt.Println(version)
}

type inspectArgs struct {
	project int64
}

func parseInspectArgs(args []string) (*sharedArgs, *inspectArgs, error) {
	set := flag.NewFlagSet("inspect", flag.ExitOnError)

	shared := parseSharedArgs(set)
	project := set.Int64("project", -1, "Project ID (required)")

	set.Parse(args)

	if *project == -1 {
		return nil, nil, errors.New("required arg: -project")
	}

	return shared, &inspectArgs{
		project: *project,
	}, nil
}

func (a *inspectArgs) run(ctx context.Context, log *zap.Logger, c *client.Client) {
	inspect, err := c.Inspect(ctx, a.project)
	if err != nil {
		log.Fatal("inspect project", zap.Int64("project", a.project), zap.Error(err))
	}

	log.Info("inspect objects",
		zap.Int64("project", a.project),
		zap.Int64("latest_version", inspect.LatestVersion),
		zap.Int64("live_objects_count", inspect.LiveObjectsCount),
		zap.Int64("total_objects_count", inspect.TotalObjectsCount),
	)
}

type snapshotArgs struct{}

func parseSnapshotArgs(args []string) (*sharedArgs, *snapshotArgs, error) {
	set := flag.NewFlagSet("snapshot", flag.ExitOnError)

	shared := parseSharedArgs(set)

	set.Parse(args)

	return shared, &snapshotArgs{}, nil
}

func (a *snapshotArgs) run(ctx context.Context, log *zap.Logger, c *client.Client) {
	state, err := c.Snapshot(ctx)
	if err != nil {
		log.Fatal("snapshot", zap.Error(err))
	}

	log.Info("successful snapshot")
	fmt.Println(state)
}

type resetArgs struct {
	state string
}

func parseResetArgs(args []string) (*sharedArgs, *resetArgs, error) {
	set := flag.NewFlagSet("reset", flag.ExitOnError)

	shared := parseSharedArgs(set)
	state := set.String("state", "", "State string from a snapshot command")

	set.Parse(args)

	return shared, &resetArgs{
		state: *state,
	}, nil
}

func (a *resetArgs) run(ctx context.Context, log *zap.Logger, c *client.Client) {
	err := c.Reset(ctx, a.state)
	if err != nil {
		log.Fatal("reset", zap.String("state", a.state), zap.Error(err))
	}

	log.Info("successful reset", zap.String("state", a.state))
}

func buildLogger(level zapcore.Level, encoding string) *zap.Logger {
	config := zap.NewProductionConfig()
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	config.Level = zap.NewAtomicLevelAt(level)
	config.Encoding = encoding

	log, err := config.Build()
	if err != nil {
		panic(fmt.Sprintf("Cannot setup logger: %v", err))
	}

	return log
}

func main() {
	var shared *sharedArgs
	var cmd Command
	var err error

	switch os.Args[1] {
	case "new":
		shared, cmd, err = parseNewArgs(os.Args[2:])
	case "get":
		shared, cmd, err = parseGetArgs(os.Args[2:])
	case "rebuild":
		shared, cmd, err = parseRebuildArgs(os.Args[2:])
	case "update":
		shared, cmd, err = parseUpdateArgs(os.Args[2:])
	case "inspect":
		shared, cmd, err = parseInspectArgs(os.Args[2:])
	case "snapshot":
		shared, cmd, err = parseSnapshotArgs(os.Args[2:])
	case "reset":
		shared, cmd, err = parseResetArgs(os.Args[2:])
	default:
		err = errors.New("requires a subcommand: [new, get, rebuild, update, inspect, snapshot, reset]")
	}

	log := buildLogger(*shared.level, *shared.encoding)

	if err != nil {
		log.Fatal(err.Error())
	}

	token := os.Getenv("DL_TOKEN")
	if token == "" {
		log.Fatal("missing token: set the DL_TOKEN environment variable")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Second)
	defer cancel()

	c, err := client.NewClient(ctx, *shared.server, token)
	if err != nil {
		log.Fatal("could not connect to server", zap.String("server", *shared.server), zap.Error(err))
	}
	defer c.Close()

	cmd.run(ctx, log, c)
}
