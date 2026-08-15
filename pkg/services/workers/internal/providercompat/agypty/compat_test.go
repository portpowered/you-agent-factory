package agypty

import (
	"context"
	"errors"
	"testing"
)

func TestMockAllocatorAndSession(t *testing.T) {
	allocator := &MockAllocator{}
	session, err := allocator.Allocate(context.Background(), ProcessLaunch{}, SessionConfig{})
	if err != nil {
		t.Fatalf("Allocate() error = %v", err)
	}
	if len(allocator.Sessions) != 1 || session != allocator.Sessions[0] {
		t.Fatalf("allocated sessions = %#v, want the returned session", allocator.Sessions)
	}
	if _, err := session.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	mock := allocator.Sessions[0]
	if mock.RunCall != 1 {
		t.Fatalf("Run() calls = %d, want 1", mock.RunCall)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if !mock.Closed {
		t.Fatal("Close() did not mark the mock session closed")
	}
}

func TestMockAllocatorAndSessionErrors(t *testing.T) {
	wantErr := errors.New("pty unavailable")
	if _, err := (&MockAllocator{Err: wantErr}).Allocate(context.Background(), ProcessLaunch{}, SessionConfig{}); !errors.Is(err, wantErr) {
		t.Fatalf("Allocate() error = %v, want %v", err, wantErr)
	}

	session := &MockSession{RunErr: wantErr}
	if _, err := session.Run(context.Background()); !errors.Is(err, wantErr) {
		t.Fatalf("Run() error = %v, want %v", err, wantErr)
	}
	if session.RunCall != 1 {
		t.Fatalf("failed Run() calls = %d, want 1", session.RunCall)
	}
}
