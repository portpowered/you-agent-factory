package operatorsettings

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestACPIntegrationSemanticUpdatesPreserveUnrelatedSettings(t *testing.T) {
	t.Parallel()

	service := ConfigDocumentService{}
	document := ConfigDocument{config: Config{
		BackendScopeID: "local-scope",
		Defaults:       Defaults{WorkerModelProvider: "CODEX", WorkerModel: "model"},
		WorkerPresets:  []WorkerPreset{{ID: "build", ModelProvider: "CODEX"}},
	}}
	added, err := service.AddACPIntegration(document, ACPIntegration{
		ID: "entry-1", Name: "cursor-acp", Transport: "STDIO", Command: " cursor-agent acp ",
	})
	if err != nil {
		t.Fatalf("AddACPIntegration() error = %v", err)
	}
	got := added.FileConfig()
	if got.BackendScopeID != "local-scope" || got.Defaults != document.config.Defaults || !reflect.DeepEqual(got.WorkerPresets, document.config.WorkerPresets) {
		t.Fatalf("unrelated settings changed: %#v", got)
	}
	wantIntegration := ACPIntegration{ID: "entry-1", Name: "cursor-acp", Transport: "stdio", Command: "cursor-agent acp"}
	if !reflect.DeepEqual(got.Workers.ACP.Integrations, []ACPIntegration{wantIntegration}) {
		t.Fatalf("integrations = %#v, want %#v", got.Workers.ACP.Integrations, wantIntegration)
	}

	deleted, err := service.DeleteACPIntegration(added, " cursor-acp ")
	if err != nil {
		t.Fatalf("DeleteACPIntegration() error = %v", err)
	}
	if len(deleted.FileConfig().Workers.ACP.Integrations) != 0 {
		t.Fatalf("integrations after delete = %#v, want empty", deleted.FileConfig().Workers.ACP.Integrations)
	}
	if _, err := service.DeleteACPIntegration(deleted, "cursor-acp"); !errors.Is(err, ErrACPIntegrationNotFound) {
		t.Fatalf("DeleteACPIntegration(missing) error = %v, want ErrACPIntegrationNotFound", err)
	}
}

func TestACPIntegrationRejectsDuplicateAndMalformedProviderIdentities(t *testing.T) {
	t.Parallel()

	service := ConfigDocumentService{}
	base := ConfigDocument{config: Config{Workers: WorkerSettings{ACP: ACPSettings{Integrations: []ACPIntegration{{
		ID: "entry-1", Name: "cursor-acp", Transport: "stdio", Command: "cursor-agent acp",
	}}}}}}
	if _, err := service.AddACPIntegration(base, ACPIntegration{
		ID: "entry-2", Name: "cursor-acp", Transport: "stdio", Command: "replacement",
	}); err == nil {
		t.Fatal("AddACPIntegration(duplicate name) error = nil")
	}
	if _, err := service.AddACPIntegration(ConfigDocument{}, ACPIntegration{
		ID: "entry-1", Name: "Cursor ACP", Transport: "stdio", Command: "cursor-agent acp",
	}); err == nil {
		t.Fatal("AddACPIntegration(malformed name) error = nil")
	}
	for _, integration := range []ACPIntegration{
		{ID: "entry-1", Name: "missing-command", Transport: "stdio"},
		{ID: "entry-1", Name: "custom-acp", Transport: "http", Command: "agent acp"},
	} {
		if _, err := service.AddACPIntegration(ConfigDocument{}, integration); err == nil {
			t.Fatalf("AddACPIntegration(%#v) error = nil", integration)
		}
	}
	duplicateID := ConfigDocument{config: Config{Workers: WorkerSettings{ACP: ACPSettings{Integrations: []ACPIntegration{
		{ID: "same", Name: "first-acp", Transport: "stdio", Command: "first acp"},
		{ID: "same", Name: "second-acp", Transport: "stdio", Command: "second acp"},
	}}}}}
	if _, err := duplicateID.config.Normalize(); err == nil {
		t.Fatal("Normalize(duplicate ACP ID) error = nil")
	}
}

