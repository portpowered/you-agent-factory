package root_composition_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestRecordedFactoryRedactsDeclaredSecretAtRecordingWriteBoundary(t *testing.T) {
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
	// declared secret. The typed assertion below proves the replacement shape.
	if bytes.Contains(data, []byte(secret)) {
		t.Fatalf("persisted recording contains the declared secret; artifact=%s", artifactPath)
	}
	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	if len(artifact.Events) == 0 {
		t.Fatal("persisted recording contains no Factory Events")
	}

	redactedRoles := make(map[string]bool)
	controlCount := 0
	for _, event := range artifact.Events {
		switch string(event.Type) {
		case "AGENT_RUN_RESPONSE":
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
	}
	for _, role := range []string{"system", "user"} {
		if !redactedRoles[role] {
			t.Fatalf("persisted agent-run %s transcript entry has no typed declared-secret redaction marker", role)
		}
	}
	if controlCount == 0 {
		t.Fatalf("persisted recording lost nonsecret control %q", control)
	}
	loadReplayArtifact := recordingswire.NewReplayArtifactLoader(
		platformreplay.Local{},
		factorydefinitionswire.FactorySnapshotJSONDecoder(),
	)
	if _, err := loadReplayArtifact(artifactPath); err != nil {
		t.Fatalf("load persisted replay artifact: %v", err)
	}

	replayInputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--replay", artifactPath, "--no-record", "--quiet",
	})
	replayInputs.Input.WorkingDirectory = dir
	replayInputs.Input.Env = append(replayInputs.Input.Env, "HOME="+homeDir, "USERPROFILE="+homeDir)
	replayProcess := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, replayProcess)
	if err := replayProcess.Execute(replayInputs.Input); err != nil {
		t.Fatalf("replay persisted recording: %v\nstderr=%s", err, replayInputs.Stderr())
	}
}
