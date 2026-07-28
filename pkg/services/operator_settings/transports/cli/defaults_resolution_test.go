package cli_test

import (
	"strings"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	operatorsettingscli "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/cli"
)

func TestResolveOperatorDefaults_CanonicalizesAcceptedProviderAliases(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := operatorsettings.DefaultConfigPath(homeDir)
	root := newFakeSettingsRoot(map[string]operatorsettings.Document{
		configPath: {
			Defaults: operatorsettings.DocumentDefaults{
				WorkerModelProvider: "openai",
				WorkerModel:         "gpt-5-codex",
			},
			Runtime: operatorsettings.EmptyDocument().Runtime,
		},
	})
	service := operatorsettingscli.New(root)
	if service == nil {
		t.Fatal("New(root) = nil, want Settings CLI service")
	}

	got, err := service.ResolveOperatorDefaults(operatorsettingscli.ResolveOperatorDefaultsConfig{
		HomeDir: homeDir,
	})
	if err != nil {
		t.Fatalf("ResolveOperatorDefaults() error = %v", err)
	}
	if got.WorkerModelProvider != "CODEX" {
		t.Fatalf("provider = %q, want CODEX from openai alias", got.WorkerModelProvider)
	}
}

func TestResolveOperatorDefaults_UnresolvedDefaultPreservesDocumentedGuidance(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	root := newFakeSettingsRoot(nil)
	service := operatorsettingscli.New(root)
	if service == nil {
		t.Fatal("New(root) = nil, want Settings CLI service")
	}

	_, err := service.ResolveOperatorDefaults(operatorsettingscli.ResolveOperatorDefaultsConfig{
		HomeDir: homeDir,
		Flags: operatorsettings.FlagOverrides{
			WorkerModelProvider: "DEFAULT",
		},
	})
	if err == nil {
		t.Fatal("expected unresolved DEFAULT provider error")
	}
	for _, want := range []string{
		"DEFAULT requires a concrete provider",
		"YOU_DEFAULT_WORKER_MODEL_PROVIDER",
		"--default-worker-model-provider",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want documented guidance %q", err.Error(), want)
		}
	}
}
