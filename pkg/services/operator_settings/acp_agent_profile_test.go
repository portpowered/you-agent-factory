package operatorsettings

import (
	"errors"
	"testing"
)

func TestACPAgentProfileNormalizeTrimsAndPreservesOrder(t *testing.T) {
	t.Parallel()

	profile := ACPAgentProfile{
		DefaultTarget:  " factory:@you/review ",
		AllowedTargets: []string{" factory:@you/review ", " factory:@you/factory-builder ", " factory:local/software-auto "},
	}
	got, err := profile.Normalize()
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	want := ACPAgentProfile{
		DefaultTarget: "factory:@you/review",
		AllowedTargets: []string{
			"factory:@you/review",
			"factory:@you/factory-builder",
			"factory:local/software-auto",
		},
	}
	if got.DefaultTarget != want.DefaultTarget {
		t.Fatalf("DefaultTarget = %q, want %q", got.DefaultTarget, want.DefaultTarget)
	}
	if len(got.AllowedTargets) != len(want.AllowedTargets) {
		t.Fatalf("AllowedTargets = %#v, want %#v", got.AllowedTargets, want.AllowedTargets)
	}
	for i := range want.AllowedTargets {
		if got.AllowedTargets[i] != want.AllowedTargets[i] {
			t.Fatalf("AllowedTargets[%d] = %q, want %q (order must be preserved)", i, got.AllowedTargets[i], want.AllowedTargets[i])
		}
	}
}

func TestACPAgentProfileNormalizeRejectsInvalidShapes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		profile ACPAgentProfile
	}{
		{
			name:    "blank default",
			profile: ACPAgentProfile{DefaultTarget: "   ", AllowedTargets: []string{"factory:@you/factory-builder"}},
		},
		{
			name:    "default outside factory namespace",
			profile: ACPAgentProfile{DefaultTarget: "worker:@you/factory-builder", AllowedTargets: []string{"worker:@you/factory-builder"}},
		},
		{
			name:    "default is bare namespace with no reference",
			profile: ACPAgentProfile{DefaultTarget: "factory:", AllowedTargets: []string{"factory:"}},
		},
		{
			name:    "blank allowlist entry",
			profile: ACPAgentProfile{DefaultTarget: "factory:@you/factory-builder", AllowedTargets: []string{"factory:@you/factory-builder", "  "}},
		},
		{
			name:    "allowlist entry outside factory namespace",
			profile: ACPAgentProfile{DefaultTarget: "factory:@you/factory-builder", AllowedTargets: []string{"factory:@you/factory-builder", "local/software-auto"}},
		},
		{
			name:    "empty allowlist",
			profile: ACPAgentProfile{DefaultTarget: "factory:@you/factory-builder", AllowedTargets: nil},
		},
		{
			name: "duplicate entries after normalization",
			profile: ACPAgentProfile{
				DefaultTarget:  "factory:@you/factory-builder",
				AllowedTargets: []string{"factory:@you/factory-builder", " factory:@you/factory-builder "},
			},
		},
		{
			name:    "default absent from allowlist",
			profile: ACPAgentProfile{DefaultTarget: "factory:@you/review", AllowedTargets: []string{"factory:@you/factory-builder"}},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if _, err := testCase.profile.Normalize(); !errors.Is(err, ErrACPAgentProfileInvalid) {
				t.Fatalf("Normalize() error = %v, want ErrACPAgentProfileInvalid", err)
			}
		})
	}
}

func TestACPAgentProfileCloneDoesNotAliasAllowedTargets(t *testing.T) {
	t.Parallel()

	original := ACPAgentProfile{
		DefaultTarget:  "factory:@you/factory-builder",
		AllowedTargets: []string{"factory:@you/factory-builder", "factory:@you/review"},
	}
	cloned := original.Clone()
	cloned.AllowedTargets[0] = "factory:mutated/target"

	if original.AllowedTargets[0] != "factory:@you/factory-builder" {
		t.Fatalf("original mutated through clone: %#v", original.AllowedTargets)
	}
}

