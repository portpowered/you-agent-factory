package service

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
)

type PublicationTemporaryFile interface {
	io.Writer
	Name() string
	Chmod(fs.FileMode) error
	Sync() error
	Close() error
}

type publicationOperations struct {
	makeDirectories     func(string, fs.FileMode) error
	createTemporaryFile func(string, string) (PublicationTemporaryFile, error)
	removePath          func(string) error
	renamePath          func(string, string) error
	readFile            func(string) ([]byte, error)
}

// Publication atomically publishes completed portable artifact bytes to a
// public destination. Failed writes never leave a destination that could be
// mistaken for a complete artifact.
type Publication struct {
	operations publicationOperations
}

// NewPublication constructs a portable-artifact publisher from injectable
// filesystem effects.
func NewPublication(
	makeDirectories func(string, fs.FileMode) error,
	createTemporaryFile func(string, string) (PublicationTemporaryFile, error),
	removePath func(string) error,
	renamePath func(string, string) error,
	readFile func(string) ([]byte, error),
) (*Publication, error) {
	if makeDirectories == nil ||
		createTemporaryFile == nil ||
		removePath == nil ||
		renamePath == nil ||
		readFile == nil {
		return nil, fmt.Errorf("portable artifact publication operations are required")
	}
	return &Publication{
		operations: publicationOperations{
			makeDirectories:     makeDirectories,
			createTemporaryFile: createTemporaryFile,
			removePath:          removePath,
			renamePath:          renamePath,
			readFile:            readFile,
		},
	}, nil
}

func (publication *Publication) Publish(ctx context.Context, destination string, payload []byte) error {
	if publication == nil {
		return fmt.Errorf("portable artifact publisher is required")
	}
	if err := publicationContextErr(ctx); err != nil {
		return err
	}
	if len(payload) == 0 {
		return fmt.Errorf("portable artifact payload is required")
	}
	dir := filepath.Dir(destination)
	if err := publication.operations.makeDirectories(dir, 0o700); err != nil {
		return fmt.Errorf("create portable artifact directory: %w", err)
	}
	if err := publicationContextErr(ctx); err != nil {
		return err
	}
	temporary, err := publication.operations.createTemporaryFile(
		dir,
		filepath.Base(destination)+".*.tmp",
	)
	if err != nil {
		return fmt.Errorf("create temporary portable artifact: %w", err)
	}
	temporaryPath := temporary.Name()
	defer publication.operations.removePath(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("secure temporary portable artifact: %w", err)
	}
	if err := publicationContextErr(ctx); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(payload); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary portable artifact: %w", err)
	}
	if err := publicationContextErr(ctx); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary portable artifact: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary portable artifact: %w", err)
	}
	if err := publicationContextErr(ctx); err != nil {
		return err
	}
	if err := publication.operations.renamePath(temporaryPath, destination); err != nil {
		return fmt.Errorf("publish portable artifact: %w", err)
	}
	return nil
}

func (publication *Publication) Read(ctx context.Context, destination string) ([]byte, error) {
	if publication == nil {
		return nil, fmt.Errorf("portable artifact publisher is required")
	}
	if err := publicationContextErr(ctx); err != nil {
		return nil, err
	}
	payload, err := publication.operations.readFile(destination)
	if err != nil {
		return nil, err
	}
	if err := publicationContextErr(ctx); err != nil {
		return nil, err
	}
	return payload, nil
}

func publicationContextErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
