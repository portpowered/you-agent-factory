package workers

// RuntimeOpeningRequest contains the worker-side selections used while
// opening a Factory Session. The opened session receives these values through
// its explicit request-scoped build specification; Workers retains none of
// them on the process root.
type RuntimeOpeningRequest struct {
	RunnerID                          string
	Worktree                          string
	WorkerReasoningEffort             string
	MockWorkers                       *MockWorkersConfig
	InvocationSkipPermissionsOverride *bool
	SkipBuiltInPrerequisiteValidation bool
}