func TestConfigureACPIntegrationHonorsCanceledContextBeforeIO(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	service := ConfigDocumentService{}
	if _, err := service.ConfigureACPIntegrationAdd(ctx, "config.json", ACPIntegration{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("ConfigureACPIntegrationAdd(canceled) = %v", err)
	}
	if _, err := service.ConfigureACPIntegrationDelete(ctx, "config.json", "cursor-acp"); !errors.Is(err, context.Canceled) {
		t.Fatalf("ConfigureACPIntegrationDelete(canceled) = %v", err)
	}
}

func TestConfigureACPIntegrationRejectsNilContext(t *testing.T) {
	t.Parallel()

	service := ConfigDocumentService{}
	const want = "operator config context is required"
	if _, err := service.ConfigureACPIntegrationAdd(nil, "config.json", ACPIntegration{}); err == nil || err.Error() != want {
		t.Fatalf("ConfigureACPIntegrationAdd(nil) = %v, want %q", err, want)
	}
	if _, err := service.ConfigureACPIntegrationDelete(nil, "config.json", "cursor-acp"); err == nil || err.Error() != want {
		t.Fatalf("ConfigureACPIntegrationDelete(nil) = %v, want %q", err, want)
	}
	if _, err := service.EnsurePackagedACPIntegrations(nil, "config.json", nil); err == nil || err.Error() != want {
		t.Fatalf("EnsurePackagedACPIntegrations(nil) = %v, want %q", err, want)
	}
}

func TestBuiltInACPAgentProfileIsDeterministicAndValid(t *testing.T) {
	t.Parallel()

	profile := BuiltInACPAgentProfile()
	if profile.DefaultFactoryReference != DefaultACPAgentFactoryReference {
		t.Fatalf("DefaultFactoryReference = %q, want %q", profile.DefaultFactoryReference, DefaultACPAgentFactoryReference)
	}
	if !reflect.DeepEqual(profile.Allowlist, []string{DefaultACPAgentFactoryReference}) {
		t.Fatalf("Allowlist = %#v, want [%q]", profile.Allowlist, DefaultACPAgentFactoryReference)
	}

	normalized, err := NormalizeACPAgentProfile(profile.DefaultFactoryReference, profile.Allowlist)
	if err != nil {
		t.Fatalf("NormalizeACPAgentProfile(builtin) error = %v", err)
	}
	if !reflect.DeepEqual(normalized, profile) {
		t.Fatalf("NormalizeACPAgentProfile(builtin) = %#v, want %#v", normalized, profile)
	}
}

func TestACPAgentProfileCloneIsDetached(t *testing.T) {
	t.Parallel()

	profile := ACPAgentProfile{
		DefaultFactoryReference: "@you/factory-builder",
		Allowlist:               []string{"@you/factory-builder", "@you/other"},
	}
	cloned := profile.Clone()
	cloned.Allowlist[0] = "mutated"
	if profile.Allowlist[0] == "mutated" {
		t.Fatalf("mutating clone allowlist changed original: %#v", profile.Allowlist)
	}

	documentProfile := DocumentACPAgentProfile{
		DefaultFactoryReference: "@you/factory-builder",
		Allowlist:               []string{"@you/factory-builder"},
	}
	clonedDocument := documentProfile.Clone()
	clonedDocument.Allowlist[0] = "mutated"
	if documentProfile.Allowlist[0] == "mutated" {
		t.Fatalf("mutating clone allowlist changed original document profile: %#v", documentProfile.Allowlist)
	}
}

func TestNormalizeACPAgentProfileAcceptsValidAuthoredProfile(t *testing.T) {
	t.Parallel()

	profile, err := NormalizeACPAgentProfile("@you/custom", []string{"@you/custom", "@you/factory-builder"})
	if err != nil {
		t.Fatalf("NormalizeACPAgentProfile() error = %v", err)
	}
	want := ACPAgentProfile{
		DefaultFactoryReference: "@you/custom",
		Allowlist:               []string{"@you/custom", "@you/factory-builder"},
	}
	if !reflect.DeepEqual(profile, want) {
		t.Fatalf("NormalizeACPAgentProfile() = %#v, want %#v", profile, want)
	}
}

func TestNormalizeACPAgentProfileRejectsInvalidInputs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		defaultRef string
		allowlist  []string
	}{
		{name: "blank default", defaultRef: "", allowlist: []string{"@you/factory-builder"}},
		{name: "whitespace default", defaultRef: " @you/factory-builder ", allowlist: []string{"@you/factory-builder"}},
		{name: "blank allowlist entry", defaultRef: "@you/factory-builder", allowlist: []string{"@you/factory-builder", ""}},
		{name: "whitespace allowlist entry", defaultRef: "@you/factory-builder", allowlist: []string{"@you/factory-builder", " @you/other"}},
		{name: "duplicate allowlist entries", defaultRef: "@you/factory-builder", allowlist: []string{"@you/factory-builder", "@you/factory-builder"}},
		{name: "default absent from allowlist", defaultRef: "@you/custom", allowlist: []string{"@you/factory-builder"}},
		{name: "default absent from empty allowlist", defaultRef: "@you/custom", allowlist: nil},
		{name: "default missing scope separator", defaultRef: "factory-builder", allowlist: []string{"factory-builder"}},
		{name: "default missing scope prefix", defaultRef: "you/factory-builder", allowlist: []string{"you/factory-builder"}},
		{name: "default contains internal space", defaultRef: "@you/bad ref", allowlist: []string{"@you/bad ref"}},
		{name: "default contains uppercase", defaultRef: "@You/Factory-Builder", allowlist: []string{"@You/Factory-Builder"}},
		{name: "default contains control character", defaultRef: "@you/factory\tbuilder", allowlist: []string{"@you/factory\tbuilder"}},
		{name: "default has empty scope", defaultRef: "@/factory-builder", allowlist: []string{"@/factory-builder"}},
		{name: "default has empty name", defaultRef: "@you/", allowlist: []string{"@you/"}},
		{name: "default has multiple segments", defaultRef: "@you/factory/builder", allowlist: []string{"@you/factory/builder"}},
		{name: "allowlist entry is malformed", defaultRef: "@you/factory-builder", allowlist: []string{"@you/factory-builder", "not a reference"}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := NormalizeACPAgentProfile(testCase.defaultRef, testCase.allowlist)
			if err == nil {
				t.Fatal("NormalizeACPAgentProfile() error = nil, want error")
			}
			if !errors.Is(err, ErrACPAgentProfileInvalid) {
				t.Fatalf("NormalizeACPAgentProfile() error = %v, want ErrACPAgentProfileInvalid", err)
			}
			var failure ACPAgentProfileFailure
			if !errors.As(err, &failure) {
				t.Fatalf("NormalizeACPAgentProfile() error = %v, want ACPAgentProfileFailure", err)
			}
			if failure.Kind != ACPAgentProfileFailureKindInvalid {
				t.Fatalf("failure.Kind = %q, want %q", failure.Kind, ACPAgentProfileFailureKindInvalid)
			}
		})
	}
}

