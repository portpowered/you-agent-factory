package root

import (
	"bytes"
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"

	runcli "github.com/portpowered/infinite-you/pkg/cli/run"
	startupcli "github.com/portpowered/infinite-you/pkg/cli/startup"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil/factoryfixtures"
)

func TestRunTranslatesSuccessfulAndFailingProcessOutcomes(t *testing.T) {
	t.Parallel()
	environment := rootTestEnvironment()

	var help bytes.Buffer
	if code := Run(Input{
		Args: []string{"you", "--help"}, Env: environment, Stdout: &help,
	}, Dependencies{}); code != ExitSuccess {
		t.Fatalf("help exit code = %d, want %d", code, ExitSuccess)
	}
	if !strings.Contains(help.String(), "Usage:") {
		t.Fatalf("help output = %q, want usage", help.String())
	}

	var diagnostics bytes.Buffer
	if code := Run(Input{
		Args: []string{"you", "unknown-command"}, Env: environment, Stderr: &diagnostics,
	}, Dependencies{}); code != ExitFailure {
		t.Fatalf("invalid command exit code = %d, want %d", code, ExitFailure)
	}
	if count := strings.Count(diagnostics.String(), `unknown command "unknown-command"`); count != 1 {
		t.Fatalf("invalid command diagnostic count = %d, want 1; stderr = %q", count, diagnostics.String())
	}
}

func TestRunPreservesConstructionInitializerAndCancellationFailures(t *testing.T) {
	t.Parallel()
	environment := rootTestEnvironment()

	constructionErr := errors.New("construction failed")
	if code := Run(Input{
		Args: []string{"you", "run", "--dir", "."}, Env: environment,
	}, Dependencies{GraphBuilder: &recordingGraphBuilder{err: constructionErr}}); code != ExitFailure {
		t.Fatalf("construction failure exit code = %d, want %d", code, ExitFailure)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	initializer := &recordingInitializer{err: ctx.Err()}
	builder := &recordingGraphBuilder{graph: recordingGraph{
		lifecycle: startupcli.LifecycleFunc(func(context.Context) error { return nil }),
	}}
	if code := Run(Input{
		Args: []string{"you", "run", "--dir", "."}, Env: environment, Context: ctx,
	}, Dependencies{GraphBuilder: builder, Initializer: initializer}); code != ExitFailure {
		t.Fatalf("cancellation exit code = %d, want %d", code, ExitFailure)
	}
	if builder.calls != 1 || initializer.calls != 1 {
		t.Fatalf("builder/initializer calls = %d/%d, want 1/1", builder.calls, initializer.calls)
	}
}

func TestRunUsesSuppliedRuntimeBuilderOnce(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())

	builderCalls := 0
	runner := &processTestRunner{}
	builder := runcli.FactoryServiceBuilder(func(context.Context, *service.FactoryServiceConfig) (runcli.RuntimeRunner, error) {
		builderCalls++
		return runner, nil
	})
	code := Run(Input{
		Args: []string{"you", "run", "--dir", dir, "--quiet", "--no-record"},
		Env:  rootTestEnvironment(),
	}, Dependencies{FactoryServiceBuilder: builder})

	if code != ExitSuccess {
		t.Fatalf("exit code = %d, want %d", code, ExitSuccess)
	}
	if builderCalls != 1 || runner.calls != 1 {
		t.Fatalf("builder/runner calls = %d/%d, want 1/1", builderCalls, runner.calls)
	}
}

type processTestRunner struct{ calls int }

func (runner *processTestRunner) Run(context.Context) error {
	runner.calls++
	return nil
}

func rootTestEnvironment() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{"USERPROFILE=C:\\tmp"}
	case "plan9":
		return []string{"home=/tmp"}
	default:
		return []string{"HOME=/tmp"}
	}
}
