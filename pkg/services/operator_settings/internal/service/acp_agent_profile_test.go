package service_test

import (
	"errors"
	"reflect"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	operatorservice "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/service"
	documentwire "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/document/wire"
	resolutionwire "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution/wire"
	internaltestproviders "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/testproviders"
)

func newACPAgentProfileTestRoot(t *testing.T) operatorsettings.Service {
	t.Helper()

	providersRoot := internaltestproviders.StandardCatalog()
	documentService := documentwire.NewService(
		&rootTestFileSystem{},
		rootTestCreateTemporaryFile,
		rootTestConfigDecoder,
		rootTestConfigEncoder,
		rootTestProviderCatalog,
	)
	resolutionService, err := resolutionwire.NewService(providersRoot)
	if err != nil {
		t.Fatalf("resolutionwire.NewService() = %v", err)
	}
	root, err := operatorservice.New(
		documentService,
		resolutionService,
		&rootTestFileSystem{},
		rootTestCreateTemporaryFile,
		rootTestConfigDecoder,
		rootTestConfigEncoder,
		func() string { return "00000000-0000-4000-8000-000000000001" },
	)
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	return root
}

func TestResolveACPAgentProfileWithNoAuthoredProfileReturnsBuiltIn(t *testing.T) {
	t.Parallel()

	root := newACPAgentProfileTestRoot(t)

	result, err := root.ResolveACPAgentProfile(operatorsettings.ResolveACPAgentProfileRequest{})
	if err != nil {
		t.Fatalf("ResolveACPAgentProfile() error = %v", err)
	}
	if !reflect.DeepEqual(result.Profile, operatorsettings.BuiltInACPAgentProfile()) {
		t.Fatalf("ResolveACPAgentProfile() = %#v, want built-in profile", result.Profile)
	}
}

func TestResolveACPAgentProfileWithAuthoredProfileReturnsNormalizedValue(t *testing.T) {
	t.Parallel()

	root := newACPAgentProfileTestRoot(t)

	authored := &operatorsettings.DocumentACPAgentProfile{
		DefaultFactoryReference: "@you/custom",
		Allowlist:               []string{"@you/factory-builder", "@you/custom"},
	}
	result, err := root.ResolveACPAgentProfile(operatorsettings.ResolveACPAgentProfileRequest{AuthoredProfile: authored})
	if err != nil {
		t.Fatalf("ResolveACPAgentProfile() error = %v", err)
	}
	want := operatorsettings.ACPAgentProfile{
		DefaultFactoryReference: "@you/custom",
		Allowlist:               []string{"@you/factory-builder", "@you/custom"},
	}
	if !reflect.DeepEqual(result.Profile, want) {
		t.Fatalf("ResolveACPAgentProfile() = %#v, want %#v", result.Profile, want)
	}

	// Mutating the request input after resolution must not affect the
	// already-returned detached result.
	authored.Allowlist[0] = "mutated"
	if result.Profile.Allowlist[0] == "mutated" {
		t.Fatalf("resolved profile was not detached from request input: %#v", result.Profile.Allowlist)
	}
}

func TestResolveACPAgentProfileWithInvalidAuthoredProfileFails(t *testing.T) {
	t.Parallel()

	root := newACPAgentProfileTestRoot(t)

	authored := &operatorsettings.DocumentACPAgentProfile{
		DefaultFactoryReference: "@you/custom",
		Allowlist:               []string{"@you/factory-builder"},
	}
	_, err := root.ResolveACPAgentProfile(operatorsettings.ResolveACPAgentProfileRequest{AuthoredProfile: authored})
	if !errors.Is(err, operatorsettings.ErrACPAgentProfileInvalid) {
		t.Fatalf("ResolveACPAgentProfile() error = %v, want ErrACPAgentProfileInvalid", err)
	}
}