func TestDefaultACPAgentProfileIsFactoryBuilderOnly(t *testing.T) {
	t.Parallel()

	profile := DefaultACPAgentProfile()
	if profile.DefaultTarget != DefaultACPAgentProfileTarget {
		t.Fatalf("DefaultTarget = %q, want %q", profile.DefaultTarget, DefaultACPAgentProfileTarget)
	}
	if len(profile.AllowedTargets) != 1 || profile.AllowedTargets[0] != DefaultACPAgentProfileTarget {
		t.Fatalf("AllowedTargets = %#v, want exactly [%q]", profile.AllowedTargets, DefaultACPAgentProfileTarget)
	}
	if normalized, err := profile.Normalize(); err != nil {
		t.Fatalf("DefaultACPAgentProfile() must already be normalized, got error = %v", err)
	} else if normalized.DefaultTarget != profile.DefaultTarget {
		t.Fatalf("DefaultACPAgentProfile() normalized to a different value: %#v", normalized)
	}
}

func TestWorkerSettingsNormalizePreservesAgentProfileWithoutIntegrations(t *testing.T) {
	t.Parallel()

	settings := WorkerSettings{ACP: ACPSettings{
		AgentProfile: &ACPAgentProfile{
			DefaultTarget:  " factory:@you/review ",
			AllowedTargets: []string{" factory:@you/review "},
		},
	}}
	normalized, err := settings.normalize()
	if err != nil {
		t.Fatalf("normalize() error = %v", err)
	}
	if normalized.ACP.Integrations != nil {
		t.Fatalf("Integrations = %#v, want nil", normalized.ACP.Integrations)
	}
	if normalized.ACP.AgentProfile == nil || normalized.ACP.AgentProfile.DefaultTarget != "factory:@you/review" {
		t.Fatalf("AgentProfile = %#v, want normalized factory:@you/review", normalized.ACP.AgentProfile)
	}
}

func TestWorkerSettingsNormalizeRejectsInvalidAgentProfile(t *testing.T) {
	t.Parallel()

	settings := WorkerSettings{ACP: ACPSettings{
		AgentProfile: &ACPAgentProfile{DefaultTarget: "", AllowedTargets: nil},
	}}
	if _, err := settings.normalize(); !errors.Is(err, ErrACPAgentProfileInvalid) {
		t.Fatalf("normalize() error = %v, want ErrACPAgentProfileInvalid", err)
	}
}

func TestConfigDocumentFileConfigDoesNotAliasAgentProfile(t *testing.T) {
	t.Parallel()

	document := ConfigDocument{config: Config{Workers: WorkerSettings{ACP: ACPSettings{
		AgentProfile: &ACPAgentProfile{
			DefaultTarget:  "factory:@you/factory-builder",
			AllowedTargets: []string{"factory:@you/factory-builder"},
		},
	}}}}

	first := document.FileConfig()
	first.Workers.ACP.AgentProfile.DefaultTarget = "factory:mutated"
	first.Workers.ACP.AgentProfile.AllowedTargets[0] = "factory:mutated"

	second := document.FileConfig()
	if second.Workers.ACP.AgentProfile.DefaultTarget != "factory:@you/factory-builder" {
		t.Fatalf("stored DefaultTarget mutated through FileConfig(): %#v", second.Workers.ACP.AgentProfile)
	}
	if second.Workers.ACP.AgentProfile.AllowedTargets[0] != "factory:@you/factory-builder" {
		t.Fatalf("stored AllowedTargets mutated through FileConfig(): %#v", second.Workers.ACP.AgentProfile)
	}
}

func TestDocumentACPSettingsCloneDoesNotAliasAgentProfile(t *testing.T) {
	t.Parallel()

	original := DocumentACPSettings{AgentProfile: &ACPAgentProfile{
		DefaultTarget:  "factory:@you/factory-builder",
		AllowedTargets: []string{"factory:@you/factory-builder"},
	}}
	cloned := original.Clone()
	cloned.AgentProfile.DefaultTarget = "factory:mutated"
	cloned.AgentProfile.AllowedTargets[0] = "factory:mutated"

	if original.AgentProfile.DefaultTarget != "factory:@you/factory-builder" {
		t.Fatalf("original DefaultTarget mutated through Clone(): %#v", original.AgentProfile)
	}
	if original.AgentProfile.AllowedTargets[0] != "factory:@you/factory-builder" {
		t.Fatalf("original AllowedTargets mutated through Clone(): %#v", original.AgentProfile)
	}
}
