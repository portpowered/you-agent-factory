package callbehavior

import (
	"sort"
	"strings"
)

// ProjectInstalledCallBehavior builds a deterministic call-behavior inventory for
// the currently installed JavaScript runtime surface. The descriptor is pure and
// read-only: it does not construct a goja VM or mutate installed bindings.
func ProjectInstalledCallBehavior() Inventory {
	records := installedCallBehaviorRecords()
	sort.SliceStable(records, func(i, j int) bool {
		return records[i].Path < records[j].Path
	})
	return Inventory{
		FormatVersion: FormatVersion,
		Records:       records,
	}
}

// ExpectedInstalledPaths returns the sorted full paths for the call-behavior
// inventory, aligned with the symbol-identity installed surface.
func ExpectedInstalledPaths() []string {
	records := installedCallBehaviorRecords()
	paths := make([]string, len(records))
	for i, record := range records {
		paths[i] = record.Path
	}
	sort.Strings(paths)
	return paths
}

func installedCallBehaviorRecords() []CallBehaviorRecord {
	return []CallBehaviorRecord{
		argsValueRecord(),
		metaValueRecord(),
		agentNamespaceRecord(),
		agentRunRecord(),
		logRecord(),
		parallelRecord(),
		phaseRecord(),
		pipelineRecord(),
		workflowNamespaceRecord(),
		workflowArtifactRecord(),
		workflowBudgetRecord(),
		workflowCheckpointRecord(),
		workflowFinalRecord(),
		workflowLogRecord(),
		workflowResumeStateRecord(),
	}
}

func argsValueRecord() CallBehaviorRecord {
	return CallBehaviorRecord{
		IDCandidate: idCandidate("args"),
		Path:        "args",
		Kind:        kindValue,
		Mutability:  "mutable-object",
		Nullability: "non-null",
		Lifecycle:   "snapshot-at-bind",
	}
}

func metaValueRecord() CallBehaviorRecord {
	return CallBehaviorRecord{
		IDCandidate: idCandidate("meta"),
		Path:        "meta",
		Kind:        kindValue,
		Mutability:  "mutable-object",
		Nullability: "non-null",
		Lifecycle:   "snapshot-at-bind",
	}
}

func agentNamespaceRecord() CallBehaviorRecord {
	return CallBehaviorRecord{
		IDCandidate: idCandidate("agent"),
		Path:        "agent",
		Kind:        kindNamespace,
		Mutability:  "fixed-binding",
		Nullability: "non-null",
		Lifecycle:   "live-namespace",
	}
}

func workflowNamespaceRecord() CallBehaviorRecord {
	return CallBehaviorRecord{
		IDCandidate: idCandidate("workflow"),
		Path:        "workflow",
		Kind:        kindNamespace,
		Mutability:  "fixed-binding",
		Nullability: "non-null",
		Lifecycle:   "live-namespace",
	}
}

func workflowFinalRecord() CallBehaviorRecord {
	return CallBehaviorRecord{
		IDCandidate: idCandidate("workflow.final"),
		Path:        "workflow.final",
		Kind:        kindFunction,
		Callable:    true,
		Parameters: []Parameter{
			{
				IDCandidate: "value",
				Name:        "value",
				Required:    false,
				Type:        "any",
			},
		},
		Return: &ReturnBehavior{
			SyncType: "undefined",
		},
		Determinism: "workflow.final wins over a returned workflow value for terminal result selection",
	}
}

