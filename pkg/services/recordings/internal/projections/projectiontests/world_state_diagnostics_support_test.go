package projections_test

import workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

func projectionSafeResponseDiagnostics() *workerexecution.SafeWorkDiagnostics {
	return &workerexecution.SafeWorkDiagnostics{
		RenderedPrompt: &workerexecution.SafeRenderedPromptDiagnostic{
			SystemPromptHash: "system-hash",
			UserMessageHash:  "user-hash",
			Variables: map[string]string{
				"prompt_source":  "factory-renderer",
				"work_type_name": "task",
			},
		},
		Provider: &workerexecution.SafeProviderDiagnostic{
			Provider: "codex",
			Model:    "gpt-5.4",
			RequestMetadata: map[string]string{
				"worker_type": "builder",
			},
			ResponseMetadata: map[string]string{
				"provider_session_id": "resp-1",
				"retry_count":         "1",
			},
		},
	}
}
