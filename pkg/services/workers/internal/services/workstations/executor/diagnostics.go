package executor

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func workDiagnosticsForInferenceRequest(req workerexecution.ProviderInferenceRequest) *workerexecution.WorkDiagnostics {
	requestMetadata := map[string]string{
		"worker_type":       firstNonEmpty(req.WorkerType, req.Dispatch.WorkerType),
		"workstation_type":  req.WorkstationType,
		"worktree":          req.Worktree,
		"working_directory": req.WorkingDirectory,
		"session_id":        req.SessionID,
		"output_schema":     req.OutputSchema,
	}
	if req.OpenCodeAgent != "" {
		requestMetadata["opencode_agent"] = req.OpenCodeAgent
	}
	return &workerexecution.WorkDiagnostics{
		RenderedPrompt: &workerexecution.RenderedPromptDiagnostic{
			SystemPromptHash: hashText(req.SystemPrompt),
			UserMessageHash:  hashText(req.UserMessage),
		},
		Provider: &workerexecution.ProviderDiagnostic{
			Provider:        req.ModelProvider,
			Model:           req.Model,
			RequestMetadata: requestMetadata,
		},
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func withInferenceResponseDiagnostics(base *workerexecution.WorkDiagnostics, resp workerexecution.InferenceResponse, retryCount int) *workerexecution.WorkDiagnostics {
	diagnostics := workerexecution.CloneWorkDiagnostics(base)
	diagnostics = mergeWorkDiagnostics(diagnostics, resp.Diagnostics)
	if diagnostics == nil {
		diagnostics = &workerexecution.WorkDiagnostics{}
	}
	if diagnostics.Provider == nil {
		diagnostics.Provider = &workerexecution.ProviderDiagnostic{}
	}
	if diagnostics.Provider.ResponseMetadata == nil {
		diagnostics.Provider.ResponseMetadata = make(map[string]string)
	}
	diagnostics.Provider.ResponseMetadata["content_bytes"] = fmt.Sprintf("%d", len(resp.Content))
	diagnostics.Provider.ResponseMetadata["retry_count"] = fmt.Sprintf("%d", retryCount)
	if resp.ProviderSession != nil {
		diagnostics.Provider.ResponseMetadata["provider_session_provider"] = workerexecution.CanonicalProviderSessionProvider(resp.ProviderSession.Provider)
		diagnostics.Provider.ResponseMetadata["provider_session_kind"] = resp.ProviderSession.Kind
		diagnostics.Provider.ResponseMetadata["provider_session_id"] = resp.ProviderSession.ID
	}
	return diagnostics
}

func withInferenceErrorDiagnostics(base *workerexecution.WorkDiagnostics, err error, retryCount int) *workerexecution.WorkDiagnostics {
	diagnostics := workerexecution.CloneWorkDiagnostics(base)
	if diagnostics == nil {
		diagnostics = &workerexecution.WorkDiagnostics{}
	}
	if diagnostics.Provider == nil {
		diagnostics.Provider = &workerexecution.ProviderDiagnostic{}
	}
	if diagnostics.Provider.ResponseMetadata == nil {
		diagnostics.Provider.ResponseMetadata = make(map[string]string)
	}
	diagnostics.Provider.ResponseMetadata["error"] = err.Error()
	diagnostics.Provider.ResponseMetadata["retry_count"] = fmt.Sprintf("%d", retryCount)
	return diagnostics
}

func mergeWorkDiagnostics(base, overlay *workerexecution.WorkDiagnostics) *workerexecution.WorkDiagnostics {
	if base == nil {
		return workerexecution.CloneWorkDiagnostics(overlay)
	}
	if overlay == nil {
		return base
	}
	overlay = workerexecution.CloneWorkDiagnostics(overlay)
	if overlay.RenderedPrompt != nil {
		base.RenderedPrompt = overlay.RenderedPrompt
	}
	if overlay.Provider != nil {
		base.Provider = mergeProviderDiagnostic(base.Provider, overlay.Provider)
	}
	if overlay.Invocation != nil {
		base.Invocation = workerexecution.CloneWorkDiagnostics(&workerexecution.WorkDiagnostics{Invocation: overlay.Invocation}).Invocation
	}
	if overlay.Command != nil {
		base.Command = overlay.Command
	}
	if overlay.Panic != nil {
		base.Panic = overlay.Panic
	}
	if len(overlay.Metadata) > 0 {
		if base.Metadata == nil {
			base.Metadata = make(map[string]string, len(overlay.Metadata))
		}
		for k, v := range overlay.Metadata {
			base.Metadata[k] = v
		}
	}
	return base
}

func mergeProviderDiagnostic(base, overlay *workerexecution.ProviderDiagnostic) *workerexecution.ProviderDiagnostic {
	if base == nil {
		return overlay
	}
	if overlay.Provider != "" {
		base.Provider = overlay.Provider
	}
	if overlay.Model != "" {
		base.Model = overlay.Model
	}
	if len(overlay.RequestMetadata) > 0 {
		if base.RequestMetadata == nil {
			base.RequestMetadata = make(map[string]string, len(overlay.RequestMetadata))
		}
		for k, v := range overlay.RequestMetadata {
			base.RequestMetadata[k] = v
		}
	}
	if len(overlay.ResponseMetadata) > 0 {
		if base.ResponseMetadata == nil {
			base.ResponseMetadata = make(map[string]string, len(overlay.ResponseMetadata))
		}
		for k, v := range overlay.ResponseMetadata {
			base.ResponseMetadata[k] = v
		}
	}
	return base
}

func hashText(value string) string {
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
