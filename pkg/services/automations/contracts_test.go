package automations

import (
	"context"
	"errors"
	"testing"
)

func TestRootRejectsTypedNilOperations(t *testing.T) {
	t.Parallel()

	var typedNil *typedNilService
	root := Root{Operations: typedNil}
	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "Reconcile",
			call: func() error {
				_, err := root.Reconcile(context.Background(), ReconcileRequest{})
				return err
			},
		},
		{
			name: "StartSource",
			call: func() error {
				_, err := root.StartSource(context.Background(), StartSourceRequest{})
				return err
			},
		},
		{
			name: "StopSource",
			call: func() error {
				_, err := root.StopSource(context.Background(), StopSourceRequest{})
				return err
			},
		},
		{
			name: "WaitSource",
			call: func() error {
				_, err := root.WaitSource(context.Background(), WaitSourceRequest{})
				return err
			},
		},
		{
			name: "SourceStatus",
			call: func() error {
				_, err := root.SourceStatus(context.Background(), SourceStatusRequest{})
				return err
			},
		},
		{
			name: "GetStatus",
			call: func() error {
				_, err := root.GetStatus(context.Background(), GetStatusRequest{})
				return err
			},
		},
		{
			name: "GetCursor",
			call: func() error {
				_, err := root.GetCursor(context.Background(), GetCursorRequest{})
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.call()
			if err == nil {
				t.Fatal("root operation error = nil, want not-ready error")
			}
			var rootErr *Error
			if !errors.As(err, &rootErr) {
				t.Fatalf("root operation error = %v, want *Error", err)
			}
			if rootErr.Code != ErrorCodeNotReady {
				t.Fatalf("root operation error code = %q, want %q", rootErr.Code, ErrorCodeNotReady)
			}
			if !errors.Is(err, ErrNotReady) {
				t.Fatalf("root operation error = %v, want ErrNotReady", err)
			}
		})
	}
}

type typedNilService struct {
	Service
}
