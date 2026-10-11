// Package commandenv owns the shared subprocess environment policy for CLI providers.
package commandenv

import (
	"strings"

	workerprocess "github.com/portpowered/infinite-you/pkg/platform/process"
)

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
		ProviderVariables(envVars),
		automationDefaults,
	)
}

// ProviderVariables returns deterministic ordinary overrides. Worker Session
// identity can only arrive through the execution-only process environment;
// provider configuration must neither replace it nor invent an absent target.
func ProviderVariables(envVars map[string]string) []workerprocess.CommandEnvEntry {
	entries := workerprocess.CommandEnvEntriesFromMap(envVars)
	filtered := entries[:0]
	for _, entry := range entries {
		switch strings.ToUpper(entry.Name) {
		case "YOU_SERVER", "YOU_WORKER_SESSION_ID", "YOU_WORKER_SESSION_TOKEN",
			"YOU_MESSAGE_TARGET", "YOU_MESSAGE_TARGET_WORK_ID", "YOU_WORK_ID", "YOU_FACTORY_SESSION_ID":
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

// AutomationDefaults returns a copy of the enforced non-interactive defaults.
func AutomationDefaults() []workerprocess.CommandEnvEntry {
	return append([]workerprocess.CommandEnvEntry(nil), automationDefaults...)
}
