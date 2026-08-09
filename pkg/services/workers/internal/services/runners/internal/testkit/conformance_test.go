package testkit

import (
	"context"
	"testing"

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
