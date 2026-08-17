package workers

import "fmt"

// WorkerExecutorPanicError is the Workers-owned typed failure returned when a
// configured WorkerExecutor panics during execution. Cause retains the
// recovered value for errors.As-based inspection while Error preserves the
// established diagnostic text.
type WorkerExecutorPanicError struct {
	Cause any
}

func (e *WorkerExecutorPanicError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("executor panic: %v", e.Cause)
}

// Unwrap exposes the recovered cause when it is itself an error.
func (e *WorkerExecutorPanicError) Unwrap() error {
	if e == nil {
		return nil
	}
	cause, _ := e.Cause.(error)
	return cause
}
