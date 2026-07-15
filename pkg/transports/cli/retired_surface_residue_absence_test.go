package cli

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factory/packages/goal"
	factorycli "github.com/portpowered/infinite-you/pkg/transports/cli/factory"
	"github.com/portpowered/infinite-you/pkg/workers/cliprovider"
)

func TestRetiredSurfaceResidue_FactorySaveDoesNotInvokeOwningPersistence(t *testing.T) {
	originalCreate := createFactoryFromFile
	originalReplace := replaceFactoryCurrent
	defer func() {
		createFactoryFromFile = originalCreate
		replaceFactoryCurrent = originalReplace
	}()

	createCalled := false
	replaceCalled := false
	createFactoryFromFile = func(factorycli.CreateFromFileConfig) error {
		createCalled = true
		return nil
	}
	replaceFactoryCurrent = func(factorycli.ReplaceCurrentConfig) error {
		replaceCalled = true
		return nil
	}

	root := NewRootCommand()
	var output strings.Builder
	root.SetOut(&output)
	root.SetErr(&output)
	for _, args := range [][]string{
		{"factory", "save", "staging", "--from", "./factory.json"},
		{"factory", "save"},
	} {
		output.Reset()
		root.SetArgs(args)
		if err := root.Execute(); err == nil {
			t.Fatalf("args %v: expected unknown command error", args)
		}
	}

	if createCalled {
		t.Fatal("removed factory save must not invoke create persistence")
	}
	if replaceCalled {
		t.Fatal("removed factory save must not invoke replace-current persistence")
	}
}

func TestRetiredSurfaceResidue_SelectCLIProviderDoesNotInjectDeprecatedDefaults(t *testing.T) {
	discovery := cliprovider.CLIProviderDiscoveryView{
		Registrations: cliprovider.RegisteredCLIProviders(),
		Probe: func(cliprovider.CLIProviderRegistration) cliprovider.CLIProviderAvailability {
			return cliprovider.CLIProviderAvailability{Available: false}
		},
	}
	result := cliprovider.SelectCLIProvider(
		cliprovider.CLIProviderSelectionInput{
			ExplicitInvocation: "openai",
			FactoryDefault:     "DEFAULT",
			SystemDefault:      "DEFAULT",
		},
		discovery,
	)
	if result.OK() {
		t.Fatalf("selected = %#v, want nil without deprecated model-default injection", result.Selected)
	}
	if result.Failure == nil || result.Failure.Code != cliprovider.CLIProviderSelectionFailureNoAgentHarness {
		t.Fatalf("failure = %#v, want NO_AGENT_HARNESS without deprecated fallback", result.Failure)
	}
}

func TestRetiredSurfaceResidue_PackagedGoalPromptsStayOnCanonicalOwner(t *testing.T) {
	if err := goal.CheckPackagedGoalAssembledPromptDrift(); err != nil {
		t.Fatalf("assembled packaged goal prompts drifted from canonical owner: %v", err)
	}
	for _, source := range goal.PackagedGoalRolePromptSources {
		if strings.TrimSpace(source.Role) == "" {
			t.Fatalf("packaged goal prompt source %#v must identify a role", source)
		}
		if source.SourceKind == "" {
			t.Fatalf("packaged goal prompt source for role %q must declare canonical owner kind", source.Role)
		}
	}
}
