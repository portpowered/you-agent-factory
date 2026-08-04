package internal_test

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryinternal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal"
)

// capturedLogCall records one structured log call made through
// logging.Logger, preserving the level so tests can distinguish start
// (Info) calls from failure (Warn) calls.
type capturedLogCall struct {
	level string
	msg   string
	kv    []any
}

// captureLogger is a logging.Logger test double that records every call
// instead of writing anywhere, so tests can assert on exactly what a
// CatalogPathsService operation logs.
type captureLogger struct {
	calls *[]capturedLogCall
}

func newCaptureLogger() (captureLogger, *[]capturedLogCall) {
	calls := &[]capturedLogCall{}
	return captureLogger{calls: calls}, calls
}

func (l captureLogger) Debug(msg string, kv ...any) {
	*l.calls = append(*l.calls, capturedLogCall{level: "debug", msg: msg, kv: kv})
}
func (l captureLogger) Info(msg string, kv ...any) {
	*l.calls = append(*l.calls, capturedLogCall{level: "info", msg: msg, kv: kv})
}
func (l captureLogger) Warn(msg string, kv ...any) {
	*l.calls = append(*l.calls, capturedLogCall{level: "warn", msg: msg, kv: kv})
}
func (l captureLogger) Error(msg string, kv ...any)   {}
func (l captureLogger) Verbose(msg string, kv ...any) {}

func hasKV(kv []any, key string, value any) bool {
	for i := 0; i+1 < len(kv); i += 2 {
		if kv[i] == key && kv[i+1] == value {
			return true
		}
	}
	return false
}

func validCatalogPathsCollaborators() (
	factorydefinitions.EffectiveFactoryCatalogOperation,
	factorydefinitions.ResolveNamedFactoryOperation,
	factorydefinitions.CurrentFactoryDirectoryResolver,
) {
	listEffective := func(
		context.Context,
		factorydefinitions.ListEffectiveFactoriesRequest,
	) (factorydefinitions.ListEffectiveFactoriesResult, error) {
		return factorydefinitions.ListEffectiveFactoriesResult{}, nil
	}
	resolveNamedFactory := func(
		context.Context,
		factorydefinitions.ResolveNamedFactoryRequest,
	) (factorydefinitions.ResolveNamedFactoryResult, error) {
		return factorydefinitions.ResolveNamedFactoryResult{}, nil
	}
	resolveCurrentDir := func(string) (string, error) {
		return "", nil
	}
	return listEffective, resolveNamedFactory, resolveCurrentDir
}

func TestNewCatalogPathsServiceRejectsMissingCollaborators(t *testing.T) {
	t.Parallel()

	listEffective, resolveNamedFactory, resolveCurrentDir := validCatalogPathsCollaborators()

	if _, err := factoryinternal.NewCatalogPathsService(nil, resolveNamedFactory, resolveCurrentDir, logging.NoopLogger{}); err == nil {
		t.Fatal("NewCatalogPathsService with nil listEffective = nil error, want error")
	}
	if _, err := factoryinternal.NewCatalogPathsService(listEffective, nil, resolveCurrentDir, logging.NoopLogger{}); err == nil {
		t.Fatal("NewCatalogPathsService with nil resolveNamedFactory = nil error, want error")
	}
	if _, err := factoryinternal.NewCatalogPathsService(listEffective, resolveNamedFactory, nil, logging.NoopLogger{}); err == nil {
		t.Fatal("NewCatalogPathsService with nil resolveCurrentDir = nil error, want error")
	}
}

func TestNewCatalogPathsServiceAcceptsNilLogger(t *testing.T) {
	t.Parallel()

	listEffective, resolveNamedFactory, resolveCurrentDir := validCatalogPathsCollaborators()

	service, err := factoryinternal.NewCatalogPathsService(listEffective, resolveNamedFactory, resolveCurrentDir, nil)
	if err != nil {
		t.Fatalf("NewCatalogPathsService with nil logger: unexpected error: %v", err)
	}
	if _, err := service.ListEffectiveFactories(context.Background(), factorydefinitions.ListEffectiveFactoriesRequest{}); err != nil {
		t.Fatalf("ListEffectiveFactories with nil-logger-constructed service: unexpected error: %v", err)
	}
}

func TestNewCatalogPathsServicePerformsNoIOAtConstruction(t *testing.T) {
	t.Parallel()

	panicky := func(context.Context, factorydefinitions.ListEffectiveFactoriesRequest) (factorydefinitions.ListEffectiveFactoriesResult, error) {
		panic("listEffective invoked during inert construction")
	}
	panickyResolve := func(context.Context, factorydefinitions.ResolveNamedFactoryRequest) (factorydefinitions.ResolveNamedFactoryResult, error) {
		panic("resolveNamedFactory invoked during inert construction")
	}
	panickyCurrent := func(string) (string, error) {
		panic("resolveCurrentDir invoked during inert construction")
	}

	if _, err := factoryinternal.NewCatalogPathsService(panicky, panickyResolve, panickyCurrent, logging.NoopLogger{}); err != nil {
		t.Fatalf("NewCatalogPathsService: unexpected error: %v", err)
	}
}

