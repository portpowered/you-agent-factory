package locking

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTargetClaimRefusesReplacementDuringAcquisition(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"target open", "marker open"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			target, replacement := filepath.Join(dir, "history.json"), filepath.Join(dir, "replacement.json")
			for path, content := range map[string]string{target: "original history", replacement: "replacement history"} {
				if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			marker := target + ".lock"
			trigger := target
			if phase == "marker open" {
				trigger = marker
			}
			files := &replacingTargetFiles{trigger: trigger, target: target, replacement: replacement}
			service, err := New(files)
			if err != nil {
				t.Fatal(err)
			}
			lease, err := service.TryLockTarget(t.Context(), target, marker)
			if lease != nil {
				_ = lease.Close()
			}
			if lease != nil || err == nil {
				t.Fatalf("changed target acquired: %v, %v", lease, err)
			}
			got, err := os.ReadFile(target)
			if err != nil || string(got) != "replacement history" {
				t.Fatalf("refusal changed replacement bytes: %q, %v", got, err)
			}
			// Refusal must release any marker acquired before the change was
			// observed, allowing an independent owner to inspect the new file.
			lease, err = mustService(t).TryLockTarget(t.Context(), target, marker)
			if err != nil {
				t.Fatalf("refused opening retained ownership: %v", err)
			}
			if err := lease.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

type replacingTargetFiles struct {
	LocalFileSystem
	trigger, target, replacement string
}

func (files *replacingTargetFiles) OpenFile(path string, flags int, mode fs.FileMode) (File, error) {
	if path == files.trigger {
		files.trigger = ""
		if err := os.Rename(files.replacement, files.target); err != nil {
			return nil, err
		}
	}
	return files.LocalFileSystem.OpenFile(path, flags, mode)
}

func TestTargetClaimRefusesHardLinksWithoutChangingBytes(t *testing.T) {
	t.Parallel()
	service := mustService(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "history.json")
	alias := filepath.Join(dir, "alias.json")
	want := []byte("retained history")
	if err := os.WriteFile(target, want, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(target, alias); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{target, alias} {
		lease, err := service.TryLockTarget(t.Context(), path, path+".lock")
		if lease != nil || err == nil {
			t.Fatalf("hard-linked target acquired: %v, %v", lease, err)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != string(want) {
			t.Fatalf("target bytes changed: %q, %v", got, err)
		}
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	lease, err := service.TryLockTarget(t.Context(), target, target+".lock")
	if err != nil {
		t.Fatalf("single-link target refused: %v", err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
}

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

func TestServiceTryLockRefusesBusyOwnerAndReleases(t *testing.T) {
	t.Parallel()
	service := mustService(t)
	path := filepath.Join(t.TempDir(), "recording.lock")
	owner, err := service.TryLock(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close() })
	other := mustService(t)
	lease, err := other.TryLock(t.Context(), path)
	if lease != nil || !errors.Is(err, ErrBusy) {
		t.Fatalf("occupied marker = %v, %v; want no lease and ErrBusy", lease, err)
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	lease, err = other.TryLock(t.Context(), path)
	if err != nil {
		t.Fatalf("released marker: %v", err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	if err := lease.Close(); err != nil {
		t.Fatalf("repeat close: %v", err)
	}
}
