package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func (r *registry) lookupStart(req workersessions.StartRequest) (*startReplay, error) {
	tuple := startTupleFor(req)
	r.mu.RLock()
	defer r.mu.RUnlock()
	replay := r.startReplays[req.RequestID]
	if replay != nil && !reflect.DeepEqual(replay.tuple, tuple) {
		return nil, workersessions.ErrStartRequestIDConflict
	}
	return replay, nil
}

func (r *registry) validateStartExecution(ctx context.Context, executor workers.Service, req workersessions.StartRequest) error {
	request, err := executeRequestFromSessionDispatch(req.Execution)
	if err == nil {
		err = executor.ValidateExecution(ctx, request)
	}
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	// Availability is an admission failure even when Workers wraps it in its
	// invalid-request sentinel. Transport errors expose bounded messages.
	if errors.Is(err, providers.ErrProviderUnavailable) || errors.Is(err, workers.ErrExecuteUnavailable) {
		return fmt.Errorf("%w: %w", workersessions.ErrStartAdmissionFailed, err)
	}
	return fmt.Errorf("%w: %w", workersessions.ErrInvalidExecutionRequest, err)
}
