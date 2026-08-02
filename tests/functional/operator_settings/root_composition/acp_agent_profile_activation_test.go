package root_composition_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/globalconfig"
	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
)

// TestACPAgentProfileResolveAndUpdateActivateThroughPublishedRootContract
// proves ResolveACPAgentProfile and UpdateACPAgentProfile activate through the
// published Operator Settings root, composed only through the accepted public
// settingswire.NewService construction path against a real filesystem, with no
// Chat, Factory catalog, Model, transport, or OpenAPI dependency.
func TestACPAgentProfileResolveAndUpdateActivateThroughPublishedRootContract(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "operator", "config.json")
	settingsRoot := newACPAgentProfileFunctionalRoot(t)

	builtIn, err := settingsRoot.ResolveACPAgentProfile(operatorsettings.ResolveACPAgentProfileRequest{Path: configPath})
	if err != nil {
		t.Fatalf("ResolveACPAgentProfile() with no persisted profile error = %v", err)
	}
	if builtIn.Profile.DefaultFactoryReference != operatorsettings.DefaultACPAgentFactoryReference {
		t.Fatalf("built-in default = %q, want %q", builtIn.Profile.DefaultFactoryReference, operatorsettings.DefaultACPAgentFactoryReference)
	}
	if len(builtIn.Profile.Allowlist) != 1 || builtIn.Profile.Allowlist[0] != operatorsettings.DefaultACPAgentFactoryReference {
		t.Fatalf("built-in allowlist = %#v, want only %q", builtIn.Profile.Allowlist, operatorsettings.DefaultACPAgentFactoryReference)
	}

	updated, err := settingsRoot.UpdateACPAgentProfile(context.Background(), operatorsettings.UpdateACPAgentProfileRequest{
		Path:                    configPath,
		DefaultFactoryReference: "@you/custom-agent",
		Allowlist:               []string{"@you/custom-agent", operatorsettings.DefaultACPAgentFactoryReference},
	})
	if err != nil {
		t.Fatalf("UpdateACPAgentProfile() valid profile error = %v", err)
	}
	if !updated.Persisted {
		t.Fatal("UpdateACPAgentProfile() valid profile Persisted = false, want true")
	}

	reloadedRoot := newACPAgentProfileFunctionalRoot(t)
	reloaded, err := reloadedRoot.ResolveACPAgentProfile(operatorsettings.ResolveACPAgentProfileRequest{Path: configPath})
	if err != nil {
		t.Fatalf("ResolveACPAgentProfile() after reconstruction error = %v", err)
	}
	if reloaded.Profile.DefaultFactoryReference != "@you/custom-agent" {
		t.Fatalf("reloaded default = %q, want @you/custom-agent", reloaded.Profile.DefaultFactoryReference)
	}
	if len(reloaded.Profile.Allowlist) != 2 {
		t.Fatalf("reloaded allowlist = %#v, want 2 entries", reloaded.Profile.Allowlist)
	}

	_, err = reloadedRoot.UpdateACPAgentProfile(context.Background(), operatorsettings.UpdateACPAgentProfileRequest{
		Path:                    configPath,
		DefaultFactoryReference: "not a reference",
		Allowlist:               []string{"not a reference"},
	})
	if !errors.Is(err, operatorsettings.ErrACPAgentProfileInvalid) {
		t.Fatalf("UpdateACPAgentProfile() malformed reference error = %v, want ErrACPAgentProfileInvalid", err)
	}
	var failure operatorsettings.ACPAgentProfileFailure
	if !errors.As(err, &failure) || failure.Kind != operatorsettings.ACPAgentProfileFailureKindInvalid {
		t.Fatalf("UpdateACPAgentProfile() malformed reference error = %v, want ACPAgentProfileFailure(kind=invalid)", err)
	}

	afterRejected, err := reloadedRoot.ResolveACPAgentProfile(operatorsettings.ResolveACPAgentProfileRequest{Path: configPath})
	if err != nil {
		t.Fatalf("ResolveACPAgentProfile() after rejected update error = %v", err)
	}
	if afterRejected.Profile.DefaultFactoryReference != "@you/custom-agent" {
		t.Fatalf("profile after rejected update = %q, want prior @you/custom-agent preserved", afterRejected.Profile.DefaultFactoryReference)
	}

	if _, err := os.Stat(configPath); err == nil || !os.IsNotExist(err) {
		t.Fatalf("operator config document at %q exists = %v, want untouched by ACP agent profile storage", configPath, err)
	}
}

func newACPAgentProfileFunctionalRoot(t *testing.T) operatorsettings.Service {
	t.Helper()

	providersRoot, err := providerswire.NewService()
	if err != nil {
		t.Fatalf("providerswire.NewService() error = %v", err)
	}
	settingsRoot, err := settingswire.NewService(
		platformfilesystem.Local{},
		func(dir, pattern string) (operatorsettings.TemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		globalconfigmapping.Decode,
		globalconfigmapping.Encode,
		func(string) (string, bool) { return "", false },
		providersRoot,
		func() string { return "00000000-0000-4000-8000-000000000001" },
		nil,
	)
	if err != nil {
		t.Fatalf("settingswire.NewService() error = %v", err)
	}
	return settingsRoot
}