func TestCatalogPathsServiceListEffectiveFactoriesForwardsToCollaborator(t *testing.T) {
	t.Parallel()

	want := factorydefinitions.ListEffectiveFactoriesResult{
		Entries: []factorydefinitions.EffectiveFactoryCatalogEntry{{Name: "alpha"}},
	}
	var gotRequest factorydefinitions.ListEffectiveFactoriesRequest
	listEffective := func(
		_ context.Context,
		request factorydefinitions.ListEffectiveFactoriesRequest,
	) (factorydefinitions.ListEffectiveFactoriesResult, error) {
		gotRequest = request
		return want, nil
	}
	_, resolveNamedFactory, resolveCurrentDir := validCatalogPathsCollaborators()

	logger, calls := newCaptureLogger()
	service, err := factoryinternal.NewCatalogPathsService(listEffective, resolveNamedFactory, resolveCurrentDir, logger)
	if err != nil {
		t.Fatalf("NewCatalogPathsService: unexpected error: %v", err)
	}

	request := factorydefinitions.ListEffectiveFactoriesRequest{ProjectRoot: "/project", GlobalRoot: "/global"}
	got, err := service.ListEffectiveFactories(context.Background(), request)
	if err != nil {
		t.Fatalf("ListEffectiveFactories: unexpected error: %v", err)
	}
	if gotRequest != request {
		t.Fatalf("ListEffectiveFactories forwarded request = %+v, want %+v", gotRequest, request)
	}
	if len(got.Entries) != 1 || got.Entries[0].Name != "alpha" {
		t.Fatalf("ListEffectiveFactories result = %+v, want the collaborator's result", got)
	}

	if len(*calls) != 2 {
		t.Fatalf("ListEffectiveFactories logged %d calls, want 2 (start, outcome): %+v", len(*calls), *calls)
	}
	if (*calls)[0].level != "info" {
		t.Fatalf("first log level = %q, want info (start)", (*calls)[0].level)
	}
	outcome := (*calls)[1]
	if outcome.level != "info" {
		t.Fatalf("second log level = %q, want info (outcome)", outcome.level)
	}
	if !hasKV(outcome.kv, "entry_count", 1) {
		t.Fatalf("outcome log missing entry_count=1: %+v", outcome.kv)
	}
}

func TestCatalogPathsServiceListEffectiveFactoriesLogsFailureClassification(t *testing.T) {
	t.Parallel()

	listEffective := func(
		context.Context,
		factorydefinitions.ListEffectiveFactoriesRequest,
	) (factorydefinitions.ListEffectiveFactoriesResult, error) {
		return factorydefinitions.ListEffectiveFactoriesResult{}, errors.New("boom")
	}
	_, resolveNamedFactory, resolveCurrentDir := validCatalogPathsCollaborators()

	logger, calls := newCaptureLogger()
	service, err := factoryinternal.NewCatalogPathsService(listEffective, resolveNamedFactory, resolveCurrentDir, logger)
	if err != nil {
		t.Fatalf("NewCatalogPathsService: unexpected error: %v", err)
	}

	if _, err := service.ListEffectiveFactories(context.Background(), factorydefinitions.ListEffectiveFactoriesRequest{}); err == nil {
		t.Fatal("ListEffectiveFactories: got nil error, want the collaborator's error")
	}

	if len(*calls) != 2 {
		t.Fatalf("ListEffectiveFactories logged %d calls, want 2 (start, failure): %+v", len(*calls), *calls)
	}
	failure := (*calls)[1]
	if failure.level != "warn" {
		t.Fatalf("failure log level = %q, want warn", failure.level)
	}
	if !hasKV(failure.kv, "reason", "operation_failed") {
		t.Fatalf("failure log missing reason=operation_failed: %+v", failure.kv)
	}
}

