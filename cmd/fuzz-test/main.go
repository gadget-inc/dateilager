package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	stdlog "log"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gadget-inc/dateilager/internal/logger"
	dlc "github.com/gadget-inc/dateilager/pkg/client"
	"github.com/gadget-inc/dateilager/pkg/version"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	charset = "aAbBcCdDeEfFgGhHiIjJkKlLmMnNoOpPqQrRsStTuUvVwWxXyYzZ0123456789"
)

type Type int

const (
	typeMissing Type = iota
	typeRegular
	typeDirectory
	typeSymlink
)

var (
	Join = filepath.Join
	// The default filesystem on MacOS is not case sensitive
	// and in that case we need to be more careful when checking if an object exists
	IsCaseSensitiveFs = runtime.GOOS != "darwin"
)

func typeStr(type_ Type) string {
	switch type_ {
	case typeMissing:
		return "missing"
	case typeRegular:
		return "file"
	case typeDirectory:
		return "directory"
	case typeSymlink:
		return "symlink"
	default:
		panic(fmt.Sprintf("unknown type: %d", type_))
	}
}

func randString(length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

func trimDir(path, dir string) string {
	return strings.TrimPrefix(strings.TrimPrefix(path, dir), "/")
}

func projectDir(base string, project int64) string {
	return filepath.Join(base, fmt.Sprint(project))
}

func walkDir(dir string) map[string]Type {
	objects := make(map[string]Type, 10)

	err := filepath.Walk(dir, func(fullPath string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		path := trimDir(fullPath, dir)
		if strings.HasPrefix(path, ".dl") || path == "" {
			return nil
		}

		switch {
		case info.Mode()&fs.ModeSymlink == fs.ModeSymlink:
			objects[path] = typeSymlink
		case info.IsDir():
			objects[path] = typeDirectory
		default:
			objects[path] = typeRegular
		}

		return nil
	})

	if err != nil {
		logger.Fatal(context.Background(), "cannot walk directory", zap.String("dir", dir), zap.Error(err))
	}

	return objects
}

func objectFilter(objects map[string]Type, type_ Type) []string {
	var files []string
	for path, objectType := range objects {
		if objectType == type_ {
			files = append(files, path)
		}
	}
	return files
}

func objectExists(objects map[string]Type, project int64, path string) bool {
	withoutProject := trimDir(path, fmt.Sprint(project))

	if IsCaseSensitiveFs {
		_, ok := objects[withoutProject]
		return ok
	} else {
		withoutProject = strings.ToLower(withoutProject)
		for path := range objects {
			if strings.ToLower(path) == withoutProject {
				return true
			}
		}
		return false
	}
}

type Operation interface {
	Apply() error
	String() string
}

type SkipOp struct{}

func (o SkipOp) Apply() error {
	return nil
}

func (o SkipOp) String() string {
	return "Skip()"
}

type AddFileOp struct {
	base    string
	dir     string
	name    string
	content []byte
}

func newAddFileOp(base string, project int64) Operation {
	dir := fmt.Sprint(project)
	name := randString(rand.Intn(20) + 1)
	objects := walkDir(Join(base, dir))

	dirs := objectFilter(objects, typeDirectory)
	if len(dirs) > 0 && rand.Intn(2) == 1 {
		dir = Join(dir, dirs[rand.Intn(len(dirs))])
	}

	if objectExists(objects, project, Join(dir, name)) {
		return SkipOp{}
	}

	return AddFileOp{
		base:    base,
		dir:     dir,
		name:    name,
		content: []byte(randString(rand.Intn(500))),
	}
}

func (o AddFileOp) Apply() error {
	return os.WriteFile(Join(o.base, o.dir, o.name), o.content, 0755)
}

func (o AddFileOp) String() string {
	return fmt.Sprintf("AddFileOp(%s, %d)", Join(o.dir, o.name), len(o.content))
}

type UpdateFileOp struct {
	base    string
	dir     string
	name    string
	content []byte
}

func newUpdateFileOp(base string, project int64) Operation {
	dir := fmt.Sprint(project)
	files := objectFilter(walkDir(Join(base, dir)), typeRegular)

	if len(files) == 0 {
		return SkipOp{}
	}

	return AddFileOp{
		base:    base,
		dir:     dir,
		name:    files[rand.Intn(len(files))],
		content: []byte(randString(rand.Intn(500))),
	}
}

func (o UpdateFileOp) Apply() error {
	return os.WriteFile(filepath.Join(o.base, o.dir, o.name), o.content, 0755)
}

func (o UpdateFileOp) String() string {
	return fmt.Sprintf("UpdateFileOp(%s, %d)", Join(o.dir, o.name), len(o.content))
}

type AddDirOp struct {
	base string
	dir  string
	name string
}

func newAddDirOp(base string, project int64) Operation {
	dir := fmt.Sprint(project)
	name := randString(rand.Intn(20) + 1)
	objects := walkDir(Join(base, dir))

	dirs := objectFilter(objects, typeDirectory)
	if rand.Intn(10) < 1 {
		dir = Join(dir, fmt.Sprintf("pack%d", rand.Intn(2)+1))
	} else if len(dirs) > 0 && rand.Intn(2) == 1 {
		dir = Join(dir, dirs[rand.Intn(len(dirs))])
	}

	if objectExists(objects, project, Join(dir, name)) {
		return SkipOp{}
	}

	return AddDirOp{
		base: base,
		dir:  dir,
		name: name,
	}
}

func (o AddDirOp) Apply() error {
	return os.MkdirAll(Join(o.base, o.dir, o.name), 0755)
}

func (o AddDirOp) String() string {
	return fmt.Sprintf("AddDirOp(%s)", Join(o.dir, o.name))
}

type RemoveFileOp struct {
	base string
	dir  string
	name string
}

func newRemoveFileOp(base string, project int64) Operation {
	dir := fmt.Sprint(project)
	files := objectFilter(walkDir(Join(base, dir)), typeRegular)

	if len(files) == 0 {
		return SkipOp{}
	}

	return RemoveFileOp{
		base: base,
		dir:  dir,
		name: files[rand.Intn(len(files))],
	}
}

func (o RemoveFileOp) Apply() error {
	return os.Remove(Join(o.base, o.dir, o.name))
}

func (o RemoveFileOp) String() string {
	return fmt.Sprintf("RemoveFileOp(%s)", Join(o.dir, o.name))
}

type AddSymlinkOp struct {
	base   string
	dir    string
	name   string
	target string
}

func newAddSymlinkOp(base string, project int64) Operation {
	dir := fmt.Sprint(project)
	name := randString(rand.Intn(20) + 1)
	objects := walkDir(Join(base, dir))

	dirs := objectFilter(objects, typeDirectory)
	files := objectFilter(objects, typeRegular)

	if len(files) == 0 {
		return SkipOp{}
	}

	if len(dirs) > 0 && rand.Intn(2) == 1 {
		dir = Join(dir, dirs[rand.Intn(len(dirs))])
	}

	if objectExists(objects, project, Join(dir, name)) {
		return SkipOp{}
	}

	return AddSymlinkOp{
		base:   base,
		dir:    dir,
		name:   name,
		target: files[rand.Intn(len(files))],
	}
}

func (o AddSymlinkOp) Apply() error {
	return os.Symlink(Join(o.base, o.dir, o.target), Join(o.base, o.dir, o.name))
}

func (o AddSymlinkOp) String() string {
	return fmt.Sprintf("AddSymlinkOp(%s, %s)", Join(o.dir, o.target), Join(o.dir, o.name))
}

type OpConstructor func(dir string, project int64) Operation

var opConstructors = []OpConstructor{newAddFileOp, newUpdateFileOp, newAddDirOp, newRemoveFileOp, newAddSymlinkOp}

func randomOperation(baseDir string, project int64) Operation {
	var operation Operation = SkipOp{}

	for {
		operation = opConstructors[rand.Intn(len(opConstructors))](baseDir, project)
		if _, isSkip := operation.(SkipOp); !isSkip {
			break
		}
	}

	return operation
}

func createDirs(projects int) (string, string, string, string, error) {
	var dirs []string

	for _, name := range []string{"base", "continue", "reset", "step"} {
		dir, err := os.MkdirTemp("", fmt.Sprintf("dl-ft-%s-", name))
		if err != nil {
			return "", "", "", "", fmt.Errorf("cannot create tmp dir: %w", err)
		}

		for projectIdx := 1; projectIdx <= projects; projectIdx++ {
			err = os.MkdirAll(filepath.Join(dir, fmt.Sprint(projectIdx)), 0755)
			if err != nil {
				return "", "", "", "", fmt.Errorf("cannot create project dir: %w", err)
			}
		}
		dirs = append(dirs, dir)
	}

	return dirs[0], dirs[1], dirs[2], dirs[3], nil
}

func runIteration(ctx context.Context, client *dlc.Client, project int64, operation Operation, baseDir, continueDir, resetDir, stepDir string) error {
	err := operation.Apply()
	if err != nil {
		return fmt.Errorf("failed to apply operation %s: %w", operation.String(), err)
	}

	version, _, err := client.Update(ctx, project, projectDir(baseDir, project))
	if err != nil {
		return fmt.Errorf("failed to update project %d: %w", project, err)
	}

	_, _, err = client.Rebuild(ctx, project, "", nil, projectDir(continueDir, project), "")
	if err != nil {
		return fmt.Errorf("failed to rebuild continue project %d: %w", project, err)
	}

	os.RemoveAll(projectDir(resetDir, project))
	err = os.MkdirAll(projectDir(resetDir, project), 0755)
	if err != nil {
		return fmt.Errorf("failed to create reset dir %s: %w", projectDir(resetDir, project), err)
	}

	_, _, err = client.Rebuild(ctx, project, "", nil, projectDir(resetDir, project), "")
	if err != nil {
		return fmt.Errorf("failed to rebuild reset project %d: %w", project, err)
	}

	os.RemoveAll(projectDir(stepDir, project))
	err = os.MkdirAll(projectDir(stepDir, project), 0755)
	if err != nil {
		return fmt.Errorf("failed to create step dir %s: %w", projectDir(stepDir, project), err)
	}

	stepVersion := int64(rand.Intn(int(version)))
	_, _, err = client.Rebuild(ctx, project, "", &stepVersion, projectDir(stepDir, project), "")
	if err != nil {
		return fmt.Errorf("failed to rebuild step project %d: %w", project, err)
	}
	_, _, err = client.Rebuild(ctx, project, "", &version, projectDir(stepDir, project), "")
	if err != nil {
		return fmt.Errorf("failed to rebuild step project %d: %w", project, err)
	}

	return nil
}

type MatchError struct {
	project         int64
	path            string
	expectedType    Type
	actualType      Type
	expectedContent string
	actualContent   string
}

func compareDirs(project int64, expected string, actual string) ([]MatchError, error) {
	var errors []MatchError

	expectedObjects := walkDir(expected)
	actualObjects := walkDir(actual)

	for path, expectedType := range expectedObjects {
		actualType, found := actualObjects[path]
		if !found {
			errors = append(errors, MatchError{
				project:      project,
				path:         path,
				expectedType: expectedType,
				actualType:   -1,
			})
			continue
		}

		if expectedType != actualType {
			errors = append(errors, MatchError{
				project:      project,
				path:         path,
				expectedType: expectedType,
				actualType:   actualType,
			})
			continue
		}

		if expectedType == typeRegular {
			expectedContent, err := os.ReadFile(filepath.Join(expected, path))
			if err != nil {
				return nil, err
			}
			actualContent, err := os.ReadFile(filepath.Join(actual, path))
			if err != nil {
				return nil, err
			}

			if string(expectedContent) != string(actualContent) {
				errors = append(errors, MatchError{
					project:         project,
					path:            path,
					expectedType:    expectedType,
					actualType:      actualType,
					expectedContent: string(expectedContent),
					actualContent:   string(actualContent),
				})
			}
		}
	}

	for path, actualType := range actualObjects {
		_, found := expectedObjects[path]
		if !found {
			errors = append(errors, MatchError{
				project:      project,
				path:         path,
				expectedType: -1,
				actualType:   actualType,
			})
		}
	}

	return errors, nil
}

func logMatchErrors(ctx context.Context, matchErrors []MatchError) {
	for _, matchErr := range matchErrors {
		props := []zapcore.Field{zap.Int("project", int(matchErr.project)), zap.String("path", matchErr.path)}

		switch {
		case matchErr.expectedType == -1:
			logger.Info(ctx, "missing file in target dir", props...)
		case matchErr.actualType == -1:
			logger.Info(ctx, "missing file in source dir", props...)
		case matchErr.expectedType != matchErr.actualType:
			props = append(props, zap.String("expected", typeStr(matchErr.expectedType)), zap.String("actual", typeStr(matchErr.actualType)))
			logger.Info(ctx, "object type mismatch", props...)
		case matchErr.expectedContent != matchErr.actualContent:
			props = append(props, zap.String("expected", matchErr.expectedContent), zap.String("actual", matchErr.actualContent))
			logger.Info(ctx, "object content mismatch", props...)
		}
	}
}

func logOpLog(ctx context.Context, opLog []Operation) {
	for idx, operation := range opLog {
		logger.Info(ctx, "operation", zap.Int("idx", idx), zap.String("op", operation.String()))
	}
}

func verifyDirs(ctx context.Context, projects int, baseDir, continueDir, resetDir, stepDir string) error {
	for projectIdx := 1; projectIdx <= projects; projectIdx++ {
		project := int64(projectIdx)

		matchErrors, err := compareDirs(project, projectDir(baseDir, project), projectDir(resetDir, project))
		if err != nil {
			return fmt.Errorf("failed to compare base & reset dirs: %w", err)
		}
		if len(matchErrors) > 0 {
			logMatchErrors(ctx, matchErrors)
			return errors.New("reset directory match error")
		}

		matchErrors, err = compareDirs(project, projectDir(baseDir, project), projectDir(continueDir, project))
		if err != nil {
			return fmt.Errorf("failed to compare base & continue dirs: %w", err)
		}
		if len(matchErrors) > 0 {
			logMatchErrors(ctx, matchErrors)
			return errors.New("continue directory match error")
		}

		matchErrors, err = compareDirs(project, projectDir(baseDir, project), projectDir(stepDir, project))
		if err != nil {
			return fmt.Errorf("failed to compare base & step dirs: %w", err)
		}
		if len(matchErrors) > 0 {
			logMatchErrors(ctx, matchErrors)
			return errors.New("step directory match error")
		}
	}
	return nil
}

func fuzzTest(ctx context.Context, client *dlc.Client, projects, iterations int) error {
	logger.Info(ctx, "starting fuzz test", zap.Int("projects", projects), zap.Int("iterations", iterations))

	for projectIdx := 1; projectIdx <= projects; projectIdx++ {
		pattern := "^pack1/.*/,^pack2/.*/"
		err := client.NewProject(ctx, int64(projectIdx), nil, &pattern)
		if err != nil {
			return err
		}
	}

	baseDir, continueDir, resetDir, stepDir, err := createDirs(projects)
	if err != nil {
		return err
	}
	defer os.RemoveAll(baseDir)
	defer os.RemoveAll(continueDir)
	defer os.RemoveAll(resetDir)
	defer os.RemoveAll(stepDir)

	var opLog []Operation

	for iterIdx := 0; iterIdx < iterations; iterIdx++ {
		project := int64(rand.Intn(projects) + 1)

		operation := randomOperation(baseDir, project)
		opLog = append(opLog, operation)

		err := runIteration(ctx, client, project, operation, baseDir, continueDir, resetDir, stepDir)
		if err != nil {
			return fmt.Errorf("failed to run iteration %d: %w", iterIdx, err)
		}

		err = verifyDirs(ctx, projects, baseDir, continueDir, resetDir, stepDir)
		if err != nil {
			logOpLog(ctx, opLog)
			return err
		}
	}

	return nil
}

func newCommand() *cobra.Command {
	var (
		projects   int
		iterations int
		server     string
	)

	cmd := &cobra.Command{
		Use:     "fuzz",
		Short:   "DateiLager fuzz testing",
		Version: version.Version,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true // silence usage when an error occurs after flags have been parsed

			ctx := cmd.Context()

			client, err := dlc.NewClient(ctx, server)
			if err != nil {
				return err
			}

			return fuzzTest(ctx, client, projects, iterations)
		},
	}

	flags := cmd.PersistentFlags()
	flags.IntVar(&projects, "projects", 5, "How many projects to create")
	flags.IntVar(&iterations, "iterations", 1000, "How many FS operations to apply")
	flags.StringVar(&server, "server", "", "Server GRPC address")

	return cmd
}

func main() {
	ctx := context.Background()
	cmd := newCommand()

	err := logger.Init(zap.Config{
		Level:       zap.NewAtomicLevelAt(zapcore.InfoLevel),
		Development: true,
		Encoding:    "console",
		EncoderConfig: zapcore.EncoderConfig{
			TimeKey:       "",
			LevelKey:      "",
			NameKey:       "",
			CallerKey:     "",
			MessageKey:    "M",
			StacktraceKey: "",
		},
		OutputPaths:      []string{"stderr"},
		ErrorOutputPaths: []string{"stderr"},
	})
	if err != nil {
		stdlog.Fatal("failed to init logger", err)
	}

	rand.Seed(time.Now().UTC().UnixNano())

	err = cmd.ExecuteContext(ctx)
	if err != nil {
		logger.Fatal(ctx, "fuzz test failed", zap.Error(err))
	}

	logger.Info(ctx, "fuzz test completed")
	_ = logger.Sync()
}
