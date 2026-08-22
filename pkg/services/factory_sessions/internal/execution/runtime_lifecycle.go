package factorysessionexecution

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Close cancels every asynchronous durable session owned by this service and
// waits for the corresponding execution goroutines to finish their terminal
// projection and persistence before returning. The owner is closed exactly
// once; subsequent calls return the same shutdown result.
func (s *JavaScriptRuntimeService) Close() error {
	return s.closeWithTimeout(durableExecutionShutdownTimeout)
}

func (s *JavaScriptRuntimeService) closeWithTimeout(timeout time.Duration) error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		s.runLifecycleMu.Lock()
		s.runClosed = true
		s.runLifecycleMu.Unlock()

		s.cancelAsyncRuns()
		done := make(chan struct{})
		go func() {
			s.runWaitGroup.Wait()
			close(done)
		}()

		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case <-done:
		case <-timer.C:
			s.closeErr = fmt.Errorf(
				"close durable session execution: %w",
				ErrDurableExecutionShutdownTimeout,
			)
		}
	})
	return s.closeErr
}

func (s *JavaScriptRuntimeService) cancelAsyncRuns() {
	s.mu.RLock()
	cancels := make([]context.CancelFunc, 0, len(s.sessions))
	for _, state := range s.sessions {
		if state != nil && state.runCancel != nil {
			cancels = append(cancels, state.runCancel)
		}
	}
	s.mu.RUnlock()
	for _, cancel := range cancels {
		cancel()
	}
}

func (s *JavaScriptRuntimeService) launchAsyncRun(run func()) error {
	if run == nil {
		return errors.New("durable execution run is required")
	}
	s.runLifecycleMu.Lock()
	if s.runClosed {
		s.runLifecycleMu.Unlock()
		return ErrDurableExecutionClosed
	}
	s.runWaitGroup.Add(1)
	s.runLifecycleMu.Unlock()
	go func() {
		defer s.runWaitGroup.Done()
		run()
	}()
	return nil
}

func (s *JavaScriptRuntimeService) ensureOpen() error {
	s.runLifecycleMu.Lock()
	defer s.runLifecycleMu.Unlock()
	if s.runClosed {
		return ErrDurableExecutionClosed
	}
	return nil
}
