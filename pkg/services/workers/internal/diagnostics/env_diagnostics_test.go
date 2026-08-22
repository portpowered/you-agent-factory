package diagnostics_test

import (
	"reflect"
	"testing"

	workerdiagnostics "github.com/portpowered/infinite-you/pkg/services/workers/internal/diagnostics"
)

func TestClassifyCommandEnvKey_SensitiveNamesAreRedacted(t *testing.T) {
	testCases := []string{
		"API_TOKEN",
		"client_secret",
		"PASSWORD",
		"PASS",
		"OPENAI_API_KEY",
		"ANTHROPIC_AUTH_TOKEN",
		"GOOGLE_APPLICATION_CREDENTIALS",
	}

	for _, name := range testCases {
		t.Run(name, func(t *testing.T) {
			if got := workerdiagnostics.ClassifyCommandEnvKey(name); got != workerdiagnostics.CommandEnvClassificationRedacted {
				t.Fatalf("workerdiagnostics.ClassifyCommandEnvKey(%q) = %q, want %q", name, got, workerdiagnostics.CommandEnvClassificationRedacted)
			}
		})
	}
}

func TestClassifyCommandEnvKey_SafeAllowlistOnlyPermitsLowRiskValues(t *testing.T) {
	testCases := map[string]workerdiagnostics.CommandEnvClassification{
		"CI":                    workerdiagnostics.CommandEnvClassificationSafe,
		"GIT_TERMINAL_PROMPT":   workerdiagnostics.CommandEnvClassificationSafe,
		"TERM":                  workerdiagnostics.CommandEnvClassificationSafe,
		"PATH":                  workerdiagnostics.CommandEnvClassificationMetadataOnly,
		"HOME":                  workerdiagnostics.CommandEnvClassificationMetadataOnly,
		"AGENT_FACTORY_API_KEY": workerdiagnostics.CommandEnvClassificationRedacted,
		"PORTOS_API_KEY":        workerdiagnostics.CommandEnvClassificationRedacted,
	}

	for name, want := range testCases {
		t.Run(name, func(t *testing.T) {
			if got := workerdiagnostics.ClassifyCommandEnvKey(name); got != want {
				t.Fatalf("workerdiagnostics.ClassifyCommandEnvKey(%q) = %q, want %q", name, got, want)
			}
		})
	}
}

func TestProjectCommandEnvForDiagnostics_PreservesSafeMetadataAndRedactsSecrets(t *testing.T) {
	projection := workerdiagnostics.ProjectCommandEnvForDiagnostics([]string{
		"CI=true",
		"PATH=C:\\Tools",
		"OPENAI_API_KEY=sk-raw-secret",
		"ANTHROPIC_AUTH_TOKEN=raw-token",
		"INVALID_ENTRY",
	})

	if projection.Count != 4 {
		t.Fatalf("Count = %d, want 4 valid environment entries", projection.Count)
	}
	wantKeys := []string{"ANTHROPIC_AUTH_TOKEN", "CI", "OPENAI_API_KEY", "PATH"}
	if !reflect.DeepEqual(projection.Keys, wantKeys) {
		t.Fatalf("Keys = %#v, want %#v", projection.Keys, wantKeys)
	}
	if projection.Values["CI"] != "true" {
		t.Fatalf("CI value = %q, want raw allowlisted value", projection.Values["CI"])
	}
	if projection.Values["PATH"] != workerdiagnostics.MetadataOnlyCommandEnvValue {
		t.Fatalf("PATH value = %q, want metadata marker", projection.Values["PATH"])
	}
	if projection.Values["OPENAI_API_KEY"] != workerdiagnostics.RedactedCommandEnvValue {
		t.Fatalf("OPENAI_API_KEY value = %q, want redaction marker", projection.Values["OPENAI_API_KEY"])
	}
	if projection.Values["ANTHROPIC_AUTH_TOKEN"] != workerdiagnostics.RedactedCommandEnvValue {
		t.Fatalf("ANTHROPIC_AUTH_TOKEN value = %q, want redaction marker", projection.Values["ANTHROPIC_AUTH_TOKEN"])
	}
	for name, value := range projection.Values {
		if value == "sk-raw-secret" || value == "raw-token" {
			t.Fatalf("projection leaked raw sensitive value for %s", name)
		}
	}
}

func TestCommandEnvDiagnosticMetadata_RecordsCountAndKeySet(t *testing.T) {
	metadata := workerdiagnostics.CommandEnvDiagnosticMetadata(workerdiagnostics.CommandEnvDiagnosticProjection{
		Count: 2,
		Keys:  []string{"CI", "OPENAI_API_KEY"},
	})

	if metadata["env_count"] != "2" {
		t.Fatalf("env_count = %q, want 2", metadata["env_count"])
	}
	if metadata["env_keys"] != "CI,OPENAI_API_KEY" {
		t.Fatalf("env_keys = %q, want sorted key list", metadata["env_keys"])
	}
}
