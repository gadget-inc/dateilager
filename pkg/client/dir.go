package client

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	fsdiff "github.com/gadget-inc/fsdiff/pkg/diff"
	fsdiff_pb "github.com/gadget-inc/fsdiff/pkg/pb"
)

const metadataDir = ".dl"

var (
	versionFile   = filepath.Join(metadataDir, "version")
	summaryFile   = filepath.Join(metadataDir, "sum.s2")
	diffFile      = filepath.Join(metadataDir, "diff.s2")
	fsdiffIgnores = []string{metadataDir, versionFile, summaryFile, diffFile}
)

func ensureMetadataDir(dir string) error {
	path := filepath.Join(dir, metadataDir)
	err := os.MkdirAll(path, 0775)
	if err != nil {
		return fmt.Errorf("cannot create metadata dir %v: %w", path, err)
	}
	return nil
}

func ReadVersionFile(dir string) (int64, error) {
	path := filepath.Join(dir, versionFile)
	bytes, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return -1, fmt.Errorf("cannot read version file %v: %w", path, err)
	}

	version, err := strconv.ParseInt(strings.TrimSpace(string(bytes)), 10, 64)
	if err != nil {
		return -1, fmt.Errorf("cannot convert version to int64 %v: %w", string(bytes), err)
	}
	return version, nil
}

func WriteVersionFile(dir string, version int64) error {
	err := ensureMetadataDir(dir)
	if err != nil {
		return err
	}

	path := filepath.Join(dir, versionFile)
	err = os.WriteFile(path, []byte(strconv.FormatInt(version, 10)), 0755)
	if err != nil {
		return fmt.Errorf("cannot write version file to %v: %w", path, err)
	}
	return nil
}

func DiffAndSummarize(dir string) (*fsdiff_pb.Diff, error) {
	err := ensureMetadataDir(dir)
	if err != nil {
		return nil, err
	}

	path := filepath.Join(dir, summaryFile)
	summary, err := fsdiff.ReadSummary(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("cannot read summary file %v: %w", path, err)
	}

	// FIXME: Handle this in fsdiff
	if summary == nil {
		summary = &fsdiff_pb.Summary{}
	}

	diff, summary, err := fsdiff.Diff(dir, fsdiffIgnores, summary)
	if err != nil {
		return nil, fmt.Errorf("fsdiff error: %w", err)
	}

	err = fsdiff.WriteSummary(path, summary)
	if err != nil {
		return nil, fmt.Errorf("cannot write summary file to %v: %w", path, err)
	}

	return diff, nil
}