func TestACPAgentProfileFailurePersistKindWrapsPersistSentinel(t *testing.T) {
	t.Parallel()

	failure := ACPAgentProfileFailure{Kind: ACPAgentProfileFailureKindPersist, Message: "disk full", Field: "acp-agent-profile.json"}
	if !errors.Is(failure, ErrACPAgentProfilePersistFailed) {
		t.Fatalf("ACPAgentProfileFailure(persist) does not wrap ErrACPAgentProfilePersistFailed: %v", failure)
	}
	if errors.Is(failure, ErrACPAgentProfileInvalid) {
		t.Fatalf("ACPAgentProfileFailure(persist) unexpectedly wraps ErrACPAgentProfileInvalid: %v", failure)
	}
}

func TestACPAgentProfileFailureErrorFormatsMessageAndField(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		failure ACPAgentProfileFailure
		want    string
	}{
		{
			name:    "message and field",
			failure: ACPAgentProfileFailure{Kind: ACPAgentProfileFailureKindInvalid, Message: "blank reference", Field: "default"},
			want:    "ACP agent profile is invalid: blank reference (default)",
		},
		{
			name:    "message only",
			failure: ACPAgentProfileFailure{Kind: ACPAgentProfileFailureKindInvalid, Message: "blank reference"},
			want:    "ACP agent profile is invalid: blank reference",
		},
		{
			name:    "field only",
			failure: ACPAgentProfileFailure{Kind: ACPAgentProfileFailureKindPersist, Field: "acp-agent-profile.json"},
			want:    "ACP agent profile persist failed (acp-agent-profile.json)",
		},
		{
			name:    "neither message nor field",
			failure: ACPAgentProfileFailure{Kind: ACPAgentProfileFailureKindPersist},
			want:    "ACP agent profile persist failed",
		},
		{
			name:    "unrecognized kind falls back to invalid sentinel",
			failure: ACPAgentProfileFailure{Kind: ACPAgentProfileFailureKind("unrecognized")},
			want:    "ACP agent profile is invalid",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := testCase.failure.Error(); got != testCase.want {
				t.Fatalf("ACPAgentProfileFailure.Error() = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestUpdateACPAgentProfileRequestValidateRequiresPath(t *testing.T) {
	t.Parallel()

	err := UpdateACPAgentProfileRequest{DefaultFactoryReference: "@you/custom", Allowlist: []string{"@you/custom"}}.Validate()
	if !errors.Is(err, ErrACPAgentProfileInvalid) {
		t.Fatalf("UpdateACPAgentProfileRequest.Validate() error = %v, want ErrACPAgentProfileInvalid", err)
	}

	err = UpdateACPAgentProfileRequest{Path: "config.json", DefaultFactoryReference: "@you/custom", Allowlist: []string{"@you/custom"}}.Validate()
	if err != nil {
		t.Fatalf("UpdateACPAgentProfileRequest.Validate() with path error = %v, want nil", err)
	}
}
