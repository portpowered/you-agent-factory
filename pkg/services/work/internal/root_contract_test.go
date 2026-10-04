package internal_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	internalservice "github.com/portpowered/infinite-you/pkg/services/work/internal"
)

func TestNewServiceSatisfiesPublishedWorkRoot(t *testing.T) {
	t.Parallel()

	request := work.WorkRequest{RequestID: "internal-root-admission"}
	called := false
	service := internalservice.NewService(nil, nil, nil, nil, nil,
		completedStateAccess{submit: func(ctx context.Context, session string, got work.WorkRequest) (work.WorkRequestSubmitResult, error) {
			if ctx != t.Context() || session != "session-1" || got.RequestID != request.RequestID {
				t.Fatal("root changed admission inputs")
			}
			called = true
			return work.WorkRequestSubmitResult{Accepted: true}, nil
		}}, nil, nil)
	var root work.Service = service
	result, err := root.SubmitWorkRequestForSession(t.Context(), "session-1", request)
	if err != nil || !result.Accepted || !called {
		t.Fatalf("root admission = %#v, %v, called=%t", result, err, called)
	}
}
