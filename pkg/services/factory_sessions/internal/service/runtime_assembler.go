package service

import (
	"context"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"sync"
)

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func cloneInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

// successorBoardStartup selects a resumed writer only after its initial
// publication succeeds. Failed publication leaves the previous board selected.
type successorBoardStartup struct {
	roles.LifecycleRuntime
	publish func(context.Context) error
	once    sync.Once
	err     error
}

func (startup *successorBoardStartup) CompleteStartup(ctx context.Context) error {
	startup.once.Do(func() {
		startup.err = startup.LifecycleRuntime.CompleteStartup(ctx)
		if startup.err == nil {
			if err := startup.publish(ctx); err != nil {
				startup.err = startup.FailStartup(err)
			}
		}
	})
	return startup.err
}
