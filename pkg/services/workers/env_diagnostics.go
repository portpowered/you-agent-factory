package workers

import (
	"fmt"
	"sort"
	"strings"
)

const (
	// RedactedCommandEnvValue is written when an environment key looks sensitive.
	RedactedCommandEnvValue = "<redacted>"
	// MetadataOnlyCommandEnvValue is written when only the key's presence is safe to preserve.
	MetadataOnlyCommandEnvValue = "<metadata-only>"
)

// CommandEnvClassification describes how a command environment entry may be
// represented in persisted diagnostics.
type CommandEnvClassification string

const (
	CommandEnvClassificationSafe         CommandEnvClassification = "safe"
	CommandEnvClassificationRedacted     CommandEnvClassification = "redacted"
	CommandEnvClassificationMetadataOnly CommandEnvClassification = "metadata_only"
)

// CommandEnvDiagnosticProjection is the safe diagnostic view of a subprocess
// environment. Values contains only allowlisted raw values or explicit markers.
type CommandEnvDiagnosticProjection struct {
	Count  int               `json:"count"`
	Keys   []string          `json:"keys,omitempty"`
	Values map[string]string `json:"values,omitempty"`
}

var safeCommandEnvKeys = map[string]struct{}{
	"CI":                  {},
	"CGO_ENABLED":         {},
	"EDITOR":              {},
	"FORCE_COLOR":         {},
	"GIT_EDITOR":          {},
	"GIT_MERGE_AUTOEDIT":  {},
	"GIT_SEQUENCE_EDITOR": {},
	"GIT_TERMINAL_PROMPT": {},
	"GOARCH":              {},
	"GOOS":                {},
	"NO_COLOR":            {},
	"OS":                  {},
	"RUNNER_OS":           {},
	"TERM":                {},
	"VISUAL":              {},
}

var sensitiveCommandEnvNameFragments = []string{
	"TOKEN",
	"SECRET",
	"PASSWORD",
	"PASS",
	"KEY",
	"CREDENTIAL",
	"CREDENTIALS",
	"AUTH",
	"ANTHROPIC",
	"OPENAI",
	"GEMINI",
	"GOOGLE_APPLICATION_CREDENTIALS",
	"AWS_ACCESS_KEY_ID",
	"AWS_SECRET_ACCESS_KEY",
}

// ClassifyCommandEnvKey returns the diagnostic exposure class for one command
// environment key. Sensitive-looking names are always redacted, even when they
// also appear in the safe allowlist.
func ClassifyCommandEnvKey(name string) CommandEnvClassification {
	normalized := strings.ToUpper(strings.TrimSpace(name))
	if normalized == "" {
		return CommandEnvClassificationMetadataOnly
	}
	for _, fragment := range sensitiveCommandEnvNameFragments {
		if strings.Contains(normalized, fragment) {
			return CommandEnvClassificationRedacted
		}
	}
	if _, ok := safeCommandEnvKeys[normalized]; ok {
		return CommandEnvClassificationSafe
	}
	return CommandEnvClassificationMetadataOnly
}

// ProjectCommandEnvForDiagnostics converts subprocess environment entries into
// the shared safe diagnostic representation used before replay persistence.
func ProjectCommandEnvForDiagnostics(env []string) CommandEnvDiagnosticProjection {
	projection := CommandEnvDiagnosticProjection{}
	if len(env) == 0 {
		return projection
	}

	seenKeys := make(map[string]struct{}, len(env))
	values := make(map[string]string, len(env))
	for _, entry := range env {
		name, value, ok := strings.Cut(entry, "=")
		if !ok || name == "" {
			continue
		}
		projection.Count++
		seenKeys[name] = struct{}{}
		switch ClassifyCommandEnvKey(name) {
		case CommandEnvClassificationSafe:
			values[name] = value
		case CommandEnvClassificationRedacted:
			values[name] = RedactedCommandEnvValue
		default:
			values[name] = MetadataOnlyCommandEnvValue
		}
	}

	if len(seenKeys) > 0 {
		projection.Keys = make([]string, 0, len(seenKeys))
		for name := range seenKeys {
			projection.Keys = append(projection.Keys, name)
		}
		sort.Strings(projection.Keys)
	}
	if len(values) > 0 {
		projection.Values = values
	}
	return projection
}

func commandEnvDiagnosticMetadata(projection CommandEnvDiagnosticProjection) map[string]string {
	if projection.Count == 0 && len(projection.Keys) == 0 {
		return nil
	}
	return map[string]string{
		"env_count": fmt.Sprintf("%d", projection.Count),
		"env_keys":  strings.Join(projection.Keys, ","),
	}
}
