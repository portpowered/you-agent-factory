package workers

// RuntimeSelection contains worker values for one Factory Session. The
// session receives them through its request-scoped build specification;
// Workers retains none on the process root.
type RuntimeSelection struct {
	RunnerID                          string
	Worktree                          string
	WorkerReasoningEffort             string
	MockWorkers                       *MockWorkersConfig
	InvocationSkipPermissionsOverride *bool
	SkipBuiltInPrerequisiteValidation bool
}