func TestCatalogPathsServiceResolveNamedFactoryForwardsToCollaborator(t *testing.T) {
	t.Parallel()

	want := factorydefinitions.ResolveNamedFactoryResult{
		Resolution: factorydefinitions.NamedFactoryResolution{Name: "alpha", FactoryDir: "/project/alpha"},
	}
	var gotRequest factorydefinitions.ResolveNamedFactoryRequest
	resolveNamedFactory := func(
		_ context.Context,
		request factorydefinitions.ResolveNamedFactoryRequest,
	) (factorydefinitions.ResolveNamedFactoryResult, error) {
		gotRequest = request
		return want, nil
	}
	listEffective, _, resolveCurrentDir := validCatalogPathsCollaborators()

	service, err := factoryinternal.NewCatalogPathsService(listEffective, resolveNamedFactory, resolveCurrentDir, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewCatalogPathsService: unexpected error: %v", err)
	}

	request := factorydefinitions.ResolveNamedFactoryRequest{ProjectRoot: "/project", GlobalRoot: "/global", Name: "alpha"}
	got, err := service.ResolveNamedFactory(context.Background(), request)
	if err != nil {
		t.Fatalf("ResolveNamedFactory: unexpected error: %v", err)
	}
	if gotRequest != request {
		t.Fatalf("ResolveNamedFactory forwarded request = %+v, want %+v", gotRequest, request)
	}
	if got != want {
		t.Fatalf("ResolveNamedFactory result = %+v, want %+v", got, want)
	}
}

// TestCatalogPathsServiceResolveNamedFactoryHonorsCancelledContext proves
// ResolveNamedFactory preserves the pre-cancelled-context behavior of the
// ACP adapter it replaced: an already-cancelled context short-circuits
// before the named-path collaborator runs, returning context.Canceled
// instead of performing filesystem resolution.
func TestCatalogPathsServiceResolveNamedFactoryHonorsCancelledContext(t *testing.T) {
	t.Parallel()

	called := false
	resolveNamedFactory := func(
		context.Context,
		factorydefinitions.ResolveNamedFactoryRequest,
	) (factorydefinitions.ResolveNamedFactoryResult, error) {
		called = true
		return factorydefinitions.ResolveNamedFactoryResult{}, nil
	}
	listEffective, _, resolveCurrentDir := validCatalogPathsCollaborators()

	logger, calls := newCaptureLogger()
	service, err := factoryinternal.NewCatalogPathsService(listEffective, resolveNamedFactory, resolveCurrentDir, logger)
	if err != nil {
		t.Fatalf("NewCatalogPathsService: unexpected error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := service.ResolveNamedFactory(ctx, factorydefinitions.ResolveNamedFactoryRequest{ProjectRoot: "/project", Name: "alpha"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ResolveNamedFactory error = %v, want errors.Is context.Canceled", err)
	}
	if called {
		t.Fatal("ResolveNamedFactory invoked the collaborator despite a cancelled context")
	}
	if got != (factorydefinitions.ResolveNamedFactoryResult{}) {
		t.Fatalf("ResolveNamedFactory returned a non-empty result on cancellation: %+v", got)
	}

	if len(*calls) != 2 {
		t.Fatalf("ResolveNamedFactory logged %d calls, want 2 (start, failure): %+v", len(*calls), *calls)
	}
	failure := (*calls)[1]
	if failure.level != "warn" {
		t.Fatalf("failure log level = %q, want warn", failure.level)
	}
	if !hasKV(failure.kv, "reason", "context_canceled") {
		t.Fatalf("failure log missing reason=context_canceled: %+v", failure.kv)
	}
}

func TestCatalogPathsServiceResolveNamedFactoryLogsSuccessAndFailure(t *testing.T) {
	t.Parallel()

	listEffective, _, resolveCurrentDir := validCatalogPathsCollaborators()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		resolveNamedFactory := func(
			context.Context,
			factorydefinitions.ResolveNamedFactoryRequest,
		) (factorydefinitions.ResolveNamedFactoryResult, error) {
			return factorydefinitions.ResolveNamedFactoryResult{
				Resolution: factorydefinitions.NamedFactoryResolution{
					Name:       "alpha",
					FactoryDir: "/project/alpha",
					Source:     factorydefinitions.NamedFactoryResolutionSourceProjectLocal,
				},
			}, nil
		}
		logger, calls := newCaptureLogger()
		service, err := factoryinternal.NewCatalogPathsService(listEffective, resolveNamedFactory, resolveCurrentDir, logger)
		if err != nil {
			t.Fatalf("NewCatalogPathsService: unexpected error: %v", err)
		}
		if _, err := service.ResolveNamedFactory(context.Background(), factorydefinitions.ResolveNamedFactoryRequest{Name: "alpha"}); err != nil {
			t.Fatalf("ResolveNamedFactory: unexpected error: %v", err)
		}
		if len(*calls) != 2 || (*calls)[1].level != "info" {
			t.Fatalf("ResolveNamedFactory success logs = %+v, want [info, info]", *calls)
		}
	})

	t.Run("failure", func(t *testing.T) {
		t.Parallel()
		resolveNamedFactory := func(
			context.Context,
			factorydefinitions.ResolveNamedFactoryRequest,
		) (factorydefinitions.ResolveNamedFactoryResult, error) {
			return factorydefinitions.ResolveNamedFactoryResult{}, factorydefinitions.ErrNamedFactoryNotFound
		}
		logger, calls := newCaptureLogger()
		service, err := factoryinternal.NewCatalogPathsService(listEffective, resolveNamedFactory, resolveCurrentDir, logger)
		if err != nil {
			t.Fatalf("NewCatalogPathsService: unexpected error: %v", err)
		}
		if _, err := service.ResolveNamedFactory(context.Background(), factorydefinitions.ResolveNamedFactoryRequest{Name: "missing"}); !errors.Is(err, factorydefinitions.ErrNamedFactoryNotFound) {
			t.Fatalf("ResolveNamedFactory error = %v, want errors.Is ErrNamedFactoryNotFound", err)
		}
		if len(*calls) != 2 {
			t.Fatalf("ResolveNamedFactory logged %d calls, want 2 (start, failure): %+v", len(*calls), *calls)
		}
		if !hasKV((*calls)[1].kv, "reason", "named_factory_not_found") {
			t.Fatalf("failure log missing reason=named_factory_not_found: %+v", (*calls)[1].kv)
		}
	})
}

