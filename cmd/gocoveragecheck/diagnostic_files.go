package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// prepareCoverageProfile resolves the coverage profile a lane writes and
// returns the cleanup its caller owns. A caller-named profile is kept; an
// unnamed one becomes a temp file the caller removes.
//
// A caller-named profile routinely points into a CI artifact directory that
// does not exist yet, and go test -coverprofile does not create it, so the
// directory is created here. Without it the lane fails before measuring
// anything.
func prepareCoverageProfile(profilePath string) (string, func() error, error) {
	if profilePath != "" {
		if directory := filepath.Dir(profilePath); directory != "" && directory != "." {
			if err := os.MkdirAll(directory, 0o755); err != nil {
				return "", nil, fmt.Errorf("create coverage profile directory: %w", err)
			}
		}
		return profilePath, func() error { return nil }, nil
	}
	file, err := os.CreateTemp("", "go-coverage-*.out")
	if err != nil {
		return "", nil, fmt.Errorf("create temp coverage profile: %w", err)
	}
	profilePath = file.Name()
	cleanup := func() error { return os.Remove(profilePath) }
	if err := file.Close(); err != nil {
		return "", nil, errors.Join(
			fmt.Errorf("close temp coverage profile: %w", err),
			cleanup(),
		)
	}
	return profilePath, cleanup, nil
}

// writeAtomicDiagnosticFile makes a completed JSON document visible in one
// rename. A timeout can interrupt the producer at any point, so readers must
// never observe a half-written timing or coverage artifact.
func writeAtomicDiagnosticFile(path string, data []byte) (err error) {
	directory := filepath.Dir(path)
	if directory != "" && directory != "." {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return fmt.Errorf("create diagnostic output directory: %w", err)
		}
	}

	temporary, err := os.CreateTemp(directory, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary diagnostic file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		removeErr := os.Remove(temporaryPath)
		if err == nil && removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			err = fmt.Errorf("remove temporary diagnostic file: %w", removeErr)
		}
	}()

	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary diagnostic file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary diagnostic file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary diagnostic file: %w", err)
	}
	if err := replaceDiagnosticFile(temporaryPath, path); err != nil {
		return err
	}
	return nil
}

func replaceDiagnosticFile(temporaryPath, path string) error {
	if err := os.Rename(temporaryPath, path); err == nil {
		return nil
	} else if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
		return fmt.Errorf("replace diagnostic file: rename: %w; remove existing: %v", err, removeErr)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace diagnostic file: %w", err)
	}
	return nil
}
