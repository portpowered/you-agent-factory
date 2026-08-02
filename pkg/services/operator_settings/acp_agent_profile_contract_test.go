package operatorsettings

import (
	"errors"
	"reflect"
	"testing"
)

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

func TestDocumentCloneDetachesACPAgentProfile(t *testing.T) {
	t.Parallel()

	document := Document{
		ACPAgentProfile: &DocumentACPAgentProfile{
			DefaultFactoryReference: "@you/custom",
			Allowlist:               []string{"@you/custom"},
		},
	}
	cloned := document.Clone()
	cloned.ACPAgentProfile.Allowlist[0] = "mutated"
	if document.ACPAgentProfile.Allowlist[0] == "mutated" {
		t.Fatalf("mutating cloned document changed original ACPAgentProfile: %#v", document.ACPAgentProfile)
	}
	cloned.ACPAgentProfile.DefaultFactoryReference = "mutated"
	if document.ACPAgentProfile.DefaultFactoryReference == "mutated" {
		t.Fatalf("mutating cloned document changed original ACPAgentProfile default: %#v", document.ACPAgentProfile)
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
