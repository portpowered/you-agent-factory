package testkit

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
)

func TestInMemoryRunnerConformsToCommonContract(t *testing.T) {
	Run(t, NewInMemorySubject())
}

func TestRunServiceProvesCommonContractThroughServiceExecuteBoundary(t *testing.T) {
	subject := NewInMemorySubject()
	service := &inMemoryConformanceService{runner: subject.Runner}

	RunService(t, ServiceSubject{
		Service:                   service,
		Identity:                  "memory",
		ValidRequest:              subject.ValidRequest,
		InvalidRequest:            subject.InvalidRequest,
		UnsupportedRequest:        subject.UnsupportedRequest,
		FailureRequest:            subject.FailureRequest,
		ExpectedResult:            subject.ExpectedResult,
		CapturedRequest:           subject.CapturedRequest,
		SkipUnsupportedCapability: subject.SkipUnsupportedCapability,
	})
}

func TestSubjectExecuteRequiresRunner(t *testing.T) {
	_, err := (Subject{}).execute(t.Context(), workers.RunnerExecutionRequest{})
	if err == nil {
		t.Fatal("Subject.execute() error = nil, want missing-runner error")
	}
}

func TestInMemoryRunnerCapturedRequestReportsNoCall(t *testing.T) {
	var runner *InMemoryRunner
	_, ok := runner.CapturedRequest()
	if ok {
		t.Fatal("CapturedRequest() ok = true before execution, want false")
	}
}

func TestInMemoryRunnerConformsWithNilDiagnostics(t *testing.T) {
	subject := NewInMemorySubject()
	runner, ok := subject.Runner.(*InMemoryRunner)
	if !ok {
		t.Fatalf("subject runner type = %T, want *InMemoryRunner", subject.Runner)
	}
	runner.result.Diagnostics = nil
	subject.ExpectedResult.Diagnostics = nil

	Run(t, subject)
}

// inMemoryConformanceService adapts the foundational in-memory Runner into
// runners.Service so RunService can be proven through the real
// runners.Service.Execute boundary rather than the direct Runner interface.
type inMemoryConformanceService struct {
	runner interface {
		Execute(context.Context, runners.AttemptRequest) (runners.AttemptResult, error)
	}
}

func (service *inMemoryConformanceService) Resolve(runners.ResolutionRequest) (runners.Binding, error) {
	return runners.Binding{}, nil
}

func (service *inMemoryConformanceService) Execute(
	ctx context.Context,
	request runners.ExecuteRequest,
) (runners.ExecuteResult, error) {
	return service.runner.Execute(ctx, request.Attempt)
}
