package root_composition_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestRecordedFactoryRedactsDeclaredSecretAtRecordingWriteBoundary proves declared secrets are redacted before recording persistence.
func TestRecordedFactoryRedactsDeclaredSecretAtRecordingWriteBoundary(t *testing.T) {
	t.Parallel()
	acquireRootCompositionFixtureSlot(t)

	secret := "story003-declared-secret-9e5c2a7f"
	control := "story003-visible-control"
	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "recordings-secret-redaction-guard",
		"invocationSignature": map[string]any{
			"parameters": []any{
				map[string]any{
					"name":      "secret",
					"required":  true,
					"sensitive": true,
					"bindings":  []any{map[string]any{"kind": "NAMED"}},
				},
			},
		},
		"workTypes": []any{map[string]any{
			"name":             "task",
			"handlingBehavior": []string{"DEFAULT"},
			"states": []any{
				map[string]any{"name": "init", "type": "INITIAL"},
				map[string]any{"name": "complete", "type": "TERMINAL"},
				map[string]any{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []any{map[string]any{
			"name":             "model-worker",
			"type":             "MODEL_WORKER",
			"modelProvider":    "CODEX",
			"executorProvider": "SCRIPT_WRAP",
			"model":            "gpt-5-codex",
		}},
		"workstations": []map[string]any{{
			"name":      "process-task",
			"worker":    "model-worker",
			"inputs":    []any{map[string]any{"workType": "task", "state": "init"}},
			"outputs":   []any{map[string]any{"workType": "task", "state": "complete"}},
			"onFailure": []any{map[string]any{"workType": "task", "state": "failed"}},
		}},
	})
	support.WriteWorkstationConfig(t, dir, "process-task", "---\ntype: MODEL_WORKSTATION\n---\ncontrol=story003-visible-control secret=${secret}\n")
	artifactPath := filepath.Join(t.TempDir(), "recordings-secret-redaction-guard.replay.json")
	homeDir := t.TempDir()
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run",
		"--factory", filepath.Join(dir, "factory.json"),
		"--record", artifactPath,
		"--quiet",
		"--secret", secret,
	})
	inputs.Input.WorkingDirectory = dir
	inputs.Input.Env = append(inputs.Input.Env, "HOME="+homeDir, "USERPROFILE="+homeDir)
	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner: support.NewStaticSuccessCommandRunner(control),
	})
	support.CleanupProcess(t, process)
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("recording Factory run: %v\nstderr=%s", err, inputs.Stderr())
	}

	data, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("read persisted recording: %v", err)
	}
	// Keep this guard as the single complete-artifact literal search for the
	// declared secret. The transcript assertion below proves the safe shape.
	if bytes.Contains(data, []byte(secret)) {
		t.Fatalf("persisted recording contains the declared secret; artifact=%s", artifactPath)
	}
	assertRecordedSecretArtifact(t, artifactPath, control)
	replayInputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--replay", artifactPath, "--no-record", "--quiet",
	})
	replayInputs.Input.WorkingDirectory = dir
	replayInputs.Input.Env = append(replayInputs.Input.Env, "HOME="+homeDir, "USERPROFILE="+homeDir)
	if err := process.Execute(replayInputs.Input); err != nil {
		t.Fatalf("replay persisted recording: %v\nstderr=%s", err, replayInputs.Stderr())
	}
}

