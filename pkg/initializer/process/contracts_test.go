package process

import (
	"context"
	"errors"
	"testing"
)

func TestEntrypointHandlersPreserveTypedInputsAndResult(t *testing.T) {
	ctx := context.WithValue(context.Background(), lifecycleContextKey{}, "value")
	wantErr := errors.New("lifecycle failed")
	wantIntent := MCPIntent{ProjectRoot: "/tmp/project"}
	called := 0
	entrypoints := Functions{
		StdioFunc: func(got context.Context, intent MCPIntent) error {
			called++
			if got != ctx {
				t.Fatal("StdioHandler did not preserve context identity")
			}
			if intent.ProjectRoot != wantIntent.ProjectRoot {
				t.Fatalf("StdioHandler intent = %+v, want %+v", intent, wantIntent)
			}
			return wantErr
		},
		RunFunc: func(context.Context, RunIntent, RunSelection) error { return nil },
	}
	if err := entrypoints.Stdio(ctx, wantIntent); !errors.Is(err, wantErr) {
		t.Fatalf("StdioHandler() error = %v, want %v", err, wantErr)
	}
	if called != 1 {
		t.Fatalf("stdio calls = %d, want 1", called)
	}
}

func TestEntrypointHandlersInitializeSystemWhenConfigured(t *testing.T) {
	ctx := context.Background()
	called := false
	entrypoints := Functions{
		InitializeSystemFunc: func(got context.Context, homeDir string) error {
			called = true
			if got != ctx || homeDir != "customer-home" {
				t.Fatalf("InitializeSystem() inputs = (%v, %q), want original context and customer-home", got, homeDir)
			}
			return nil
		},
	}
	if err := entrypoints.InitializeSystem(ctx, "customer-home"); err != nil {
		t.Fatalf("InitializeSystem() error = %v", err)
	}
	if !called {
		t.Fatal("InitializeSystemFunc was not called")
	}
	if err := (Functions{}).InitializeSystem(ctx, "customer-home"); err != nil {
		t.Fatalf("nil InitializeSystemFunc error = %v, want nil", err)
	}
}

type lifecycleContextKey struct{}
