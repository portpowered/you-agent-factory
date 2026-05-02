package workers

const (
	omitZeroWorkerEventExitCode    = false
	includeZeroWorkerEventExitCode = true
)

func workerEventExitCode(exitCode int, present bool, includeZero bool) *int {
	if !present {
		return nil
	}
	if exitCode == 0 && !includeZero {
		return nil
	}
	exitCodeCopy := exitCode
	return &exitCodeCopy
}