func workflowCheckpointRecord() CallBehaviorRecord {
	return CallBehaviorRecord{
		IDCandidate: idCandidate("workflow.checkpoint"),
		Path:        "workflow.checkpoint",
		Kind:        kindFunction,
		Callable:    true,
		Parameters: []Parameter{
			{
				IDCandidate: "spec",
				Name:        "spec",
				Required:    true,
				Type:        "object",
				ObjectProperties: []ObjectProperty{
					idProperty("label", true, "string"),
					idProperty("state", false, "json-compatible"),
				},
			},
		},
		Return: &ReturnBehavior{
			SyncType: "undefined",
		},
		EmittedRecords: []string{"checkpoint"},
		Errors: []ErrorCase{
			{
				Condition: "missing-or-non-object-argument",
				Type:      "TypeError",
				Message:   "workflow.checkpoint() requires an object argument",
			},
			{
				Condition: "missing-label",
				Type:      "TypeError",
				Message:   `workflow.checkpoint() requires a string "label" property`,
			},
			{
				Condition: "non-json-state",
				Type:      "GoError",
				Message:   "workflow.checkpoint state must be JSON-compatible",
			},
		},
		ResumeNotes: "persists checkpoint state for workflow.resumeState() on resumed sessions",
	}
}

func workflowResumeStateRecord() CallBehaviorRecord {
	return CallBehaviorRecord{
		IDCandidate: idCandidate("workflow.resumeState"),
		Path:        "workflow.resumeState",
		Kind:        kindFunction,
		Callable:    true,
		Parameters:  nil,
		Return: &ReturnBehavior{
			SyncType: "object-or-undefined",
		},
		Errors: []ErrorCase{
			{
				Condition: "arguments-provided",
				Type:      "TypeError",
				Message:   "workflow.resumeState() does not accept arguments",
			},
		},
		ResumeNotes: "returns bound resume checkpoint state or undefined when absent",
	}
}

func workflowBudgetRecord() CallBehaviorRecord {
	return CallBehaviorRecord{
		IDCandidate: idCandidate("workflow.budget"),
		Path:        "workflow.budget",
		Kind:        kindFunction,
		Callable:    true,
		Return: &ReturnBehavior{
			SyncType: "object",
		},
		EmittedRecords: []string{"budget"},
		Errors: []ErrorCase{
			{
				Condition: "arguments-provided",
				Type:      "TypeError",
				Message:   "workflow.budget() does not accept arguments",
			},
		},
	}
}

func workflowLogRecord() CallBehaviorRecord {
	return logLikeRecord("workflow.log", "workflow.log()")
}

func logRecord() CallBehaviorRecord {
	return logLikeRecord("log", "log()")
}

func logLikeRecord(path, primitive string) CallBehaviorRecord {
	return CallBehaviorRecord{
		IDCandidate: idCandidate(path),
		Path:        path,
		Kind:        kindFunction,
		Callable:    true,
		Parameters: []Parameter{
			{
				IDCandidate: "message",
				Name:        "message",
				Required:    true,
				Type:        "string",
			},
			{
				IDCandidate: "fields",
				Name:        "fields",
				Required:    false,
				Type:        "object",
			},
		},
		Return: &ReturnBehavior{
			SyncType: "undefined",
		},
		EmittedRecords: []string{"log"},
		Errors: []ErrorCase{
			{
				Condition: "missing-or-non-string-message",
				Type:      "TypeError",
				Message:   primitive + " requires a string message",
			},
			{
				Condition: "non-json-fields",
				Type:      "GoError",
				Message:   primitive + " fields must be JSON-compatible",
			},
		},
	}
}

func workflowArtifactRecord() CallBehaviorRecord {
	return CallBehaviorRecord{
		IDCandidate: idCandidate("workflow.artifact"),
		Path:        "workflow.artifact",
		Kind:        kindFunction,
		Callable:    true,
		Parameters: []Parameter{
			{
				IDCandidate: "spec",
				Name:        "spec",
				Required:    true,
				Type:        "object",
				ObjectProperties: []ObjectProperty{
					idProperty("kind", true, "string"),
					idProperty("label", true, "string"),
					idProperty("content", false, "json-compatible"),
					idProperty("visibility", false, "string", "WORKFLOW_RUNTIME"),
				},
			},
		},
		Return: &ReturnBehavior{
			SyncType: "string",
		},
		EmittedRecords: []string{"artifact"},
		PolicyChecks: []PolicyCheck{
			{
				Kind:    "maxArtifactBytes",
				Field:   "content",
				Message: "policy denied: artifact content size exceeds maxArtifactBytes",
			},
		},
		Errors: []ErrorCase{
			{
				Condition: "missing-or-non-object-argument",
				Type:      "TypeError",
				Message:   "workflow.artifact() requires an object argument",
			},
			{
				Condition: "missing-kind-or-label",
				Type:      "TypeError",
				Message:   `workflow.artifact() requires string "kind" and "label" properties`,
			},
			{
				Condition: "non-json-content",
				Type:      "GoError",
				Message:   "workflow.artifact content must be JSON-compatible",
			},
		},
	}
}

