//go:build !windows && !linux && !darwin

package pty

import (
	"context"
	"errors"
	"testing"
)

func TestUnsupportedPTYHost_FailsClosed(t *testing.T) {
	t.Parallel()
	_, err := NewHost().Allocate(context.Background())
	if !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("Allocate() error = %v, want %v", err, ErrUnsupportedPlatform)
	}
}
