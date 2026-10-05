package customer_lifecycles_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Document-only commands own no Factory Session and need no Current Factory lease.
func (fixture *sharedOperatorSettingsFixture) withOperatorSettingsDocumentRoute(
	t *testing.T, label, homeDir, workingDir, generatedID string,
	runner platformprocess.CommandRunner, run func(*operatorSettingsEffectRoute),
) {
	t.Helper()
	route, err := fixture.router.register(label, homeDir, workingDir, generatedID, runner)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := fixture.router.unregister(route); err != nil {
			t.Error(err)
		}
	})
	run(route)
}

func settingsCLIInput(t *testing.T, home, dir string, args ...string) root.Input {
	t.Helper()
	// Remove inherited settings and all Windows home selectors before supplying
	// the scenario's profile; no command may discover the operator's real home.
	var env []string
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		switch strings.ToUpper(key) {
		case "HOME", "USERPROFILE", "HOMEDRIVE", "HOMEPATH", operatorsettings.EnvDefaultWorkerModel, operatorsettings.EnvDefaultWorkerModelProvider:
			continue
		}
		env = append(env, item)
	}
	env = append(env, "HOME="+home, "USERPROFILE="+home,
		"HOMEDRIVE="+filepath.VolumeName(home), "HOMEPATH="+strings.TrimPrefix(home, filepath.VolumeName(home)))
	return root.Input{Args: append([]string{"you"}, args...), Env: env,
		WorkingDirectory: dir, Context: t.Context(), Stdin: strings.NewReader("")}
}

func testOperatorsettingsconfigurationCLISettingsUnknownFieldsPrecedenceAndSafeFailure(t *testing.T) {
	t.Parallel()
	fixture := ensureSharedOperatorSettingsFixture(t)
	t.Run("F12-R1 unknown fields round trip", func(t *testing.T) { checkSettingsUnknownFieldRoundTrip(t, fixture) })
	t.Run("F12-E1 malformed prerequisite", func(t *testing.T) { checkMalformedSettingsPrerequisite(t, fixture) })
	t.Run("F12-E2 persistence unavailable", func(t *testing.T) { checkSettingsPersistenceFailure(t, fixture) })
	t.Run("F12-B2 independent profiles", func(t *testing.T) { checkIndependentSettingsProfiles(t, fixture) })
	t.Run("runtime precedence", func(t *testing.T) { checkSettingsRuntimePrecedence(t, fixture) })
}

