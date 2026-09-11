package dispatch

func projectCycleSelectionFactoryConfig() map[string]any {
	return map[string]any{
		"name":         "project-cycle-selection",
		"workTypes":    projectCycleSelectionWorkTypes(),
		"workers":      projectCycleSelectionWorkers(),
		"workstations": projectCycleSelectionWorkstations(),
	}
}

func projectCycleSelectionWorkTypes() []map[string]any {
	return []map[string]any{
		{
			"name": "project",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "waiting", "type": "PROCESSING"},
				{"name": "needs-supervision", "type": "PROCESSING"},
				{"name": "blocked", "type": "FAILED"},
			},
		},
		{
			"name": "project-cycle",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "blocked", "type": "FAILED"},
			},
		},
		{
			"name": "reopen-gate",
			"states": []map[string]string{
				{"name": "ready", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
			},
		},
		{
			"name": "block-gate",
			"states": []map[string]string{
				{"name": "ready", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
			},
		},
		{
			"name": "thoughts",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
			},
		},
	}
}

func projectCycleSelectionWorkers() []map[string]any {
	return []map[string]any{{
		"name":    "project-cycle-emitter",
		"type":    "SCRIPT_WORKER",
		"command": "python",
	}}
}

func projectCycleSelectionWorkstations() []map[string]any {
	return []map[string]any{
		{
			"name":    "project-lead",
			"type":    "SCRIPT_RUN",
			"worker":  "project-cycle-emitter",
			"inputs":  []map[string]string{{"workType": "project", "state": "init"}},
			"outputs": []map[string]string{{"workType": "project", "state": "waiting"}},
		},
		{
			"name": "reopen-project",
			"type": "LOGICAL_MOVE",
			"inputs": []map[string]string{
				{"workType": "project", "state": "waiting"},
				{"workType": "reopen-gate", "state": "ready"},
			},
			"outputs": []map[string]string{
				{"workType": "project", "state": "init"},
				{"workType": "reopen-gate", "state": "complete"},
			},
		},
		projectCycleBlockProjectWorkstation(),
		{
			"name":   "escalate-project",
			"type":   "LOGICAL_MOVE",
			"inputs": []map[string]string{{"workType": "project", "state": "needs-supervision"}},
			"outputs": []map[string]string{
				{"workType": "project", "state": "blocked"},
				{"workType": "thoughts", "state": "init"},
			},
		},
		{
			"name":    "complete-thought",
			"type":    "LOGICAL_MOVE",
			"inputs":  []map[string]string{{"workType": "thoughts", "state": "init"}},
			"outputs": []map[string]string{{"workType": "thoughts", "state": "complete"}},
		},
	}
}

func projectCycleBlockProjectWorkstation() map[string]any {
	return map[string]any{
		"name": "block-project",
		"type": "LOGICAL_MOVE",
		"inputs": []map[string]any{
			{
				"workType": "project",
				"state":    "waiting",
				"guards":   []map[string]string{{"type": "SAME_NAME", "matchInput": "project-cycle"}},
			},
			{"workType": "project-cycle", "state": "blocked"},
			{"workType": "block-gate", "state": "ready"},
		},
		"outputs": []map[string]string{
			{"workType": "project", "state": "needs-supervision"},
			{"workType": "block-gate", "state": "complete"},
		},
	}
}