func phaseRecord() CallBehaviorRecord {
	return CallBehaviorRecord{
		IDCandidate: idCandidate("phase"),
		Path:        "phase",
		Kind:        kindFunction,
		Callable:    true,
		Parameters: []Parameter{
			{
				IDCandidate: "name",
				Name:        "name",
				Required:    true,
				Type:        "string",
			},
		},
		Return: &ReturnBehavior{
			SyncType: "undefined",
		},
		EmittedRecords: []string{"phase"},
		Errors: []ErrorCase{
			{
				Condition: "missing-or-non-string-name",
				Type:      "TypeError",
				Message:   "phase() requires a string name",
			},
		},
	}
}

func agentRunRecord() CallBehaviorRecord {
	return CallBehaviorRecord{
		IDCandidate: idCandidate("agent.run"),
		Path:        "agent.run",
		Kind:        kindFunction,
		Callable:    true,
		Async:       true,
		Parameters: []Parameter{
			{
				IDCandidate: "spec",
				Name:        "spec",
				Required:    true,
				Type:        "object",
				ObjectProperties: []ObjectProperty{
					idProperty("prompt", true, "string"),
					idProperty("label", false, "string"),
					idProperty("preset", false, "string"),
					idProperty("modelProvider", false, "string"),
					idProperty("model", false, "string"),
					idProperty("reasoningEffort", false, "string"),
				},
			},
		},
		Return: &ReturnBehavior{
			Async:       true,
			PromiseType: "child-result-object",
		},
		EmittedRecords: []string{"child_dispatch"},
		PolicyChecks: []PolicyCheck{
			{Kind: "maxAgents", Message: "policy denied: requested fanout exceeds maxAgents"},
			{Kind: "allowedModels", Field: "model"},
			{Kind: "allowedReasoningEfforts", Field: "reasoningEffort"},
			{Kind: "allowedCommands", Field: "command"},
			{Kind: "sandboxMode", Field: "sandbox"},
			{Kind: "writableRoots", Field: "writableRoots"},
			{Kind: "allowNetwork", Field: "allowNetwork"},
			{Kind: "concurrency", Field: "concurrency"},
		},
		Errors: []ErrorCase{
			{
				Condition: "missing-or-non-object-argument",
				Type:      "TypeError",
				Message:   "agent.run() requires an object argument",
			},
			{
				Condition: "unsupported-field",
				Type:      "TypeError",
				Message:   `agent.run() does not support field`,
			},
			{
				Condition: "missing-prompt",
				Type:      "TypeError",
				Message:   `agent.run() requires a non-empty string "prompt" property`,
			},
			{
				Condition: "unknown-preset",
				Type:      "TypeError",
				Message:   "agent.run() references unknown operator worker preset",
			},
			{
				Condition: "unsupported-model-provider",
				Type:      "TypeError",
				Message:   "agent.run() has unsupported effective modelProvider",
			},
			{
				Condition: "unsupported-reasoning-effort",
				Type:      "TypeError",
				Message:   "agent.run() has unsupported effective reasoningEffort",
			},
		},
	}
}