func checkMalformedSettingsPrerequisite(t *testing.T, fixture *sharedOperatorSettingsFixture) {
	t.Helper()
	t.Parallel()
	home := writeOperatorConfigForActivation(t, "codex", "before")
	path := filepath.Join(home, ".you-agent-factory", "config.json")
	original := []byte(`{"defaults":{"workerModelProvider":"codex","workerModel":"before"}} {}`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	fixture.withOperatorSettingsDocumentRoute(t, t.Name(), home, home, identityActivationGeneratedUUID, nil, func(*operatorSettingsEffectRoute) {
		input := settingsCLIInput(t, home, home, "init", "--provider", "claude", "--model", "after")
		var stdout, stderr bytes.Buffer
		input.Stdout, input.Stderr = &stdout, &stderr
		err := fixture.process.Execute(input)
		var failure operatorsettings.DocumentFailure
		if !errors.As(err, &failure) || failure.Kind != operatorsettings.DocumentFailureKindMalformed {
			t.Fatalf("init error = %v, want malformed document failure", err)
		}
		after, readErr := os.ReadFile(path)
		if readErr != nil || !bytes.Equal(original, after) || strings.Contains(stdout.String(), "Configured default provider") {
			t.Fatalf("malformed prerequisite changed destination or reported success: %v\n%s\n%s", readErr, after, &stdout)
		}
	})
}

func checkSettingsUnknownFieldRoundTrip(t *testing.T, fixture *sharedOperatorSettingsFixture) {
	t.Helper()
	t.Parallel()
	home := writeOperatorConfigForActivation(t, "codex", "before")
	path := filepath.Join(home, ".you-agent-factory", "config.json")
	initial := []byte(`{"backendScopeID":"local-11111111-1111-4111-8111-111111111111","defaults":{"workerModelProvider":"codex","workerModel":"before","futureDefault":{"enabled":true}},"futureRoot":{"secret":"preserve"},"models":{"custom":{"source":"hf://owner/repo","backend":"backend","loadPolicy":"ON_DEMAND","operations":["OMNI"],"futureModel":{"enabled":true}}},"workerPresets":[{"id":"research","modelProvider":"CODEX","model":"gpt-5","futurePreset":"kept"}]}`)
	if err := os.WriteFile(path, initial, 0600); err != nil {
		t.Fatal(err)
	}
	fixture.withOperatorSettingsDocumentRoute(t, t.Name(), home, home, identityActivationGeneratedUUID, nil, func(*operatorSettingsEffectRoute) {
		input := settingsCLIInput(t, home, home, "init", "--provider", "claude", "--model", "after", "--verbose")
		var stdout, stderr bytes.Buffer
		input.Stdout, input.Stderr = &stdout, &stderr
		if err := fixture.process.Execute(input); err != nil {
			t.Fatalf("init: %v\n%s\n%s", err, &stdout, &stderr)
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var document map[string]any
		if err := json.Unmarshal(payload, &document); err != nil {
			t.Fatal(err)
		}
		defaults := document["defaults"].(map[string]any)
		wantPresets := []any{map[string]any{"id": "research", "modelProvider": "CODEX", "model": "gpt-5", "futurePreset": "kept"}}
		wantModels := map[string]any{"custom": map[string]any{"source": "hf://owner/repo", "backend": "backend", "loadPolicy": "ON_DEMAND", "operations": []any{"OMNI"}, "futureModel": map[string]any{"enabled": true}}}
		if !reflect.DeepEqual(document["models"], wantModels) {
			t.Fatalf("rewritten settings lost model map fields: %s", payload)
		}
		if !reflect.DeepEqual(document["futureRoot"], map[string]any{"secret": "preserve"}) || !reflect.DeepEqual(defaults["futureDefault"], map[string]any{"enabled": true}) || defaults["workerModel"] != "after" || defaults["workerModelProvider"] != "claude" || document["backendScopeID"] != "local-11111111-1111-4111-8111-111111111111" || !reflect.DeepEqual(document["workerPresets"], wantPresets) {
			t.Fatalf("rewritten settings lost fields: %s", payload)
		}
		if !strings.Contains(stdout.String(), "Configured default provider claude and model after") {
			t.Fatalf("success framing: %s", &stdout)
		}
	})
}

func checkSettingsPersistenceFailure(t *testing.T, fixture *sharedOperatorSettingsFixture) {
	t.Helper()
	t.Parallel()
	home := writeOperatorConfigForActivation(t, "codex", "before")
	path := filepath.Join(home, ".you-agent-factory", "config.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	fixture.withOperatorSettingsDocumentRoute(t, t.Name(), home, home, identityActivationGeneratedUUID, nil, func(route *operatorSettingsEffectRoute) {
		fault := errors.New("F12 persistence unavailable")
		route.renameError = fault
		input := settingsCLIInput(t, home, home, "init", "--provider", "claude", "--model", "after")
		var stdout, stderr bytes.Buffer
		input.Stdout, input.Stderr = &stdout, &stderr
		if err := fixture.process.Execute(input); !errors.Is(err, fault) {
			t.Fatalf("init error = %v, want injected persistence failure", err)
		}
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(before, after) || strings.Contains(stdout.String(), "Configured default provider") {
			t.Fatalf("failed init changed destination or reported success: %v\n%s\n%s", err, after, &stdout)
		}
	})
}

func checkIndependentSettingsProfiles(t *testing.T, fixture *sharedOperatorSettingsFixture) {
	t.Helper()
	t.Parallel()
	for _, model := range []string{"isolated-alpha", "isolated-beta"} {
		t.Run(model, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			fixture.withOperatorSettingsDocumentRoute(t, t.Name(), home, home, identityActivationGeneratedUUID, nil, func(*operatorSettingsEffectRoute) {
				input := settingsCLIInput(t, home, home, "init", "--provider", "codex", "--model", model)
				var stdout, stderr bytes.Buffer
				input.Stdout, input.Stderr = &stdout, &stderr
				if err := fixture.process.Execute(input); err != nil {
					t.Fatalf("isolated init: %v", err)
				}
				payload, err := os.ReadFile(filepath.Join(home, ".you-agent-factory", "config.json"))
				if err != nil {
					t.Fatal(err)
				}
				var document struct {
					Defaults struct {
						Model string `json:"workerModel"`
					} `json:"defaults"`
				}
				if err := json.Unmarshal(payload, &document); err != nil {
					t.Fatal(err)
				}
				if document.Defaults.Model != model || !strings.Contains(stdout.String(), "Configured default provider codex and model "+model) {
					t.Fatalf("profile contamination: model=%q stdout=%s", document.Defaults.Model, &stdout)
				}
			})
		})
	}
}

