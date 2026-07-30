package builtcliacceptance

// Scenario documents one S24 root-process acceptance scenario and the focused
// test that asserts its documented customer outcome.
type Scenario struct {
	ID                string
	Title             string
	DocumentedOutcome string
	TestName          string
}

// S24Scenarios returns the cross-surface root-process acceptance matrix in
// stable priority order. Each entry maps one customer scenario to the focused
// acceptance test that proves the documented outcome from the built you CLI.
func S24Scenarios() []Scenario {
	return []Scenario{
		{
			ID:    "s24-absent-provider",
			Title: "Absent provider",
			DocumentedOutcome: "Symbolic DEFAULT without concrete provider guidance fails with operator-visible " +
				"resolution guidance and non-zero exit.",
			TestName: "TestProviderPosture_Absent_UnresolvedDefaultRejectsWithDocumentedGuidance",
		},
		{
			ID:    "s24-configured-provider",
			Title: "Configured provider",
			DocumentedOutcome: "Explicit defaults.workerModelProvider in isolated home enables the documented " +
				"named @you/goal success path.",
			TestName: "TestProviderPosture_Configured_ExplicitHomeConfigEnablesNamedGoalSuccessPath",
		},
		{
			ID:    "s24-discovered-provider",
			Title: "Discovered provider",
			DocumentedOutcome: "YOU_DEFAULT_WORKER_MODEL_PROVIDER resolves symbolic DEFAULT when the operator " +
				"file omits provider defaults.",
			TestName: "TestProviderPosture_Discovered_EnvDefaultResolvesWithoutFileProvider",
		},
		{
			ID:                "s24-invalid-goal",
			Title:             "Invalid goal",
			DocumentedOutcome: "Unknown named factories fail with a non-zero operating-system exit in default and quiet modes.",
			TestName:          "TestCLIValidationFailureExitCode",
		},
		{
			ID:    "s24-terminal-failure-exit",
			Title: "Terminal invocation failure",
			DocumentedOutcome: "A terminal invocation failure exits the root process with a non-zero operating-system status, " +
				"writes diagnostics to stderr, and leaves stdout free of a false primary result.",
			TestName: "TestCLIFailureWritesDiagnosticToStderr",
		},
		{
			ID:    "s24-local-model-invoke",
			Title: "Local model invoke",
			DocumentedOutcome: "models invoke fails with documented bootstrap readiness guidance (pull or install) when " +
				"managed local assets are absent under isolated home/log directories.",
			TestName: "TestLocalModelInvoke_MissingReadiness_FailsWithDocumentedBootstrapGuidance",
		},
		{
			ID:    "s24-goal-repeat",
			Title: "Goal repeat",
			DocumentedOutcome: "Repeated named @you/goal JSON runs assign distinct requestId/traceId while reusing the " +
				"installed global factory copy without cross-run home/log contamination.",
			TestName: "TestGoalRepeat_RepeatedNamedRunsAssignDistinctInvocationIdentityAndReuseInstalledCopy",
		},
		{
			ID:    "s24-subagent",
			Title: "Subagent",
			DocumentedOutcome: "Named @you/subagent invocation returns the documented primary JSON terminal outcome on a " +
				"deterministic mock-worker fixture.",
			TestName: "TestSubagentInvocation_SuccessfulNamedRun_ReturnsAuthoritativePrimaryResultJSON",
		},
	}
}

// ScenarioByID returns the scenario for id or nil when id is unknown.
func ScenarioByID(id string) *Scenario {
	for _, scenario := range S24Scenarios() {
		if scenario.ID == id {
			copy := scenario
			return &copy
		}
	}
	return nil
}
