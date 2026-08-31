package root_composition_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	activationConfigProviderAlias = "codex"
	activationConfigModel         = "operator-configured-model"
	activationOverrideModel       = "flag-override-model"
	activationUpdatedProvider     = "claude"
	activationUpdatedModel        = "operator-updated-model"
)

// TestOperatorConfigLoadAndDefaultsResolutionActivateThroughRootBuildProcessAfterLifecycle
// proves operator-config document load and defaults resolution activate through
// public Operator Settings surfaces after runtime lifecycle on a process composed
// only via the canonical process construction with edges.Edges effect replacement.
func TestOperatorConfigLoadAndDefaultsResolutionActivateThroughRootBuildProcessAfterLifecycle(t *testing.T) {
	t.Parallel()

	homeDir := writeOperatorConfigForActivation(t, activationConfigProviderAlias, activationConfigModel)
	fixture := ensureSharedOperatorSettingsFixture(t)
	dir := support.ScaffoldFactory(t, operatorConfigActivationFactoryConfig())
	support.WriteAgentConfig(t, dir, "worker-a", `---
type: MODEL_WORKER
stopToken: COMPLETE
---
Process the input task.
`)
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"operator settings defaults activation"}`))

	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		Stdout: support.CodexSuccessStdout("Done. COMPLETE"),
	})
	construction := fixture.constructionEffectSnapshot()
	if construction.fileSystemCalls != 0 {
		t.Fatalf("operator-config filesystem effect calls = %d during process construction, want 0", construction.fileSystemCalls)
	}
	if construction.createTemporaryCalls != 0 {
		t.Fatalf("operator-config CreateTemporaryFile calls = %d during process construction, want 0", construction.createTemporaryCalls)
	}
	beforeRead := fixture.router.readFileCalls.Load()
	fixture.withDefaultFactorySessionHandle(
		t,
		"operator config defaults activation",
		homeDir,
		dir,
		identityActivationGeneratedUUID,
		runner,
		[]string{"--model", activationOverrideModel},
		func(session *sharedOperatorSettingsSession) {
			support.WaitForTerminalStatus(t, session.baseURL, 15*time.Second)
			session.closeFactorySession(t)
			session.command.Stop(t)

			if got := fixture.router.readFileCalls.Load() - beforeRead; got == 0 {
				t.Fatalf("operator-config ReadFile calls after runtime lifecycle = %d, want > 0 via edges", got)
			}
			if runner.CallCount() != 1 {
				t.Fatalf("provider command runner calls = %d, want 1", runner.CallCount())
			}
			call := runner.LastRequest()
			if call.Command != string(modelprovider.ProviderCodex) {
				t.Fatalf("command = %q, want global default provider %q", call.Command, modelprovider.ProviderCodex)
			}
			support.AssertArgsContainSequence(t, call.Args, []string{"--model", activationOverrideModel})
		},
	)
}

// TestOperatorConfigDocumentUpdateActivatesThroughRootBuildProcessPublicCLISurface
// proves persisted provider-model updates activate through the public you init CLI
// surface on the same canonical process composition path with edges.Edges effect
// replacement after runtime lifecycle has started and stopped on that process.
func TestOperatorConfigDocumentUpdateActivatesThroughRootBuildProcessPublicCLISurface(t *testing.T) {
	t.Parallel()

	homeDir := writeOperatorConfigForActivation(t, activationConfigProviderAlias, activationConfigModel)
	fixture := ensureSharedOperatorSettingsFixture(t)
	dir := support.ScaffoldSingleStepFactory(t, "operator-config-update")
	fixture.withFactorySessionHandle(
		t,
		"operator config document update",
		homeDir,
		dir,
		identityActivationGeneratedUUID,
		nil,
		func(session *sharedOperatorSettingsSession) {
			support.WaitForStatus(t, session.baseURL, 5*time.Second, func(status factoryapi.StatusResponse) bool {
				return status.RuntimeStatus != ""
			})
			session.closeFactorySession(t)
			session.command.Stop(t)

			beforeUpdate := fixture.router.fileSystemCalls.Load()
			beforeTemporary := fixture.router.createTemporaryCalls.Load()
			var stdout bytes.Buffer
			initErr := fixture.process.Execute(root.Input{
				Args: []string{
					"you", "init",
					"--provider", activationUpdatedProvider,
					"--model", activationUpdatedModel,
				},
				Env: append(
					os.Environ(),
					"HOME="+homeDir,
					"USERPROFILE="+homeDir,
				),
				Stdin:            strings.NewReader(""),
				Stdout:           &stdout,
				Stderr:           io.Discard,
				Context:          t.Context(),
				WorkingDirectory: dir,
			})
			if initErr != nil {
				t.Fatalf("Process.Execute(you init) error = %v", initErr)
			}

			if got := fixture.router.fileSystemCalls.Load() - beforeUpdate; got == 0 {
				t.Fatalf("operator-config filesystem effect calls during init = %d, want > 0 via edges", got)
			}
			if got := fixture.router.createTemporaryCalls.Load() - beforeTemporary; got == 0 {
				t.Fatalf("operator-config CreateTemporaryFile calls during init = %d, want > 0 via edges", got)
			}

			configPath := operatorsettings.DefaultConfigPath(homeDir)
			payload, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatalf("read updated operator config: %v", err)
			}
			updated := string(payload)
			for _, want := range []string{
				`"workerModelProvider": "claude"`,
				`"workerModel": "operator-updated-model"`,
			} {
				if !strings.Contains(updated, want) {
					t.Fatalf("updated operator config omitted %q:\n%s", want, updated)
				}
			}
			if got := stdout.String(); !strings.Contains(got, "Configured default provider claude and model operator-updated-model") {
				t.Fatalf("stdout = %q, want documented configure success", got)
			}
		},
	)
}

func writeOperatorConfigForActivation(t *testing.T, providerAlias, model string) string {
	t.Helper()

	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".you-agent-factory")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("mkdir operator config directory: %v", err)
	}
	config := []byte(`{
  "defaults": {
    "workerModelProvider": "` + providerAlias + `",
    "workerModel": "` + model + `"
  }
}`)
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), config, 0o600); err != nil {
		t.Fatalf("write operator config: %v", err)
	}
	return homeDir
}

func operatorConfigActivationFactoryConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "worker-a"},
		},
		"workstations": []map[string]any{
			{
				"name":      "process",
				"worker":    "worker-a",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
		},
	}
}
