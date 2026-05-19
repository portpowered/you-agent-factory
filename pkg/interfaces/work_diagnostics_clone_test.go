package interfaces

import "testing"

func TestCloneWorkDiagnostics_Nil(t *testing.T) {
	if got := CloneWorkDiagnostics(nil); got != nil {
		t.Fatalf("CloneWorkDiagnostics(nil) = %#v, want nil", got)
	}
}

func TestCloneWorkDiagnostics_PreservesAbsentNestedValues(t *testing.T) {
	source := &WorkDiagnostics{
		RenderedPrompt: &RenderedPromptDiagnostic{},
		Provider:       &ProviderDiagnostic{},
		Command:        &CommandDiagnostic{},
		Panic:          &PanicDiagnostic{},
	}

	clone := CloneWorkDiagnostics(source)

	if clone == nil {
		t.Fatalf("CloneWorkDiagnostics(source) = nil, want clone")
	}
	if clone.RenderedPrompt == nil || clone.RenderedPrompt.Variables != nil {
		t.Fatalf("clone rendered prompt = %#v, want nil variables", clone.RenderedPrompt)
	}
	if clone.Provider == nil || clone.Provider.RequestMetadata != nil || clone.Provider.ResponseMetadata != nil {
		t.Fatalf("clone provider = %#v, want nil metadata maps", clone.Provider)
	}
	if clone.Command == nil || clone.Command.Args != nil || clone.Command.Env != nil {
		t.Fatalf("clone command = %#v, want nil args and env", clone.Command)
	}
	if clone.Metadata != nil {
		t.Fatalf("clone metadata = %#v, want nil", clone.Metadata)
	}
}

func TestCloneWorkDiagnostics_DetachesNestedMutableState(t *testing.T) {
	source := &WorkDiagnostics{
		RenderedPrompt: &RenderedPromptDiagnostic{
			SystemPromptHash: "system-hash",
			UserMessageHash:  "user-hash",
			Variables: map[string]string{
				"prompt_source": "factory",
			},
		},
		Provider: &ProviderDiagnostic{
			Provider: "openai",
			Model:    "gpt-5.4",
			RequestMetadata: map[string]string{
				"session_id": "sess-1",
			},
			ResponseMetadata: map[string]string{
				"retry_count": "0",
			},
		},
		Command: &CommandDiagnostic{
			Command: "python",
			Args:    []string{"run.py", "--verbose"},
			Env: map[string]string{
				"MODE": "test",
			},
			Stdout:     "stdout",
			Stderr:     "stderr",
			ExitCode:   1,
			TimedOut:   true,
			WorkingDir: "/tmp/work",
		},
		Panic: &PanicDiagnostic{
			Message: "boom",
			Stack:   "stack",
		},
		Metadata: map[string]string{
			"worktree": "alpha",
		},
	}

	clone := CloneWorkDiagnostics(source)

	source.RenderedPrompt.Variables["prompt_source"] = "mutated"
	source.Provider.RequestMetadata["session_id"] = "mutated"
	source.Provider.ResponseMetadata["retry_count"] = "9"
	source.Command.Args[0] = "mutated.py"
	source.Command.Env["MODE"] = "prod"
	source.Panic.Message = "changed"
	source.Panic.Stack = "changed-stack"
	source.Metadata["worktree"] = "beta"

	if got := clone.RenderedPrompt.Variables["prompt_source"]; got != "factory" {
		t.Fatalf("clone rendered prompt variable = %q, want %q", got, "factory")
	}
	if got := clone.Provider.RequestMetadata["session_id"]; got != "sess-1" {
		t.Fatalf("clone provider request metadata = %q, want %q", got, "sess-1")
	}
	if got := clone.Provider.ResponseMetadata["retry_count"]; got != "0" {
		t.Fatalf("clone provider response metadata = %q, want %q", got, "0")
	}
	if got := clone.Command.Args[0]; got != "run.py" {
		t.Fatalf("clone command arg = %q, want %q", got, "run.py")
	}
	if got := clone.Command.Env["MODE"]; got != "test" {
		t.Fatalf("clone command env = %q, want %q", got, "test")
	}
	if got := clone.Panic.Message; got != "boom" {
		t.Fatalf("clone panic message = %q, want %q", got, "boom")
	}
	if got := clone.Panic.Stack; got != "stack" {
		t.Fatalf("clone panic stack = %q, want %q", got, "stack")
	}
	if got := clone.Metadata["worktree"]; got != "alpha" {
		t.Fatalf("clone metadata = %q, want %q", got, "alpha")
	}
}