// Local run owns Current Factory/~default; only the runtime cohort takes a lease.
func checkSettingsRuntimePrecedence(t *testing.T, fixture *sharedOperatorSettingsFixture) {
	t.Helper()
	for _, tc := range []struct {
		id, envProvider, envModel, flagProvider, flagModel, command, model string
	}{
		{"F12-R2", "", "", "", "", "codex", activationConfigModel},
		{"F12-R3", "claude", "environment-model", "", "", "claude", "environment-model"},
		{"F12-R4", "claude", "environment-model", "codex", "explicit-model", "codex", "explicit-model"},
		{"F12-B1", "", "", "", "", "codex", ""},
		{"F12-E1", "", "", "", "", "", ""},
	} {
		t.Run(tc.id, func(t *testing.T) {
			home := writeOperatorConfigForActivation(t, "codex", activationConfigModel)
			dir := support.ScaffoldFactory(t, operatorConfigActivationFactoryConfig())
			support.WriteAgentConfig(t, dir, "worker-a", "---\ntype: MODEL_WORKER\nstopToken: COMPLETE\n---\nProcess the input task.\n")
			testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"settings precedence"}`))
			runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("Done. COMPLETE")})
			fixture.withOperatorSettingsRoute(t, t.Name(), home, dir, identityActivationGeneratedUUID, runner, func(route *operatorSettingsEffectRoute) {
				// Bootstrap each mutable profile through the public process before runtime.
				bootstrap := settingsCLIInput(t, home, dir, "init", "--provider", "codex", "--model", activationConfigModel)
				var bootOutput bytes.Buffer
				bootstrap.Stdout, bootstrap.Stderr = &bootOutput, &bootOutput
				if err := fixture.process.Execute(bootstrap); err != nil {
					t.Fatalf("bootstrap: %v", err)
				}
				runOperatorSettingsLifecycleInitialization(t, fixture.process, home)
				path := filepath.Join(home, ".you-agent-factory", "config.json")
				if tc.id == "F12-B1" {
					if err := os.WriteFile(path, []byte(`{"backendScopeID":"local-11111111-1111-4111-8111-111111111111"}`), 0600); err != nil {
						t.Fatal(err)
					}
				}
				before, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				fault := errors.New("F12 settings read unavailable")
				if tc.id == "F12-E1" {
					route.readError = fault
				}

				args := []string{"run", "--dir", dir, "--quiet", "--no-record"}
				if tc.flagProvider != "" {
					args = append(args, "--provider", tc.flagProvider, "--model", tc.flagModel)
				}
				input := settingsCLIInput(t, home, dir, args...)
				input.Env = append(input.Env, operatorsettings.EnvDefaultWorkerModelProvider+"="+tc.envProvider, operatorsettings.EnvDefaultWorkerModel+"="+tc.envModel)
				var stdout, stderr bytes.Buffer
				input.Stdout, input.Stderr = &stdout, &stderr
				if err := fixture.process.Execute(input); tc.id == "F12-E1" {
					after, readErr := os.ReadFile(path)
					if !errors.Is(err, fault) || runner.CallCount() != 0 || readErr != nil || !bytes.Equal(before, after) {
						t.Fatalf("unavailable settings: error=%v calls=%d read=%v changed=%v", err, runner.CallCount(), readErr, !bytes.Equal(before, after))
					}
					return
				} else if err != nil {
					t.Fatalf("run: %v\n%s\n%s", err, &stdout, &stderr)
				}
				checkSettingsResolvedDispatch(t, runner, path, tc.id, tc.command, tc.model)
			})
		})
	}
}

func checkSettingsResolvedDispatch(t *testing.T, runner *support.ShapedProviderCommandRunner, path, caseID, command, model string) {
	t.Helper()
	if runner.CallCount() != 1 || runner.LastRequest().Command != command {
		t.Fatalf("provider requests: %d, last=%#v", runner.CallCount(), runner.LastRequest())
	}
	if model != "" {
		support.AssertArgsContainSequence(t, runner.LastRequest().Args, []string{"--model", model})
	} else {
		for _, arg := range runner.LastRequest().Args {
			if arg == "--model" {
				t.Fatalf("missing default supplied a model: %#v", runner.LastRequest().Args)
			}
		}
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Runtime initialization may seed profile fields and normalize provider
	// casing. The contract forbids persisting invocation overrides, so
	// compare the stored defaults below rather than incidental formatting.
	var persisted struct {
		Defaults struct {
			Provider string `json:"workerModelProvider"`
			Model    string `json:"workerModel"`
		} `json:"defaults"`
	}
	if err := json.Unmarshal(payload, &persisted); err != nil {
		t.Fatal(err)
	}
	wantModel := activationConfigModel
	if caseID == "F12-B1" {
		wantModel = ""
	}
	if persisted.Defaults.Model != wantModel || (caseID != "F12-B1" && !strings.EqualFold(persisted.Defaults.Provider, "codex")) {
		t.Fatalf("invocation persisted an override: %s", payload)
	}
}
