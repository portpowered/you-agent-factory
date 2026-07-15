package mockworkers

// ContractFieldDocumentation carries stable documentation metadata for one inventoried field.
type ContractFieldDocumentation struct {
	Title       string
	Description string
	Examples    []any
}

// ProjectContractFieldDocumentation returns canonical English and examples for every inventoried field.
func ProjectContractFieldDocumentation() map[string]ContractFieldDocumentation {
	return map[string]ContractFieldDocumentation{
		"mockWorkers": {
			Title:       "Mock worker entries",
			Description: "Ordered mock-worker entries matched in array order; the first matching entry wins.",
			Examples: []any{
				[]map[string]any{
					{
						"id":              "reviewer-rejects-first-pass",
						"workerName":      "reviewer",
						"workstationName": "review-story",
						"runType":         "reject",
						"rejectConfig": map[string]any{
							"stdout":   "needs changes",
							"stderr":   "missing acceptance criteria",
							"exitCode": 42,
						},
					},
				},
			},
		},
		"unmatchedDispatchPolicy": {
			Title:       "Unmatched dispatch policy",
			Description: "Controls dispatches that do not match any mockWorkers[] entry. Omitted values behave as accept.",
			Examples:    []any{"accept", "passthrough"},
		},
		"mockWorkers[].id": {
			Title:       "Mock worker entry identifier",
			Description: "Optional entry identifier used for diagnostics and stable matching references.",
			Examples:    []any{"reviewer-rejects-first-pass", "executor-script-side-effect"},
		},
		"mockWorkers[].workerName": {
			Title:       "Worker name selector",
			Description: "Optional worker-name selector. Omitted values do not constrain worker-name matching.",
			Examples:    []any{"reviewer", "executor"},
		},
		"mockWorkers[].workstationName": {
			Title:       "Workstation name selector",
			Description: "Optional workstation-name selector. Omitted values do not constrain workstation matching.",
			Examples:    []any{"review-story", "execute-story"},
		},
		"mockWorkers[].workInputs": {
			Title:       "Work input selectors",
			Description: "Optional consumed work-input selectors. All specified selector fields on an entry must match.",
			Examples: []any{
				[]map[string]any{
					{
						"workType":  "story",
						"state":     "in-review",
						"inputName": "work",
					},
				},
			},
		},
		"mockWorkers[].runType": {
			Title:       "Mock worker run type",
			Description: "Required run-type union selecting accept, script, or reject behavior.",
			Examples:    []any{"accept", "script", "reject"},
		},
		"mockWorkers[].scriptConfig": {
			Title:       "Script configuration",
			Description: "Required when runType is script. Declares the local script executed through the shared command-runner boundary.",
			Examples: []any{
				map[string]any{
					"command": "printf",
					"args":    []string{"mock script stdout\n"},
					"timeout": "30s",
				},
			},
		},
		"mockWorkers[].rejectConfig": {
			Title:       "Reject configuration",
			Description: "Optional observable output for a rejected mock result when runType is reject.",
			Examples: []any{
				map[string]any{
					"stdout":   "needs changes",
					"stderr":   "missing acceptance criteria",
					"exitCode": 42,
				},
			},
		},
		"mockWorkers[].workInputs[].workId": {
			Title:       "Work identifier selector",
			Description: "Optional work identifier selector. Omitted selector fields do not constrain that dimension.",
			Examples:    []any{"work-123"},
		},
		"mockWorkers[].workInputs[].workType": {
			Title:       "Work type selector",
			Description: "Optional work type selector. Omitted selector fields do not constrain that dimension.",
			Examples:    []any{"story"},
		},
		"mockWorkers[].workInputs[].state": {
			Title:       "Work state selector",
			Description: "Optional work state selector. Omitted selector fields do not constrain that dimension.",
			Examples:    []any{"in-review", "init"},
		},
		"mockWorkers[].workInputs[].inputName": {
			Title:       "Input name selector",
			Description: "Optional consumed input name selector. Omitted selector fields do not constrain that dimension.",
			Examples:    []any{"work"},
		},
		"mockWorkers[].workInputs[].traceId": {
			Title:       "Trace identifier selector",
			Description: "Optional trace identifier selector. Omitted selector fields do not constrain that dimension.",
			Examples:    []any{"trace-abc"},
		},
		"mockWorkers[].workInputs[].channel": {
			Title:       "Channel selector",
			Description: "Optional channel selector. Omitted selector fields do not constrain that dimension.",
			Examples:    []any{"default"},
		},
		"mockWorkers[].workInputs[].payloadHash": {
			Title:       "Payload hash selector",
			Description: "Optional payload hash selector. Omitted selector fields do not constrain that dimension.",
			Examples:    []any{"sha256:deadbeef"},
		},
		"mockWorkers[].scriptConfig.command": {
			Title:       "Script command",
			Description: "Required non-empty command when runType is script.",
			Examples:    []any{"printf", "echo"},
		},
		"mockWorkers[].scriptConfig.args": {
			Title:       "Script arguments",
			Description: "Optional command arguments passed to the script command.",
			Examples:    []any{[]string{"mock script stdout\n"}},
		},
		"mockWorkers[].scriptConfig.env": {
			Title:       "Script environment",
			Description: "Optional environment variables for local script execution.",
			Examples:    []any{map[string]string{"MOCK_WORKER": "1"}},
		},
		"mockWorkers[].scriptConfig.workingDirectory": {
			Title:       "Script working directory",
			Description: "Optional working directory for local script execution.",
			Examples:    []any{"."},
		},
		"mockWorkers[].scriptConfig.stdin": {
			Title:       "Script stdin",
			Description: "Optional stdin payload for local script execution.",
			Examples:    []any{"optional script stdin payload"},
		},
		"mockWorkers[].scriptConfig.timeout": {
			Title:       "Script execution timeout",
			Description: "Optional local script execution time bound. Not a dispatch delay or timing field.",
			Examples:    []any{"30s"},
		},
		"mockWorkers[].rejectConfig.stdout": {
			Title:       "Rejected stdout",
			Description: "Optional stdout content for a rejected mock result.",
			Examples:    []any{"needs changes"},
		},
		"mockWorkers[].rejectConfig.stderr": {
			Title:       "Rejected stderr",
			Description: "Optional stderr content for a rejected mock result.",
			Examples:    []any{"missing acceptance criteria"},
		},
		"mockWorkers[].rejectConfig.exitCode": {
			Title:       "Rejected exit code",
			Description: "Optional rejected exit code between 1 and 255.",
			Examples:    []any{42},
		},
	}
}
