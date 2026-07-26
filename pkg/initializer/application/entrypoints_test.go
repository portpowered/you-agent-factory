package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/initializer"
	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
)

func TestInitializerProcessContextPreservesParentCancellation(t *testing.T) {
	t.Parallel()

	parent, cancel := context.WithCancel(context.Background())
	ctx, stop := (&Initializer{}).ProcessContext(parent)
	if ctx == nil || stop == nil {
		t.Fatal("ProcessContext() returned an invalid lifecycle")
	}
	defer stop()
	cancel()
	select {
	case <-ctx.Done():
		if !errors.Is(ctx.Err(), context.Canceled) {
			t.Fatalf("context error = %v, want context.Canceled", ctx.Err())
		}
	case <-time.After(time.Second):
		t.Fatal("process context did not preserve parent cancellation")
	}
}

func TestInitializerRunAppliesPolicyAndRunsSelectedApplication(t *testing.T) {
	selection := &runSelectionStub{}
	entrypoint := &Initializer{}
	err := entrypoint.Run(context.Background(), startupcli.RunIntent{
		WorkerSidecarsEnabled: true,
	}, selection)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !selection.intent.WorkerSidecarsEnabled || !selection.ran {
		t.Fatalf("selection = %#v", selection)
	}
}

func TestInitializerStdioOnlyOpensAndRunsLifecycleReadyApplication(t *testing.T) {
	wantIntent := startupcli.MCPIntent{RuntimeBacked: true, ProjectRoot: "project", HomeDir: "stdio-home"}
	opener := &stdioApplicationOpenerStub{}
	system := func(context.Context, string) error { return nil }
	entrypoint, err := NewInitializer(opener, system)
	if err != nil {
		t.Fatalf("NewInitializer: %v", err)
	}
	if err := entrypoint.Stdio(context.Background(), wantIntent); err != nil {
		t.Fatalf("Stdio: %v", err)
	}
	if opener.intent != wantIntent || !opener.ran {
		t.Fatalf("stdio opener = intent:%#v ran:%v", opener.intent, opener.ran)
	}
}

func TestInitializerOwnsSystemInitializationOperation(t *testing.T) {
	var initializedHome string
	entrypoint := &Initializer{systemInitialization: func(_ context.Context, homeDir string) error {
		initializedHome = homeDir
		return nil
	}}
	if err := entrypoint.InitializeSystem(t.Context(), "customer-home"); err != nil {
		t.Fatalf("InitializeSystem() error = %v", err)
	}
	if initializedHome != "customer-home" {
		t.Fatalf("system initialization home = %q, want customer-home", initializedHome)
	}
}

type runSelectionStub struct {
	intent startupcli.RunIntent
	ran    bool
}

func (s *runSelectionStub) Open(_ context.Context, intent startupcli.RunIntent) (initializer.RunApplication, error) {
	s.intent = intent
	return runApplicationFunc(func(context.Context) error { s.ran = true; return nil }), nil
}

type runApplicationFunc func(context.Context) error

func (run runApplicationFunc) Run(ctx context.Context) error { return run(ctx) }

type stdioApplicationOpenerStub struct {
	intent startupcli.MCPIntent
	ran    bool
}

func (stub *stdioApplicationOpenerStub) OpenStdio(_ context.Context, intent startupcli.MCPIntent) (initializer.RunApplication, error) {
	stub.intent = intent
	return runApplicationFunc(func(context.Context) error {
		stub.ran = true
		return nil
	}), nil
}
