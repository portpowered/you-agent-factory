package workers

func cloneSafeDiagnostics(diagnostics *SafeDiagnostics) *SafeDiagnostics {
	if diagnostics == nil {
		return nil
	}
	clone := &SafeDiagnostics{
		RenderedPrompt: cloneSafeRenderedPromptDiagnostic(diagnostics.RenderedPrompt),
		Provider:       cloneSafeProviderDiagnostic(diagnostics.Provider),
		AgentRun:       cloneSafeAgentRunDiagnostic(diagnostics.AgentRun),
		Invocation:     CloneInvocationDiagnostic(diagnostics.Invocation),
		Metadata:       cloneStringMap(diagnostics.Metadata),
	}
	if diagnostics.Command != nil {
		clone.Command = &SafeCommandDiagnostic{
			Command:    diagnostics.Command.Command,
			Args:       append([]string(nil), diagnostics.Command.Args...),
			Stdout:     diagnostics.Command.Stdout,
			Stderr:     diagnostics.Command.Stderr,
			ExitCode:   diagnostics.Command.ExitCode,
			TimedOut:   diagnostics.Command.TimedOut,
			Duration:   diagnostics.Command.Duration,
			WorkingDir: diagnostics.Command.WorkingDir,
		}
	}
	if diagnostics.Panic != nil {
		clone.Panic = &PanicDiagnostic{
			Message: diagnostics.Panic.Message,
			Stack:   diagnostics.Panic.Stack,
		}
	}
	return clone
}

func cloneSafeRenderedPromptDiagnostic(
	diagnostic *SafeRenderedPromptDiagnostic,
) *SafeRenderedPromptDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &SafeRenderedPromptDiagnostic{
		SystemPromptHash: diagnostic.SystemPromptHash,
		UserMessageHash:  diagnostic.UserMessageHash,
		Variables:        cloneStringMap(diagnostic.Variables),
	}
}

func cloneSafeProviderDiagnostic(diagnostic *SafeProviderDiagnostic) *SafeProviderDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return &SafeProviderDiagnostic{
		Provider:         diagnostic.Provider,
		Model:            diagnostic.Model,
		RequestMetadata:  cloneStringMap(diagnostic.RequestMetadata),
		ResponseMetadata: cloneStringMap(diagnostic.ResponseMetadata),
	}
}

func cloneSafeAgentRunDiagnostic(diagnostic *SafeAgentRunDiagnostic) *SafeAgentRunDiagnostic {
	if diagnostic == nil {
		return nil
	}
	clone := &SafeAgentRunDiagnostic{
		ExecutionBehavior: diagnostic.ExecutionBehavior,
		FailureClass:      diagnostic.FailureClass,
		RecoveryAction:    diagnostic.RecoveryAction,
		ToolPolicy:        diagnostic.ToolPolicy,
		ToolCallCount:     diagnostic.ToolCallCount,
	}
	clone.ToolDiagnostics = append([]AgentRunToolDiagnostic(nil), diagnostic.ToolDiagnostics...)
	clone.Transcript = append([]AgentRunTranscriptEntry(nil), diagnostic.Transcript...)
	return clone
}
