package interfaces

// CloneWorkDiagnostics returns a detached copy of canonical worker-facing
// diagnostics.
func CloneWorkDiagnostics(diagnostics *WorkDiagnostics) *WorkDiagnostics {
	if diagnostics == nil {
		return nil
	}

	clone := &WorkDiagnostics{
		RenderedPrompt: cloneRenderedPromptDiagnostic(diagnostics.RenderedPrompt),
		Provider:       cloneProviderDiagnostic(diagnostics.Provider),
		Command:        cloneCommandDiagnostic(diagnostics.Command),
		Metadata:       cloneStringMap(diagnostics.Metadata),
	}
	if diagnostics.Panic != nil {
		clone.Panic = &PanicDiagnostic{
			Message: diagnostics.Panic.Message,
			Stack:   diagnostics.Panic.Stack,
		}
	}
	return clone
}

func cloneRenderedPromptDiagnostic(diagnostic *RenderedPromptDiagnostic) *RenderedPromptDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &RenderedPromptDiagnostic{
		SystemPromptHash: diagnostic.SystemPromptHash,
		UserMessageHash:  diagnostic.UserMessageHash,
		Variables:        cloneStringMap(diagnostic.Variables),
	}
}

func cloneProviderDiagnostic(diagnostic *ProviderDiagnostic) *ProviderDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &ProviderDiagnostic{
		Provider:         diagnostic.Provider,
		Model:            diagnostic.Model,
		RequestMetadata:  cloneStringMap(diagnostic.RequestMetadata),
		ResponseMetadata: cloneStringMap(diagnostic.ResponseMetadata),
	}
}

func cloneCommandDiagnostic(diagnostic *CommandDiagnostic) *CommandDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &CommandDiagnostic{
		Command:    diagnostic.Command,
		Args:       cloneStringSlice(diagnostic.Args),
		Stdin:      diagnostic.Stdin,
		Env:        cloneStringMap(diagnostic.Env),
		Stdout:     diagnostic.Stdout,
		Stderr:     diagnostic.Stderr,
		ExitCode:   diagnostic.ExitCode,
		TimedOut:   diagnostic.TimedOut,
		Duration:   diagnostic.Duration,
		WorkingDir: diagnostic.WorkingDir,
	}
}
