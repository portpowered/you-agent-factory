package script

import (
	"testing"
)

func TestMergedEnvironmentPreservesFullProcessEnvironmentWithoutDeclaredOverrides(t *testing.T) {
	env := mergedEnvironment(
		[]string{"HOST_ONLY=visible", "PATH=/bin"},
		nil,
	)
	if !containsEnvironment(env, "HOST_ONLY=visible") {
		t.Fatalf("environment = %#v, want undeclared host value when no Factory env is declared", env)
	}
}

func TestMergedEnvironmentDropsUndeclaredHostValuesWhenFactoryDeclaresBoundedEnv(t *testing.T) {
	env := mergedEnvironment(
		[]string{
			"PATH=/bin",
			"HOST_ONLY=must-not-reach-command",
			"FACTORY_SCRIPT_ENV=declared-value",
		},
		map[string]string{"FACTORY_SCRIPT_ENV": "declared-value"},
	)
	if !containsEnvironment(env, "PATH=/bin") {
		t.Fatalf("environment = %#v, want inherited PATH for command resolution", env)
	}
	if !containsEnvironment(env, "FACTORY_SCRIPT_ENV=declared-value") {
		t.Fatalf("environment = %#v, want declared Factory env value", env)
	}
	if containsEnvironment(env, "HOST_ONLY=must-not-reach-command") {
		t.Fatalf("environment = %#v, want undeclared host value filtered out", env)
	}
}

func TestMergedEnvironmentHandlesEmptyAndMalformedInheritedEntries(t *testing.T) {
	if got := filterInheritedExecutionEnvironment(nil); got != nil {
		t.Fatalf("filterInheritedExecutionEnvironment(nil) = %#v, want nil", got)
	}

	got := filterInheritedExecutionEnvironment([]string{
		"MALFORMED",
		"=missing-name",
		"PATH=/bin",
	})
	if len(got) != 1 || got[0] != "PATH=/bin" {
		t.Fatalf("filtered environment = %#v, want only inherited PATH", got)
	}
}

func containsEnvironment(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
