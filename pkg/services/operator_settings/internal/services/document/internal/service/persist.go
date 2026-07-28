package service

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

func (service *Service) persistDocument(
	ctx context.Context,
	request operatorsettings.PersistDocumentRequest,
) error {
	if ctx == nil {
		return fmt.Errorf("operator document context is required")
	}
	if service.files == nil {
		return fmt.Errorf("operator document filesystem is required")
	}
	if service.createTemp == nil {
		return fmt.Errorf("operator document temporary-file creator is required")
	}

	path := strings.TrimSpace(request.Path)
	if path == "" {
		return fmt.Errorf("operator document path is required")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("persist operator document: %w", err)
	}

	data, err := service.marshalDocument(request.Document)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("persist operator document: %w", err)
	}

	dir := filepath.Dir(path)
	if err := service.files.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create operator document directory %q: %w", dir, err)
	}
	tmp, err := service.createTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create operator document temp file: %w", err)
	}
	return service.commitTemporaryFile(ctx, tmp, path, data)
}

func (service *Service) marshalDocument(document operatorsettings.Document) ([]byte, error) {
	if service.encoder == nil {
		return nil, fmt.Errorf("operator document encoder is required")
	}
	config, err := configFromDocument(document).Normalize()
	if err != nil {
		return nil, err
	}
	return service.encoder(config)
}

func (service *Service) commitTemporaryFile(
	ctx context.Context,
	tmp operatorsettings.TemporaryFile,
	path string,
	data []byte,
) error {
	tmpPath := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = service.files.Remove(tmpPath)
		}
	}()

	written, err := tmp.Write(data)
	if err == nil && written != len(data) {
		err = io.ErrShortWrite
	}
	if err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write operator document temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync operator document temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close operator document temp file: %w", err)
	}
	if err := service.files.Chmod(tmpPath, 0o600); err != nil {
		return fmt.Errorf("set operator document temp file permissions: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("persist operator document before commit: %w", err)
	}
	if err := service.files.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace operator document with temp file: %w", err)
	}
	committed = true
	return nil
}
