package commandenv_test

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/commandenv"
)

func TestBuildEnforcesAutomationDefaultsAfterProviderOverrides(t *testing.T) {
	t.Setenv("GIT_EDITOR", "vim")
	env := commandenv.Build(os.Environ(), map[string]string{
		"GIT_EDITOR":               "nano",
		"GIT_TERMINAL_PROMPT":      "1",
		"AGENT_FACTORY_CUSTOM_ENV": "present",
	})

	assertEnvValue(t, env, "GIT_EDITOR", "true")
	assertEnvValue(t, env, "GIT_SEQUENCE_EDITOR", "true")
	assertEnvValue(t, env, "GIT_TERMINAL_PROMPT", "0")
	assertEnvValue(t, env, "AGENT_FACTORY_CUSTOM_ENV", "present")
	if len(commandenv.AutomationDefaults()) == 0 {
		t.Fatal("AutomationDefaults() returned no defaults")
	}
}

func TestBuildEnvironmentReservesWorkerSessionIdentity(t *testing.T) {
	t.Parallel()
	for _, supplied := range []bool{false, true} {
		t.Run(map[bool]string{false: "unattributed", true: "supervised"}[supplied], func(t *testing.T) {
			t.Parallel()
			vars := map[string]string{"CUSTOM": "provider", "EDITOR": "interactive"}
			base := []string{"CUSTOM=process"}
			keys := []string{
				"YOU_SERVER", "YOU_WORKER_SESSION_ID", "YOU_WORKER_SESSION_TOKEN",
				"YOU_MESSAGE_TARGET", "YOU_MESSAGE_TARGET_WORK_ID", "YOU_WORK_ID", "YOU_FACTORY_SESSION_ID",
			}
			for _, key := range keys {
				vars[key] = "authored"
				vars[strings.ToLower(key)] = "authored"
				if supplied {
					base = append(base, key+"=supervisor")
				}
			}
			before := append([]string(nil), base...)
			env := commandenv.Build(base, vars)
			assertEnvValue(t, env, "CUSTOM", "provider")
			assertEnvValue(t, env, "EDITOR", "true")
			for _, entry := range env {
				name, value, _ := strings.Cut(entry, "=")
				for _, key := range keys {
					if strings.EqualFold(name, key) && (!supplied || name != key || value != "supervisor") {
						t.Fatalf("reserved environment key %s did not retain supervisor authority", name)
					}
				}
			}
			if supplied {
				for _, key := range keys {
					assertEnvValue(t, env, key, "supervisor")
				}
			}
			if !reflect.DeepEqual(base, before) || vars["YOU_WORKER_SESSION_TOKEN"] != "authored" {
				t.Fatal("Build mutated caller-owned environment")
			}
		})
	}
}

func assertEnvValue(t *testing.T, env []string, name, want string) {
	t.Helper()
	prefix := name + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			if got := strings.TrimPrefix(entry, prefix); got != want {
				t.Fatalf("%s = %q, want %q", name, got, want)
			}
			return
		}
	}
	t.Fatalf("environment does not contain %s", name)
}
