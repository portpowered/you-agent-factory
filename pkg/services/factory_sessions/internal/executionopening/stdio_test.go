package executionopening

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
)

func TestStdioOpeningServiceOwnsFixtureSelection(t *testing.T) {
	owned := &ownedExecutionStub{}
	opening := &stdioExecutionOpeningStub{execution: owned}
	application := stdioApplicationStub{}
	operation, err := NewStdioOpeningService(
		opening,
		func(_ context.Context, got durableexecution.Service, _ io.Reader, _ io.Writer) (roles.StdioApplication, error) {
			if got != owned {
				t.Fatal("fixture builder did not receive opened execution")
			}
			return application, nil
		},
		runtimeStdioBuilderNotCalled(t),
	)
	if err != nil {
		t.Fatalf("NewStdioOpeningService: %v", err)
	}

	input, output := &bytes.Buffer{}, &bytes.Buffer{}
	got, err := operation.OpenStdio(context.Background(), factorysessions.StdioOpeningRequest{
		FixtureCatalogPath: "fixtures.json", Input: input, Output: output,
	})
	if err != nil {
		t.Fatalf("OpenStdio: %v", err)
	}
	if got != application || opening.fixtureCatalog != "fixtures.json" || opening.runtimeOpened {
		t.Fatalf("fixture opening = application:%v fixture:%q runtime:%v", got, opening.fixtureCatalog, opening.runtimeOpened)
	}
}

