package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	stdlog "log"
	"os"
	"strings"
	"time"

	"github.com/gadget-inc/dateilager/internal/key"
	"github.com/gadget-inc/dateilager/internal/logger"
	"github.com/gadget-inc/dateilager/internal/telemetry"
	"github.com/gadget-inc/dateilager/pkg/client"
	"github.com/gadget-inc/dateilager/pkg/version"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Command interface {
	run(context.Context, *client.Client) error
}

type sharedArgs struct {
	server      *string
	level       *zapcore.Level
	encoding    *string
	tracing     *bool
	otelContext *string
}

func parseSharedArgs(set *flag.FlagSet) *sharedArgs {
	level := zapcore.DebugLevel
	set.Var(&level, "log", "Log level")

	server := set.String("server", "", "Server GRPC address")
	encoding := set.String("encoding", "console", "Log encoding (console | json)")
	tracing := set.Bool("tracing", false, "Whether tracing is enabled")
	otelContext := set.String("otel-context", "", "Open Telemetry context")

	return &sharedArgs{
		server:      server,
		level:       &level,
		encoding:    encoding,
		tracing:     tracing,
		otelContext: otelContext,
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

	_ = set.Parse(args)

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

	logger.Info(ctx, "created new project", key.Project.Field(a.id))
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

	_ = set.Parse(args)

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
	objects, err := c.Get(ctx, a.project, a.prefix, nil, a.vrange)
	if err != nil {
		return fmt.Errorf("could not fetch data: %w", err)
	}

	logger.Info(ctx, "listing objects in project", key.Project.Field(a.project), key.ObjectsCount.Field(len(objects)))
	for _, object := range objects {
		logger.Info(ctx, "object", key.ObjectPath.Field(object.Path), key.ObjectContent.Field(string(object.Content)[:10]))
	}

	return nil
}

type rebuildArgs struct {
	project int64
	to      *int64
	prefix  string
	dir     string
}

func parseRebuildArgs(args []string) (*sharedArgs, *rebuildArgs, error) {
	set := flag.NewFlagSet("rebuild", flag.ExitOnError)

	shared := parseSharedArgs(set)
	project := set.Int64("project", -1, "Project ID (required)")
	to := set.Int64("to", -1, "To version ID (optional)")
	prefix := set.String("prefix", "", "Search prefix")
	dir := set.String("dir", "", "Output directory")

	_ = set.Parse(args)

	if *project == -1 {
		return nil, nil, errors.New("required arg: -project")
	}

	if *to == -1 {
		to = nil
	}

	return shared, &rebuildArgs{
		project: *project,
		to:      to,
		prefix:  *prefix,
		dir:     *dir,
	}, nil
}

func (a *rebuildArgs) run(ctx context.Context, c *client.Client) error {
	version, count, err := c.Rebuild(ctx, a.project, a.prefix, a.to, a.dir)
	if err != nil {
		return fmt.Errorf("could not rebuild project: %w", err)
	}

	if version == -1 {
		logger.Debug(ctx, "latest version already checked out",
			key.Project.Field(a.project),
			key.Directory.Field(a.dir),
			key.ToVersion.Field(a.to),
		)
	} else {
		logger.Info(ctx, "wrote files",
			key.Project.Field(a.project),
			key.Directory.Field(a.dir),
			key.Version.Field(version),
			key.DiffCount.Field(count),
		)
	}

	fmt.Println(version)
	return nil
}

type updateArgs struct {
	project int64
	dir     string
}

func parseUpdateArgs(args []string) (*sharedArgs, *updateArgs, error) {
	set := flag.NewFlagSet("update", flag.ExitOnError)

	shared := parseSharedArgs(set)
	project := set.Int64("project", -1, "Project ID (required)")
	dir := set.String("dir", "", "Directory containing updated files")

	_ = set.Parse(args)

	if *project == -1 {
		return nil, nil, errors.New("required arg: -project")
	}

	return shared, &updateArgs{
		project: *project,
		dir:     *dir,
	}, nil
}

func (a *updateArgs) run(ctx context.Context, c *client.Client) error {
	version, count, err := c.Update(ctx, a.project, a.dir)
	if err != nil {
		return fmt.Errorf("update objects: %w", err)
	}

	if count == 0 {
		logger.Debug(ctx, "diff file is empty, nothing to update", key.Project.Field(a.project), key.Version.Field(version))
	} else {
		logger.Info(ctx, "updated objects", key.Project.Field(a.project), key.Version.Field(version), key.DiffCount.Field(count))
	}
	fmt.Println(version)
	return nil
}

type inspectArgs struct {
	project int64
}

func parseInspectArgs(args []string) (*sharedArgs, *inspectArgs, error) {
	set := flag.NewFlagSet("inspect", flag.ExitOnError)

	shared := parseSharedArgs(set)
	project := set.Int64("project", -1, "Project ID (required)")

	_ = set.Parse(args)

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
		key.Project.Field(a.project),
		key.LatestVersion.Field(inspect.LatestVersion),
		key.LiveObjectsCount.Field(inspect.LiveObjectsCount),
		key.TotalObjectsCount.Field(inspect.TotalObjectsCount),
	)

	return nil
}

