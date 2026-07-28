package identityinputinventory

import (
	"strings"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

func TestIndexedPersistScopeCases_MatchProductionLoader(t *testing.T) {
	cases := []struct {
		name           string
		scopeID        string
		wantErrorParts []string
	}{
		{name: "valid-local-scope", scopeID: "local-aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"},
		{name: "invalid-empty-scope", wantErrorParts: []string{"backend scope ID is required"}},
		{name: "invalid-non-local-scope", scopeID: "not-a-local-scope", wantErrorParts: []string{"not a valid local backend scope"}},
		{name: "invalid-provider-scope", scopeID: "provider-codex-account-workspace", wantErrorParts: []string{"not a valid local backend scope"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			runPersistScopeCase(t, testCase.scopeID, testCase.wantErrorParts)
		})
	}
}

func runPersistScopeCase(t *testing.T, scopeID string, wantErrorParts []string) {
	t.Helper()

	configPath := operatorsettings.DefaultConfigPath(t.TempDir())
	err := persistBackendScopeID(testFiles, testCreateTemp, encodeTestConfig, configPath, operatorsettings.Config{BackendScopeID: scopeID})
	if len(wantErrorParts) == 0 {
		if err != nil {
			t.Fatalf("persistBackendScopeID() error = %v, want accept", err)
		}
		return
	}
	if err == nil {
		t.Fatal("persistBackendScopeID() error = nil, want reject")
	}
	for _, fragment := range wantErrorParts {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("error = %q, want fragment %q", err.Error(), fragment)
		}
	}
}
