// Package commandenv owns the shared subprocess environment policy for CLI providers.
package commandenv

import workerprocess "github.com/portpowered/infinite-you/pkg/platform/process"

var automationDefaults = []workerprocess.CommandEnvEntry{
	{Name: "GIT_EDITOR", Value: "true"},
	{Name: "GIT_SEQUENCE_EDITOR", Value: "true"},
	{Name: "GIT_MERGE_AUTOEDIT", Value: "no"},
	{Name: "GIT_TERMINAL_PROMPT", Value: "0"},
	{Name: "EDITOR", Value: "true"},
	{Name: "VISUAL", Value: "true"},
}

// Build merges environment sources with deterministic precedence: process
// environment, provider variables, then non-interactive automation defaults.
func Build(processEnvironment []string, envVars map[string]string) []string {
	return workerprocess.MergeCommandEnv(
		processEnvironment,
		workerprocess.CommandEnvEntriesFromMap(envVars),
		automationDefaults,
	)
}

// AutomationDefaults returns a copy of the enforced non-interactive defaults.
func AutomationDefaults() []workerprocess.CommandEnvEntry {
	return append([]workerprocess.CommandEnvEntry(nil), automationDefaults...)
}