type snapshotArgs struct{}

func parseSnapshotArgs(args []string) (*sharedArgs, *snapshotArgs, error) {
	set := flag.NewFlagSet("snapshot", flag.ExitOnError)

	shared := parseSharedArgs(set)

	_ = set.Parse(args)

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

	_ = set.Parse(args)

	return shared, &resetArgs{
		state: *state,
	}, nil
}

func (a *resetArgs) run(ctx context.Context, c *client.Client) error {
	err := c.Reset(ctx, a.state)
	if err != nil {
		return fmt.Errorf("reset: %w", err)
	}

	logger.Info(ctx, "successful reset", key.State.Field(a.state))
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
		stdlog.Fatal("requires a subcommand: [new, get, rebuild, update, inspect, snapshot, reset, version]")
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
	case "version":
		fmt.Println(version.Version)
		return
	default:
		stdlog.Fatal("requires a subcommand: [new, get, rebuild, update, inspect, snapshot, reset, version]")
	}

	if err != nil {
		stdlog.Fatal(err.Error())
	}

	token := os.Getenv("DL_TOKEN")
	if token == "" {
		tokenFile := os.Getenv("DL_TOKEN_FILE")
		if tokenFile == "" {
			stdlog.Fatal("missing token: set the DL_TOKEN or DL_TOKEN_FILE environment variable")
		}

		bytes, err := os.ReadFile(tokenFile)
		if err != nil {
			stdlog.Fatalf("failed to read contents of DL_TOKEN_FILE: %v", err)
		}

		token = string(bytes)
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

	defer logger.Sync() // nolint:errcheck

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Second)
	defer cancel()

	if *shared.tracing {
		shutdown, err := telemetry.Init(ctx, telemetry.Client)
		if err != nil {
			logger.Error(ctx, "could not initialize telemetry", zap.Error(err))
			return
		}
		defer shutdown()
	}

	if *shared.otelContext != "" {
		var mapCarrier propagation.MapCarrier
		err = json.NewDecoder(strings.NewReader(*shared.otelContext)).Decode(&mapCarrier)
		if err != nil {
			logger.Error(ctx, "failed to decode otel-context", zap.Error(err))
			return
		}
		ctx = otel.GetTextMapPropagator().Extract(ctx, mapCarrier)
	}

	ctx, span := telemetry.Start(ctx, "cmd.main")
	defer span.End()

	c, err := client.NewClient(ctx, *shared.server, token)
	if err != nil {
		logger.Error(ctx, "could not connect to server", key.Server.Field(*shared.server), zap.Error(err))
		return
	}
	defer c.Close()

	err = cmd.run(ctx, c)
	if err != nil {
		logger.Error(ctx, "failed to run command", zap.Error(err))
	}
}
