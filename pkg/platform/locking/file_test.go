package locking

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestServiceSerializesAndReleasesOwnership(t *testing.T) {
	service := mustService(t)
	path := filepath.Join(t.TempDir(), "asset.lock")
	owner, err := service.Lock(context.Background(), path)
	if err != nil {
		t.Fatalf("Lock(owner): %v", err)
	}
	defer owner.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	started := make(chan error, 1)
	go func() {
		follower, err := service.Lock(ctx, path)
		if err == nil {
			err = follower.Close()
		}
		started <- err
	}()

	select {
	case err := <-started:
		t.Fatalf("follower acquired before owner release: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	if err := owner.Close(); err != nil {
		t.Fatalf("owner.Close(): %v", err)
	}
	if err := <-started; err != nil {
		t.Fatalf("follower Lock/Close: %v", err)
	}
}

func TestServiceWaiterCancellationReleasesDescriptor(t *testing.T) {
	service := mustService(t)
	path := filepath.Join(t.TempDir(), "asset.lock")
	owner, err := service.Lock(context.Background(), path)
	if err != nil {
		t.Fatalf("Lock(owner): %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		follower, err := service.Lock(ctx, path)
		if err == nil {
			err = follower.Close()
		}
		result <- err
	}()
	select {
	case err := <-result:
		t.Fatalf("follower completed before cancellation: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("follower error = %v, want context canceled", err)
	}
	if err := owner.Close(); err != nil {
		t.Fatalf("owner.Close(): %v", err)
	}
	follower, err := service.Lock(context.Background(), path)
	if err != nil {
		t.Fatalf("Lock(after cancellation): %v", err)
	}
	if err := follower.Close(); err != nil {
		t.Fatalf("follower.Close(): %v", err)
	}
}

func TestServiceAllowsDistinctIdentitiesToOverlap(t *testing.T) {
	service := mustService(t)
	root := t.TempDir()
	first, err := service.Lock(context.Background(), filepath.Join(root, "first.lock"))
	if err != nil {
		t.Fatalf("Lock(first): %v", err)
	}
	defer first.Close()
	second, err := service.Lock(context.Background(), filepath.Join(root, "second.lock"))
	if err != nil {
		t.Fatalf("Lock(second) while first is held: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatalf("second.Close(): %v", err)
	}
}

func mustService(t *testing.T) Service {
	t.Helper()
	service, err := New(LocalFileSystem{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return service
}
