package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	stdlog "log"
	"os"
	"time"

	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/pkg/client"
	fsdiff "github.com/gadget-inc/fsdiff/pkg/diff"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Command interface {
	run(context.Context, *client.Client) error
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

func (a *newArgs) run(ctx context.Context, c *client.Client) error {
	err := c.NewProject(ctx, a.id, a.template, a.patterns)
	if err != nil {
		return fmt.Errorf("could not create new project: %w", err)
	}

	logger.Info(ctx, "created new project", zap.Int64("id", a.id))
	return nil
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

func (a *getArgs) run(ctx context.Context, c *client.Client) error {
	objects, err := c.Get(ctx, a.project, a.prefix, a.vrange)
	if err != nil {
		return fmt.Errorf("could not fetch data: %w", err)
	}

	logger.Info(ctx, "listing objects in project", zap.Int64("project", a.project), zap.Int("count", len(objects)))
	for _, object := range objects {
		logger.Info(ctx, "object", zap.String("path", object.Path), zap.String("content", string(object.Content)))
	}

	return nil
}

type rebuildArgs struct {
	project        int64
	vrange         client.VersionRange
	prefix         string
	output         string
	skipDecompress bool
}

func parseRebuildArgs(args []string) (*sharedArgs, *rebuildArgs, error) {
	set := flag.NewFlagSet("rebuild", flag.ExitOnError)

	shared := parseSharedArgs(set)
	project := set.Int64("project", -1, "Project ID (required)")
	from := set.Int64("from", -1, "From version ID (optional)")
	to := set.Int64("to", -1, "To version ID (optional)")
	prefix := set.String("prefix", "", "Search prefix")
	output := set.String("output", "", "Output directory")
	skipDecompress := set.Bool("skip_decompress", false, "Skip decompression and write archives to disk")

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
		project:        *project,
		vrange:         client.VersionRange{From: from, To: to},
		prefix:         *prefix,
		output:         *output,
		skipDecompress: *skipDecompress,
	}, nil
}

func (a *rebuildArgs) run(ctx context.Context, c *client.Client) error {
	if a.skipDecompress {
		_, err := c.FetchArchives(ctx, a.project, a.prefix, a.vrange, a.output)
		if err != nil {
			return fmt.Errorf("could not fetch archives: %w", err)
		}

		logger.Info(ctx, "wrote archives", zap.Int64("project", a.project))
		return nil
	}

	version, count, err := c.Rebuild(ctx, a.project, a.prefix, a.vrange, a.output)
	if err != nil {
		return fmt.Errorf("could not rebuild project: %w", err)
	}

	if version == -1 {
		logger.Debug(ctx, "latest version already checked out", zap.Int64("project", a.project), zap.String("output", a.output), zap.Int64p("version", a.vrange.From))
	} else {
		logger.Info(ctx, "wrote files", zap.Int64("project", a.project), zap.String("output", a.output), zap.Int64("version", version), zap.Uint32("diff_count", count))
	}

	fmt.Println(version)
	return nil
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

func (a *updateArgs) run(ctx context.Context, c *client.Client) error {
	diff, err := fsdiff.ReadDiff(a.diff)
	if err != nil {
		return fmt.Errorf("parse diff file: %w", err)
	}

	if len(diff.Updates) == 0 {
		logger.Debug(ctx, "diff file empty, nothing to update", zap.Int64("project", a.project))
		fmt.Println(-1)
	} else {
		version, count, err := c.Update(ctx, a.project, diff, a.directory)
		if err != nil {
			return fmt.Errorf("update objects: %w", err)
		}

		logger.Info(ctx, "updated objects", zap.Int64("project", a.project), zap.Int64("version", version), zap.Uint32("count", count))
		fmt.Println(version)
	}

	return nil
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

func (a *inspectArgs) run(ctx context.Context, c *client.Client) error {
	inspect, err := c.Inspect(ctx, a.project)
	if err != nil {
		return fmt.Errorf("inspect project: %w", err)
	}

	logger.Info(ctx, "inspect objects",
		zap.Int64("project", a.project),
		zap.Int64("latest_version", inspect.LatestVersion),
		zap.Int64("live_objects_count", inspect.LiveObjectsCount),
		zap.Int64("total_objects_count", inspect.TotalObjectsCount),
	)

	return nil
}

type snapshotArgs struct{}

func parseSnapshotArgs(args []string) (*sharedArgs, *snapshotArgs, error) {
	set := flag.NewFlagSet("snapshot", flag.ExitOnError)

	shared := parseSharedArgs(set)

	set.Parse(args)

	return shared, &snapshotArgs{}, nil
}

func (a *snapshotArgs) run(ctx context.Context, c *client.Client) error {
	state, err := c.Snapshot(ctx)
	if err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}

	logger.Info(ctx, "successful snapshot")
	fmt.Println(state)
	return nil
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

func (a *resetArgs) run(ctx context.Context, c *client.Client) error {
	err := c.Reset(ctx, a.state)
	if err != nil {
		return fmt.Errorf("reset: %w", err)
	}

	logger.Info(ctx, "successful reset", zap.String("state", a.state))
	return nil
}

func initLogger(level zapcore.Level, encoding string) error {
	config := zap.NewProductionConfig()
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	config.Level = zap.NewAtomicLevelAt(level)
	config.Encoding = encoding

	return logger.Init(config)
}

func main() {
	var shared *sharedArgs
	var cmd Command
	var err error

	if len(os.Args) < 2 {
		stdlog.Fatal("requires a subcommand: [new, get, rebuild, update, inspect, snapshot, reset]")
	}

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
		stdlog.Fatal("requires a subcommand: [new, get, rebuild, update, inspect, snapshot, reset]")
	}

	if err != nil {
		stdlog.Fatal(err.Error())
	}

	token := os.Getenv("DL_TOKEN")
	if token == "" {
		stdlog.Fatal("missing token: set the DL_TOKEN environment variable")
	}

	err = initLogger(*shared.level, *shared.encoding)
	if err != nil {
		stdlog.Fatal(err.Error())
	}

	// make sure this is the first deferred func, so it happens last
	exitCode := 0
	defer func() {
		if err != nil {
			exitCode = 1
		}
		os.Exit(exitCode)
	}()

	defer logger.Sync()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Second)
	defer cancel()

	c, err := client.NewClient(ctx, *shared.server, token)
	if err != nil {
		logger.Error(ctx, "could not connect to server", zap.Stringp("server", shared.server), zap.Error(err))
		return
	}
	defer c.Close()

	err = cmd.run(ctx, c)
	if err != nil {
		logger.Error(ctx, "failed to run command", zap.Error(err))
	}
}