func TestStdioOpeningServiceOwnsRuntimeBackedSelection(t *testing.T) {
	opened := roles.OpenedExecutionRuntime{Execution: &ownedExecutionStub{}}
	opening := &stdioExecutionOpeningStub{resolvedRoot: "resolved", opened: opened}
	application := stdioApplicationStub{}
	operation, err := NewStdioOpeningService(
		opening,
		fixtureStdioBuilderNotCalled(t),
		func(_ context.Context, got roles.OpenedExecutionRuntime, _ io.Reader, _ io.Writer) (roles.StdioApplication, error) {
			if got.Execution != opened.Execution {
				t.Fatal("runtime builder did not receive opened execution")
			}
			return application, nil
		},
	)
	if err != nil {
		t.Fatalf("NewStdioOpeningService: %v", err)
	}

	got, err := operation.OpenStdio(context.Background(), factorysessions.StdioOpeningRequest{
		RuntimeBacked: true, ProjectRoot: "project", SystemConfigHome: "home",
		Input: &bytes.Buffer{}, Output: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("OpenStdio: %v", err)
	}
	if got != application || opening.resolvedInput != "project" || opening.opening.ProjectRoot != "resolved" || opening.opening.SystemConfigHome != "home" {
		t.Fatalf("runtime opening = application:%v input:%q request:%#v", got, opening.resolvedInput, opening.opening)
	}
}

func TestStdioOpeningServiceClosesFixtureWhenApplicationBuildFails(t *testing.T) {
	buildErr, closeErr := errors.New("build failed"), errors.New("close failed")
	owned := &ownedExecutionStub{closeErr: closeErr}
	operation, err := NewStdioOpeningService(
		&stdioExecutionOpeningStub{execution: owned},
		func(context.Context, durableexecution.Service, io.Reader, io.Writer) (roles.StdioApplication, error) {
			return nil, buildErr
		},
		runtimeStdioBuilderNotCalled(t),
	)
	if err != nil {
		t.Fatalf("NewStdioOpeningService: %v", err)
	}
	_, err = operation.OpenStdio(context.Background(), factorysessions.StdioOpeningRequest{
		FixtureCatalogPath: "fixtures.json", Input: &bytes.Buffer{}, Output: &bytes.Buffer{},
	})
	if !errors.Is(err, buildErr) || !errors.Is(err, closeErr) || !owned.closed {
		t.Fatalf("OpenStdio error = %v, closed = %v", err, owned.closed)
	}
}

func TestStdioOpeningServiceLeavesRuntimeFailureCleanupWithBuilder(t *testing.T) {
	buildErr := errors.New("build failed")
	closeCount := 0
	opening := &stdioExecutionOpeningStub{
		resolvedRoot: "resolved",
		opened: roles.OpenedExecutionRuntime{
			Execution: &ownedExecutionStub{},
			Resources: roles.RuntimeResources{Close: func() error {
				closeCount++
				return nil
			}},
		},
	}
	operation, err := NewStdioOpeningService(
		opening,
		fixtureStdioBuilderNotCalled(t),
		func(_ context.Context, opened roles.OpenedExecutionRuntime, _ io.Reader, _ io.Writer) (roles.StdioApplication, error) {
			if closeErr := opened.Resources.Close(); closeErr != nil {
				t.Fatalf("close runtime resources: %v", closeErr)
			}
			return nil, buildErr
		},
	)
	if err != nil {
		t.Fatalf("NewStdioOpeningService: %v", err)
	}
	_, err = operation.OpenStdio(context.Background(), factorysessions.StdioOpeningRequest{
		RuntimeBacked: true, ProjectRoot: "project",
		Input: &bytes.Buffer{}, Output: &bytes.Buffer{},
	})
	if !errors.Is(err, buildErr) || closeCount != 1 {
		t.Fatalf("OpenStdio error = %v, close count = %d, want build error and one close", err, closeCount)
	}
}

func TestStdioOpeningServiceRejectsNilApplications(t *testing.T) {
	owned := &ownedExecutionStub{}
	operation, err := NewStdioOpeningService(
		&stdioExecutionOpeningStub{execution: owned},
		func(context.Context, durableexecution.Service, io.Reader, io.Writer) (roles.StdioApplication, error) {
			return nil, nil
		},
		runtimeStdioBuilderNotCalled(t),
	)
	if err != nil {
		t.Fatalf("NewStdioOpeningService: %v", err)
	}
	_, err = operation.OpenStdio(context.Background(), factorysessions.StdioOpeningRequest{
		FixtureCatalogPath: "fixtures.json", Input: &bytes.Buffer{}, Output: &bytes.Buffer{},
	})
	if err == nil || !owned.closed {
		t.Fatalf("OpenStdio error = %v, closed = %v, want fail-closed nil fixture application", err, owned.closed)
	}
}

type stdioApplicationStub struct{}

func (stdioApplicationStub) Run(context.Context) error { return nil }

type stdioExecutionOpeningStub struct {
	execution      roles.OwnedExecutionService
	opened         roles.OpenedExecutionRuntime
	resolvedRoot   string
	resolvedInput  string
	fixtureCatalog string
	runtimeOpened  bool
	opening        factorysessions.ExecutionRuntimeOpeningRequest
}

func (stub *stdioExecutionOpeningStub) ResolveProjectRoot(value string) (string, error) {
	stub.resolvedInput = value
	return stub.resolvedRoot, nil
}

func (stub *stdioExecutionOpeningStub) OpenExecutionRuntime(_ context.Context, request factorysessions.ExecutionRuntimeOpeningRequest) (roles.OpenedExecutionRuntime, error) {
	stub.runtimeOpened = true
	stub.opening = request
	return stub.opened, nil
}

func (stub *stdioExecutionOpeningStub) Build(_ context.Context, _, _, fixtureCatalogPath, _ string) (roles.OwnedExecutionService, error) {
	stub.fixtureCatalog = fixtureCatalogPath
	return stub.execution, nil
}

func fixtureStdioBuilderNotCalled(t *testing.T) roles.FixtureStdioApplicationBuilder {
	t.Helper()
	return func(context.Context, durableexecution.Service, io.Reader, io.Writer) (roles.StdioApplication, error) {
		t.Fatal("fixture builder called")
		return nil, nil
	}
}

func runtimeStdioBuilderNotCalled(t *testing.T) roles.RuntimeStdioApplicationBuilder {
	t.Helper()
	return func(context.Context, roles.OpenedExecutionRuntime, io.Reader, io.Writer) (roles.StdioApplication, error) {
		t.Fatal("runtime builder called")
		return nil, nil
	}
}
