package reclamation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MeasureRevisionBytes totals regular files in a managed revision without
// following symlinks or counting non-file entries.
func (s *Service) MeasureRevisionBytes(ctx context.Context, revisionPath string) (int64, error) {
	if err := s.checkContext(ctx); err != nil {
		return 0, err
	}
	if strings.TrimSpace(revisionPath) == "" {
		return 0, fmt.Errorf("managed cache revision path is empty")
	}
	return s.measureDirectoryBytes(ctx, revisionPath)
}

func (s *Service) measureDirectoryBytes(ctx context.Context, directory string) (int64, error) {
	if err := s.checkContext(ctx); err != nil {
		return 0, err
	}
	entries, err := s.files.ReadDirectory(directory)
	if err != nil {
		return 0, fmt.Errorf("read managed cache revision %q: %w", directory, err)
	}
	var total int64
	for _, entry := range entries {
		bytes, entryErr := s.measureDirectoryEntryBytes(ctx, directory, entry)
		if entryErr != nil {
			return 0, entryErr
		}
		total, entryErr = addBytes(total, bytes)
		if entryErr != nil {
			return 0, entryErr
		}
	}
	return total, nil
}

func (s *Service) measureDirectoryEntryBytes(
	ctx context.Context,
	directory string,
	entry os.DirEntry,
) (int64, error) {
	if err := s.checkContext(ctx); err != nil {
		return 0, err
	}
	if entry == nil || entry.Type()&os.ModeSymlink != 0 {
		return 0, nil
	}
	name := entry.Name()
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
		return 0, fmt.Errorf("invalid managed cache entry %q", name)
	}
	info, err := entry.Info()
	if err != nil {
		return 0, fmt.Errorf("inspect managed cache entry %q: %w", filepath.Join(directory, name), err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return 0, nil
	}
	if info.IsDir() {
		return s.measureDirectoryBytes(ctx, filepath.Join(directory, name))
	}
	if !info.Mode().IsRegular() {
		return 0, nil
	}
	if info.Size() < 0 {
		return 0, fmt.Errorf("managed cache entry %q has a negative size", filepath.Join(directory, name))
	}
	return info.Size(), nil
}
