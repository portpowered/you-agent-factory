package reclamation

import (
	"context"
	"errors"
	"fmt"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	"os"
	"path/filepath"
)

func (s *Service) RemoveManagedTree(ctx context.Context, directory string) error {
	if err := s.checkContext(ctx); err != nil {
		return err
	}
	entries, err := s.readDirectory(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: read managed cache revision: %w", models.ErrModelCacheRemovalFailed, err)
	}
	for _, entry := range entries {
		if err := s.checkContext(ctx); err != nil {
			return err
		}
		if entry != nil {
			if err := s.removeManagedEntry(ctx, directory, entry); err != nil {
				return err
			}
		}
	}
	if err := s.checkContext(ctx); err != nil {
		return err
	}
	return s.removeManagedPath(directory)
}

func (s *Service) removeManagedEntry(
	ctx context.Context,
	directory string,
	entry os.DirEntry,
) error {
	name := entry.Name()
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
		return fmt.Errorf("%w: invalid managed cache entry %q", models.ErrModelCacheUnsafe, name)
	}
	child := filepath.Join(directory, name)
	if entry.Type()&os.ModeSymlink != 0 {
		return s.removeManagedPath(child)
	}
	info, err := entry.Info()
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: inspect managed cache entry: %w", models.ErrModelCacheRemovalFailed, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return s.removeManagedPath(child)
	}
	if info.IsDir() {
		return s.RemoveManagedTree(ctx, child)
	}
	return s.removeManagedPath(child)
}

func (s *Service) removeManagedPath(path string) error {
	if err := s.removePath(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: remove managed cache path: %w", models.ErrModelCacheRemovalFailed, err)
	}
	return nil
}

func (s *Service) VerifyManagedPathRemoved(ctx context.Context, parent, child string) error {
	if err := s.checkContext(ctx); err != nil {
		return err
	}
	entries, err := s.readDirectory(parent)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: verify managed cache removal: %w", models.ErrModelCacheRemovalFailed, err)
	}
	for _, entry := range entries {
		if entry != nil && entry.Name() == child {
			return fmt.Errorf("%w: managed cache revision still exists", models.ErrModelCacheRemovalFailed)
		}
	}
	return nil
}