func parallelRecord() CallBehaviorRecord {
	return CallBehaviorRecord{
		IDCandidate: idCandidate("parallel"),
		Path:        "parallel",
		Kind:        kindFunction,
		Callable:    true,
		Async:       true,
		Parameters: []Parameter{
			{
				IDCandidate: "items",
				Name:        "items",
				Required:    true,
				Type:        "array",
			},
		},
		Callback: &CallbackShape{
			Role: "item",
			Parameters: []Parameter{
				{
					IDCandidate: "this",
					Name:        "this",
					Required:    false,
					Type:        "undefined",
					Default:     "undefined",
				},
			},
			Notes: "array items may be agent run specs (object with prompt) or functions invoked with undefined this",
		},
		Return: &ReturnBehavior{
			Async:       true,
			PromiseType: "array",
		},
		EmittedRecords: []string{"child_dispatch"},
		PolicyChecks: []PolicyCheck{
			{Kind: "maxAgents", Message: "policy denied: requested fanout exceeds maxAgents"},
			{Kind: "childRequest", Message: "policy denied"},
		},
		Determinism: "agent-spec completion order may differ from input order under concurrency",
		Errors: []ErrorCase{
			{
				Condition: "missing-or-non-array-argument",
				Type:      "TypeError",
				Message:   "parallel() requires an array argument",
			},
			{
				Condition: "null-or-undefined-item",
				Type:      "TypeError",
				Message:   "parallel() items must not contain null or undefined entries",
			},
			{
				Condition: "invalid-item-shape",
				Type:      "TypeError",
				Message:   "parallel() items must be agent run specs or functions",
			},
		},
	}
}

func pipelineRecord() CallBehaviorRecord {
	return CallBehaviorRecord{
		IDCandidate: idCandidate("pipeline"),
		Path:        "pipeline",
		Kind:        kindFunction,
		Callable:    true,
		Async:       true,
		Parameters: []Parameter{
			{
				IDCandidate: "items",
				Name:        "items",
				Required:    true,
				Type:        "array",
			},
			{
				IDCandidate: "worker",
				Name:        "worker",
				Required:    true,
				Type:        "function",
			},
			{
				IDCandidate: "next",
				Name:        "next",
				Required:    false,
				Type:        "function",
			},
		},
		Callback: &CallbackShape{
			Role: "worker",
			Parameters: []Parameter{
				{IDCandidate: "this", Name: "this", Type: "undefined", Default: "undefined"},
				{IDCandidate: "item", Name: "item", Required: true, Type: "any"},
				{IDCandidate: "index", Name: "index", Required: true, Type: "number"},
			},
			Notes: "optional next stage receives (priorResult, item, index)",
		},
		Return: &ReturnBehavior{
			Async:       true,
			PromiseType: "pipeline-result-array",
		},
		Determinism: "stages run sequentially per item; worker and next may return values or Promises",
		Errors: []ErrorCase{
			{
				Condition: "missing-items-or-worker",
				Type:      "TypeError",
				Message:   "pipeline() requires items and worker arguments",
			},
			{
				Condition: "missing-or-non-array-items",
				Type:      "TypeError",
				Message:   "pipeline() requires an array items argument",
			},
			{
				Condition: "missing-worker-function",
				Type:      "TypeError",
				Message:   "pipeline() requires a worker function argument",
			},
			{
				Condition: "invalid-next-function",
				Type:      "TypeError",
				Message:   "pipeline() next argument must be a function when provided",
			},
			{
				Condition: "null-or-undefined-item",
				Type:      "TypeError",
				Message:   "pipeline() items must not contain null or undefined entries",
			},
		},
	}
}

func idProperty(name string, required bool, typ string, defaults ...string) ObjectProperty {
	prop := ObjectProperty{
		IDCandidate: strings.ReplaceAll(name, ".", "-"),
		Name:        name,
		Required:    required,
		Type:        typ,
	}
	if len(defaults) > 0 {
		prop.Default = defaults[0]
	}
	return prop
}

func idCandidate(path string) string {
	return strings.ReplaceAll(path, ".", "-")
}
