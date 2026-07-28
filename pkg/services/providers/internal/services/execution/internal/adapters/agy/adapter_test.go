package agy_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	agy "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers/agypty"
)

func TestAgyNewRegistrationBindsCanonicalIdentity(t *testing.T) {
	t.Parallel()

	registration := agy.NewRegistration(nil)
	if registration.Provider != providers.IDAgy {
		t.Fatalf("Provider = %q, want %q", registration.Provider, providers.IDAgy)
	}
	if registration.Attempt == nil {
		t.Fatal("Attempt = nil, want unavailable attempt")
	}
}

func TestAgyRootFailsClosedWhenEffectAbsent(t *testing.T) {
	t.Parallel()

	root := newAgyRoot(t, nil)
	result, err := root.Execute(
		t.Context(),
		providers.ExecuteRequest{
			Provider:  providers.IDAgy,
			AttemptID: "attempt-agy-unavailable",
		},
	)
	assertAgyDependencyFailure(t, result, err)
}

func TestAgyBuiltInRegistrationFailsClosedWithoutEffect(t *testing.T) {
	t.Parallel()

	catalog, err := catalogwire.NewService()
	if err != nil {
		t.Fatalf("catalogwire.NewService() = %v", err)
	}
	executionService, err := executionwire.NewBuiltInService(catalog)
	if err != nil {
		t.Fatalf("NewBuiltInService() = %v", err)
	}
	root, err := providerservice.New(catalog, executionService)
	if err != nil {
		t.Fatalf("providerservice.New() = %v", err)
	}

	result, err := root.Execute(
		t.Context(),
		providers.ExecuteRequest{
			Provider:  providers.IDAgy,
			AttemptID: "attempt-agy-built-in-unavailable",
		},
	)
	assertAgyDependencyFailure(t, result, err)
}

func TestAgyRootPTYExecutionEndToEnd(t *testing.T) {
	t.Parallel()

	factoryRoot := t.TempDir()
	mock := &stubAllocator{result: agypty.SessionResult{ExitCode: 0, CleanedText: "Hello from Agy"}}
	effect := agy.NewPTYEffect(agy.PTYEffectOptions{
		FactoryRoot:            factoryRoot,
		Allocator:              mock,
		Executable:             "agy",
		ExecutableDependencies: executableDependencies(nil),
	})
	root := newAgyRoot(t, effect)

	result, err := root.Execute(t.Context(), providers.ExecuteRequest{
		Provider:         providers.IDAgy,
		AttemptID:        "dispatch-agy-e2e",
		WorkingDirectory: ".",
		UserMessage:      privatePrompt,
		ResumeSession: &providers.SessionRef{
			Provider: providers.IDAgy,
			Kind:     providers.SessionIDKind,
			ID:       "session-e2e",
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Content != "Hello from Agy" {
		t.Fatalf("Content = %q, want cleaned final text", result.Content)
	}
	if result.SessionRef == nil || result.SessionRef.ID != "session-e2e" {
		t.Fatalf("SessionRef = %#v, want resumed session", result.SessionRef)
	}
	if result.Diagnostics == nil || result.Diagnostics.DurationMillis < 0 {
		t.Fatalf("Diagnostics = %#v", result.Diagnostics)
	}
}

func TestAgyRootRejectsUnusableFinalOutput(t *testing.T) {
	t.Parallel()

	factoryRoot := t.TempDir()
	mock := &stubAllocator{result: agypty.SessionResult{ExitCode: 0, CleanedText: ""}}
	effect := agy.NewPTYEffect(agy.PTYEffectOptions{
		FactoryRoot:            factoryRoot,
		Allocator:              mock,
		Executable:             "agy",
		ExecutableDependencies: executableDependencies(nil),
	})
	root := newAgyRoot(t, effect)

	result, err := root.Execute(t.Context(), providers.ExecuteRequest{
		Provider:    providers.IDAgy,
		AttemptID:   "dispatch-agy-empty",
		UserMessage: "hello",
	})
	if err == nil {
		t.Fatal("Execute() error = nil, want final parse failure")
	}
	if result.Content != "" {
		t.Fatalf("result = %#v, want empty content on failure", result)
	}
}

func TestAgyRootPreservesRequestAndFinalStdout(t *testing.T) {
	t.Parallel()

	request := providers.ExecuteRequest{
		Provider:         providers.IDAgy,
		AttemptID:        "attempt-agy-success",
		Model:            "agy-model",
		UserMessage:      "perform the accepted work",
		WorkingDirectory: "C:/factory",
	}
	const content = "agy final answer"
	var received providers.ExecuteRequest
	effect := agy.EffectFunc(func(
		_ context.Context,
		got providers.ExecuteRequest,
		observe func([]byte) error,
	) (agy.EffectResult, error) {
		received = got.Clone()
		if err := observe([]byte(content)); err != nil {
			return agy.EffectResult{}, err
		}
		return agy.EffectResult{DurationMillis: 23}, nil
	})
	root := newAgyRoot(t, effect)

	result, err := root.Execute(t.Context(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !reflect.DeepEqual(received, request) {
		t.Fatalf("native request = %#v, want %#v", received, request)
	}
	if result.Content != content {
		t.Fatalf("Content = %q, want %q", result.Content, content)
	}
	if result.Diagnostics == nil || result.Diagnostics.DurationMillis != 23 {
		t.Fatalf("Diagnostics = %#v", result.Diagnostics)
	}
}

func assertAgyDependencyFailure(
	t *testing.T,
	result providers.ExecuteResult,
	err error,
) {
	t.Helper()
	if !reflect.DeepEqual(result, providers.ExecuteResult{}) {
		t.Fatalf("failed Execute() result = %#v, want zero result", result)
	}
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) ||
		failure.Kind != providers.ExecuteFailureKindDependency {
		t.Fatalf("Execute() error = %#v, want dependency failure", err)
	}
	if !strings.Contains(failure.Message, "Agy") {
		t.Fatalf("failure message = %q, want Agy unavailable message", failure.Message)
	}
}

func newAgyRoot(t *testing.T, effect agy.Effect) providers.Service {
	t.Helper()
	catalog, err := catalogwire.NewService()
	if err != nil {
		t.Fatal(err)
	}
	executionService, err := executionwire.NewService(
		catalog,
		agy.NewRegistration(effect),
	)
	if err != nil {
		t.Fatal(err)
	}
	root, err := providerservice.New(catalog, executionService)
	if err != nil {
		t.Fatal(err)
	}
	return root
}
