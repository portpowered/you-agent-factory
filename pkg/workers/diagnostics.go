package workers

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func workDiagnosticsForInferenceRequest(req interfaces.ProviderInferenceRequest) *interfaces.WorkDiagnostics {
	return &interfaces.WorkDiagnostics{
		RenderedPrompt: &interfaces.RenderedPromptDiagnostic{
			SystemPromptHash: hashText(req.SystemPrompt),
			UserMessageHash:  hashText(req.UserMessage),
		},
		Provider: &interfaces.ProviderDiagnostic{
			Provider: req.ModelProvider,
			Model:    req.Model,
			RequestMetadata: map[string]string{
				"worker_type":       firstNonEmpty(req.WorkerType, req.Dispatch.WorkerType),
				"workstation_type":  req.WorkstationType,
				"worktree":          req.Worktree,
				"working_directory": req.WorkingDirectory,
				"session_id":        req.SessionID,
				"output_schema":     req.OutputSchema,
			},
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

func withInferenceResponseDiagnostics(base *interfaces.WorkDiagnostics, resp interfaces.InferenceResponse, retryCount int) *interfaces.WorkDiagnostics {
	diagnostics := cloneWorkDiagnostics(base)
	diagnostics = mergeWorkDiagnostics(diagnostics, resp.Diagnostics)
	if diagnostics == nil {
		diagnostics = &interfaces.WorkDiagnostics{}
	}
	if diagnostics.Provider == nil {
		diagnostics.Provider = &interfaces.ProviderDiagnostic{}
	}
	if diagnostics.Provider.ResponseMetadata == nil {
		diagnostics.Provider.ResponseMetadata = make(map[string]string)
	}
	diagnostics.Provider.ResponseMetadata["content_bytes"] = fmt.Sprintf("%d", len(resp.Content))
	diagnostics.Provider.ResponseMetadata["retry_count"] = fmt.Sprintf("%d", retryCount)
	if resp.ProviderSession != nil {
		diagnostics.Provider.ResponseMetadata["provider_session_provider"] = resp.ProviderSession.Provider
		diagnostics.Provider.ResponseMetadata["provider_session_kind"] = resp.ProviderSession.Kind
		diagnostics.Provider.ResponseMetadata["provider_session_id"] = resp.ProviderSession.ID
	}
	return diagnostics
}

func withInferenceErrorDiagnostics(base *interfaces.WorkDiagnostics, err error, retryCount int) *interfaces.WorkDiagnostics {
	diagnostics := cloneWorkDiagnostics(base)
	if diagnostics == nil {
		diagnostics = &interfaces.WorkDiagnostics{}
	}
	if diagnostics.Provider == nil {
		diagnostics.Provider = &interfaces.ProviderDiagnostic{}
	}
	if diagnostics.Provider.ResponseMetadata == nil {
		diagnostics.Provider.ResponseMetadata = make(map[string]string)
	}
	diagnostics.Provider.ResponseMetadata["error"] = err.Error()
	diagnostics.Provider.ResponseMetadata["retry_count"] = fmt.Sprintf("%d", retryCount)
	return diagnostics
}

func commandDiagnostics(req CommandRequest, result CommandResult, duration time.Duration, timedOut bool) *interfaces.WorkDiagnostics {
	envProjection := ProjectCommandEnvForDiagnostics(req.Env)
	return &interfaces.WorkDiagnostics{
		Command: &interfaces.CommandDiagnostic{
			Command:    req.Command,
			Args:       append([]string(nil), req.Args...),
			Stdin:      string(req.Stdin),
			Env:        envProjection.Values,
			Stdout:     string(result.Stdout),
			Stderr:     string(result.Stderr),
			ExitCode:   result.ExitCode,
			TimedOut:   timedOut,
			Duration:   duration,
			WorkingDir: req.WorkDir,
		},
		Metadata: commandEnvDiagnosticMetadata(envProjection),
	}
}

func mergeWorkDiagnostics(base, overlay *interfaces.WorkDiagnostics) *interfaces.WorkDiagnostics {
	if base == nil {
		return cloneWorkDiagnostics(overlay)
	}
	if overlay == nil {
		return base
	}
	if overlay.RenderedPrompt != nil {
		base.RenderedPrompt = cloneRenderedPromptDiagnostic(overlay.RenderedPrompt)
	}
	if overlay.Provider != nil {
		base.Provider = mergeProviderDiagnostic(base.Provider, overlay.Provider)
	}
	if overlay.Command != nil {
		base.Command = cloneCommandDiagnostic(overlay.Command)
	}
	if overlay.Panic != nil {
		base.Panic = &interfaces.PanicDiagnostic{Message: overlay.Panic.Message, Stack: overlay.Panic.Stack}
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

func cloneWorkDiagnostics(in *interfaces.WorkDiagnostics) *interfaces.WorkDiagnostics {
	if in == nil {
		return nil
	}
	out := &interfaces.WorkDiagnostics{
		RenderedPrompt: cloneRenderedPromptDiagnostic(in.RenderedPrompt),
		Provider:       cloneProviderDiagnostic(in.Provider),
		Command:        cloneCommandDiagnostic(in.Command),
		Metadata:       cloneStringMap(in.Metadata),
	}
	if in.Panic != nil {
		out.Panic = &interfaces.PanicDiagnostic{Message: in.Panic.Message, Stack: in.Panic.Stack}
	}
	return out
}

func cloneRenderedPromptDiagnostic(in *interfaces.RenderedPromptDiagnostic) *interfaces.RenderedPromptDiagnostic {
	if in == nil {
		return nil
	}
	return &interfaces.RenderedPromptDiagnostic{
		SystemPromptHash: in.SystemPromptHash,
		UserMessageHash:  in.UserMessageHash,
		Variables:        cloneStringMap(in.Variables),
	}
}

func mergeProviderDiagnostic(base, overlay *interfaces.ProviderDiagnostic) *interfaces.ProviderDiagnostic {
	if base == nil {
		return cloneProviderDiagnostic(overlay)
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

func cloneProviderDiagnostic(in *interfaces.ProviderDiagnostic) *interfaces.ProviderDiagnostic {
	if in == nil {
		return nil
	}
	return &interfaces.ProviderDiagnostic{
		Provider:         in.Provider,
		Model:            in.Model,
		RequestMetadata:  cloneStringMap(in.RequestMetadata),
		ResponseMetadata: cloneStringMap(in.ResponseMetadata),
	}
}

func cloneCommandDiagnostic(in *interfaces.CommandDiagnostic) *interfaces.CommandDiagnostic {
	if in == nil {
		return nil
	}
	return &interfaces.CommandDiagnostic{
		Command:    in.Command,
		Args:       append([]string(nil), in.Args...),
		Stdin:      in.Stdin,
		Env:        cloneStringMap(in.Env),
		Stdout:     in.Stdout,
		Stderr:     in.Stderr,
		ExitCode:   in.ExitCode,
		TimedOut:   in.TimedOut,
		Duration:   in.Duration,
		WorkingDir: in.WorkingDir,
	}
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func envSliceToMap(env []string) map[string]string {
	if len(env) == 0 {
		return nil
	}
	out := make(map[string]string, len(env))
	for _, pair := range env {
		name, value, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		out[name] = value
	}
	return out
}

func hashText(value string) string {
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
