package workers

import (
	"strings"

	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
)

var providerAutomationEnvDefaults = []workerprocess.CommandEnvEntry{
	{Name: "GIT_EDITOR", Value: "true"},
	{Name: "GIT_SEQUENCE_EDITOR", Value: "true"},
	{Name: "GIT_MERGE_AUTOEDIT", Value: "no"},
	{Name: "GIT_TERMINAL_PROMPT", Value: "0"},
	{Name: "EDITOR", Value: "true"},
	{Name: "VISUAL", Value: "true"},
}

func assertEnvValue(t testingT, env []string, name, want string) {
	values := envSliceToMap(env)
	if got := values[name]; got != want {
		t.Fatalf("expected env %s=%q, got %q", name, want, got)
	}
}

func assertEnvEntryCount(t testingT, env []string, name string, want int) {
	prefix := name + "="
	got := 0
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			got++
		}
	}
	if got != want {
		t.Fatalf("expected env %s to appear %d time(s), got %d in %v", name, want, got, env)
	}
}

type testingT interface {
	Fatalf(format string, args ...any)
}
