package commandenv_test

import (
	"os"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers/provider/commandenv"
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