func assertRecordedSecretArtifact(t *testing.T, artifactPath, control string) {
	t.Helper()
	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	if len(artifact.Events) == 0 {
		t.Fatal("persisted recording contains no Factory Events")
	}
	redactedRoles := make(map[string]bool)
	controlCount := 0
	for _, event := range artifact.Events {
		if string(event.Type) != "AGENT_RUN_RESPONSE" {
			continue
		}
		var payload struct {
			Diagnostics struct {
				AgentRun struct {
					Transcript []struct {
						Role    string          `json:"role"`
						Summary json.RawMessage `json:"summary"`
					} `json:"transcript"`
				} `json:"agentRun"`
			} `json:"diagnostics"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode persisted agent-run response: %v", err)
		}
		for _, entry := range payload.Diagnostics.AgentRun.Transcript {
			switch entry.Role {
			case "system", "user":
				var summary string
				if err := json.Unmarshal(entry.Summary, &summary); err == nil {
					if summary != "control="+control+" secret=<redacted>" {
						t.Fatalf("persisted %s prompt = %q, want adjacent control with only secret redacted", entry.Role, summary)
					}
					redactedRoles[entry.Role] = true
					break
				}
				var marker recordings.RecordingRedactedValue
				if err := json.Unmarshal(entry.Summary, &marker); err != nil {
					t.Fatalf("decode persisted %s redaction marker: %v", entry.Role, err)
				}
				if err := marker.Validate(); err != nil {
					t.Fatalf("persisted %s redaction marker: %v", entry.Role, err)
				}
				redactedRoles[entry.Role] = true
			case "assistant":
				var summary string
				if err := json.Unmarshal(entry.Summary, &summary); err != nil {
					t.Fatalf("decode persisted assistant summary: %v", err)
				}
				if summary == control {
					controlCount++
				}
			}
		}
	}
	for _, role := range []string{"system", "user"} {
		if !redactedRoles[role] {
			t.Fatalf("persisted agent-run %s transcript entry has no declared-secret redaction", role)
		}
	}
	if controlCount == 0 {
		t.Fatalf("persisted recording lost nonsecret control %q", control)
	}
}

// TestRecordedFactoryRedactsSecretStepsAcrossLifecycle proves that recording
// and replaying a two-workstation Factory run redacts a declared secret for
// both file-backed and inline workstation definitions while preserving visible
// controls, outputs, event lineage, cleanup, and replay.
func TestRecordedFactoryRedactsSecretStepsAcrossLifecycle(t *testing.T) {
	t.Parallel()
	acquireRootCompositionFixtureSlot(t)

	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner: support.NewStaticSuccessCommandRunner("story003-two-step-output"),
	})
	support.CleanupProcess(t, process)
	t.Run("secret step", func(t *testing.T) {
		runRecordedTwoWorkstationLifecycle(t, process, false)
	})
	t.Run("inline secret step", func(t *testing.T) {
		runRecordedTwoWorkstationLifecycle(t, process, true)
	})
}

func runRecordedTwoWorkstationLifecycle(t *testing.T, process support.Process, inline bool) {
	secret := "sk-fake-story003-lifecycle-secret-4c9e2a7f"
	secretControl := "secret-step-visible-control"
	plainControl := "plain-step-visible-control"
	output := "story003-two-step-output"
	dir := scaffoldRecordedTwoWorkstationFactory(t, inline, secretControl, plainControl)
	if !inline {
		support.WriteWorkstationConfig(t, dir, "secret-step", "---\ntype: MODEL_WORKSTATION\n---\ncontrol="+secretControl+" secret=${secret}\n")
		support.WriteWorkstationConfig(t, dir, "plain-step", "---\ntype: MODEL_WORKSTATION\n---\ncontrol="+plainControl+"\n")
	}

	artifactPath := filepath.Join(t.TempDir(), "recordings-secret-redaction-two-step.replay.json")
	homeDir := t.TempDir()
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run",
		"--factory", filepath.Join(dir, "factory.json"),
		"--record", artifactPath,
		"--quiet",
		"--secret", secret,
	})
	inputs.Input.WorkingDirectory = dir
	inputs.Input.Env = append(inputs.Input.Env, "HOME="+homeDir, "USERPROFILE="+homeDir)
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("recording two-workstation Factory run: %v\nstderr=%s", err, inputs.Stderr())
	}

	data, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("read persisted two-workstation recording: %v", err)
	}
	if bytes.Contains(data, []byte(secret)) {
		t.Fatalf("persisted two-workstation recording contains the declared secret; artifact=%s", artifactPath)
	}
	for _, visible := range []string{secretControl, plainControl} {
		if !bytes.Contains(data, []byte(visible)) {
			t.Fatalf("persisted two-workstation recording lost visible control %q", visible)
		}
	}
	assertRecordedTwoWorkstationArtifact(t, artifactPath, secretControl, plainControl, output)

	replayFunctionalRecording(t, process, artifactPath, dir, homeDir)
}

func scaffoldRecordedTwoWorkstationFactory(t *testing.T, inline bool, secretControl, plainControl string) string {
	workstations := []map[string]any{
		{
			"name":      "secret-step",
			"worker":    "secret-worker",
			"inputs":    []any{map[string]any{"workType": "task", "state": "init"}},
			"outputs":   []any{map[string]any{"workType": "task", "state": "processing"}},
			"onFailure": []any{map[string]any{"workType": "task", "state": "failed"}},
		},
		{
			"name":      "plain-step",
			"worker":    "plain-worker",
			"inputs":    []any{map[string]any{"workType": "task", "state": "processing"}},
			"outputs":   []any{map[string]any{"workType": "task", "state": "complete"}},
			"onFailure": []any{map[string]any{"workType": "task", "state": "failed"}},
		},
	}
	var factoryWorkstations any = workstations
	if inline {
		workstations[0]["body"] = "control=" + secretControl + " secret=${secret}"
		workstations[1]["body"] = "control=" + plainControl
		inlineWorkstations := make([]any, len(workstations))
		for index, workstation := range workstations {
			inlineWorkstations[index] = workstation
		}
		factoryWorkstations = inlineWorkstations
	}
	return support.ScaffoldFactory(t, map[string]any{
		"name": "recordings-secret-redaction-two-step",
		"invocationSignature": map[string]any{
			"parameters": []any{
				map[string]any{
					"name":      "secret",
					"required":  true,
					"sensitive": true,
					"bindings":  []any{map[string]any{"kind": "NAMED"}},
				},
			},
		},
		"workTypes": []any{map[string]any{
			"name":             "task",
			"handlingBehavior": []string{"DEFAULT"},
			"states": []any{
				map[string]any{"name": "init", "type": "INITIAL"},
				map[string]any{"name": "processing"},
				map[string]any{"name": "complete", "type": "TERMINAL"},
				map[string]any{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []any{
			map[string]any{
				"name":             "secret-worker",
				"type":             "MODEL_WORKER",
				"modelProvider":    "CODEX",
				"executorProvider": "SCRIPT_WRAP",
				"model":            "gpt-5-codex",
			},
			map[string]any{
				"name":             "plain-worker",
				"type":             "MODEL_WORKER",
				"modelProvider":    "CODEX",
				"executorProvider": "SCRIPT_WRAP",
				"model":            "gpt-5-codex",
			},
		},
		"workstations": factoryWorkstations,
	})
}

func replayFunctionalRecording(t *testing.T, process support.Process, artifactPath, dir, homeDir string) {
	t.Helper()
	replayInputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--replay", artifactPath, "--no-record", "--quiet",
	})
	replayInputs.Input.WorkingDirectory = dir
	replayInputs.Input.Env = append(replayInputs.Input.Env, "HOME="+homeDir, "USERPROFILE="+homeDir)
	if err := process.Execute(replayInputs.Input); err != nil {
		t.Fatalf("replay two-workstation recording: %v\nstderr=%s", err, replayInputs.Stderr())
	}
}

func assertRecordedTwoWorkstationArtifact(
	t *testing.T,
	artifactPath string,
	secretControl string,
	plainControl string,
	output string,
) {
	t.Helper()
	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	if len(artifact.Events) == 0 {
		t.Fatal("persisted two-workstation recording contains no Factory Events")
	}

	type dispatchRequestPayload struct {
		TransitionID string `json:"transitionId"`
	}
	type transcriptEntry struct {
		Role    string          `json:"role"`
		Summary json.RawMessage `json:"summary"`
	}
	type agentRunPayload struct {
		Diagnostics struct {
			AgentRun struct {
				Transcript []transcriptEntry `json:"transcript"`
			} `json:"agentRun"`
		} `json:"diagnostics"`
	}
	dispatches := make(map[string]string)
	for _, event := range artifact.Events {
		if string(event.Type) != "DISPATCH_REQUEST" || event.Context.DispatchID == nil {
			continue
		}
		var payload dispatchRequestPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode two-workstation DISPATCH_REQUEST: %v", err)
		}
		dispatches[*event.Context.DispatchID] = payload.TransitionID
	}
	seen := make(map[string]bool)
	for _, event := range artifact.Events {
		if string(event.Type) != "AGENT_RUN_RESPONSE" || event.Context.DispatchID == nil {
			continue
		}
		transition := dispatches[*event.Context.DispatchID]
		if transition != "secret-step" && transition != "plain-step" {
			continue
		}
		var payload agentRunPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode two-workstation AGENT_RUN_RESPONSE: %v", err)
		}
		wantPrompt := "control=" + plainControl
		if transition == "secret-step" {
			wantPrompt = "control=" + secretControl + " secret=<redacted>"
		}
		roles := make(map[string]bool)
		for _, entry := range payload.Diagnostics.AgentRun.Transcript {
			roles[entry.Role] = true
			switch entry.Role {
			case "system", "user":
				var summary string
				if err := json.Unmarshal(entry.Summary, &summary); err != nil {
					t.Fatalf("decode %s %s prompt: %v", transition, entry.Role, err)
				}
				if summary != wantPrompt {
					t.Fatalf("persisted %s %s prompt = %q, want %q", transition, entry.Role, summary, wantPrompt)
				}
			case "assistant":
				var summary string
				if err := json.Unmarshal(entry.Summary, &summary); err != nil {
					t.Fatalf("decode %s assistant transcript: %v", transition, err)
				}
				if summary != output {
					t.Fatalf("persisted %s assistant transcript = %q, want %q", transition, summary, output)
				}
			}
		}
		for _, role := range []string{"system", "user", "assistant"} {
			if !roles[role] {
				t.Fatalf("persisted %s transcript has no %s entry", transition, role)
			}
		}
		seen[transition] = true
	}
	for _, transition := range []string{"secret-step", "plain-step"} {
		if !seen[transition] {
			t.Fatalf("persisted recording has no AGENT_RUN_RESPONSE for %s; dispatches=%#v", transition, dispatches)
		}
	}
}