func TestCatalogPathsServiceResolveCurrentFactoryLocationForwardsToCollaborator(t *testing.T) {
	t.Parallel()

	var gotRootDir string
	resolveCurrentDir := func(rootDir string) (string, error) {
		gotRootDir = rootDir
		return "/project/current", nil
	}
	listEffective, resolveNamedFactory, _ := validCatalogPathsCollaborators()

	service, err := factoryinternal.NewCatalogPathsService(listEffective, resolveNamedFactory, resolveCurrentDir, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewCatalogPathsService: unexpected error: %v", err)
	}

	got, err := service.ResolveCurrentFactoryLocation(context.Background(), factorydefinitions.ResolveCurrentFactoryLocationRequest{
		RootDir: "/project",
	})
	if err != nil {
		t.Fatalf("ResolveCurrentFactoryLocation: unexpected error: %v", err)
	}
	if gotRootDir != "/project" {
		t.Fatalf("ResolveCurrentFactoryLocation forwarded RootDir = %q, want %q", gotRootDir, "/project")
	}
	if got.FactoryDir != "/project/current" {
		t.Fatalf("ResolveCurrentFactoryLocation FactoryDir = %q, want %q", got.FactoryDir, "/project/current")
	}
}

func TestCatalogPathsServiceResolveCurrentFactoryLocationHonorsCancelledContext(t *testing.T) {
	t.Parallel()

	called := false
	resolveCurrentDir := func(string) (string, error) {
		called = true
		return "", nil
	}
	listEffective, resolveNamedFactory, _ := validCatalogPathsCollaborators()

	logger, calls := newCaptureLogger()
	service, err := factoryinternal.NewCatalogPathsService(listEffective, resolveNamedFactory, resolveCurrentDir, logger)
	if err != nil {
		t.Fatalf("NewCatalogPathsService: unexpected error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = service.ResolveCurrentFactoryLocation(ctx, factorydefinitions.ResolveCurrentFactoryLocationRequest{RootDir: "/project"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ResolveCurrentFactoryLocation error = %v, want errors.Is context.Canceled", err)
	}
	if called {
		t.Fatal("ResolveCurrentFactoryLocation invoked the collaborator despite a cancelled context")
	}
	if len(*calls) != 2 {
		t.Fatalf("ResolveCurrentFactoryLocation logged %d calls, want 2 (start, failure): %+v", len(*calls), *calls)
	}
	if !hasKV((*calls)[1].kv, "reason", "context_canceled") {
		t.Fatalf("failure log missing reason=context_canceled: %+v", (*calls)[1].kv)
	}
}

func TestCatalogPathsServiceResolveCurrentFactoryLocationPropagatesTypedError(t *testing.T) {
	t.Parallel()

	resolveCurrentDir := func(string) (string, error) {
		return "", factorydefinitions.ErrFactoryLayoutNotFound
	}
	listEffective, resolveNamedFactory, _ := validCatalogPathsCollaborators()

	service, err := factoryinternal.NewCatalogPathsService(listEffective, resolveNamedFactory, resolveCurrentDir, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewCatalogPathsService: unexpected error: %v", err)
	}

	_, err = service.ResolveCurrentFactoryLocation(context.Background(), factorydefinitions.ResolveCurrentFactoryLocationRequest{RootDir: "/project"})
	if !errors.Is(err, factorydefinitions.ErrFactoryLayoutNotFound) {
		t.Fatalf("ResolveCurrentFactoryLocation error = %v, want errors.Is ErrFactoryLayoutNotFound", err)
	}
}
