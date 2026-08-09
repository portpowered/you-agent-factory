package executor

import "time"

// PrintTimeoutFromWorkerTimeout parses the authored worker timeout for native
// providers that expose their own print-mode deadline. Invalid values return
// zero; workstation execution remains responsible for reporting the authored
// timeout error before dispatch.
func PrintTimeoutFromWorkerTimeout(raw string) time.Duration {
	if raw == "" {
		return 0
	}
	timeout, err := time.ParseDuration(raw)
	if err != nil || timeout <= 0 {
		return 0
	}
	return timeout
}
