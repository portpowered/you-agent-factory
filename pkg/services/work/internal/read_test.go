package internal_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestProjectExpectedArtifactReadModels_TracksPendingSatisfiedFailedAndMixed(t *testing.T) {
	t.Parallel()

	declarations := []work.ExpectedArtifactDeclaration{
		{Name: "report", Pattern: "reports/{{ (index .Inputs 0).Name }}.json", NonEmpty: true},
		{Name: "manifest", Pattern: "reports/manifest.json"},
		{Name: "duplicate", Pattern: "reports/manifest.json"},
	}
	inputs := []work.ExpectedArtifactInput{{Name: "review", WorkID: "work-1", WorkTypeID: "story"}}

	pending := work.ExpectedArtifactReadModelProjector{}.Project(declarations, nil, inputs, work.ExpectedArtifactObservation{})
	if len(pending) != 3 || pending[0].Pattern != "reports/review.json" ||
		pending[0].Verification != work.ExpectedArtifactVerificationPending {
		t.Fatalf("pending projection = %#v", pending)
	}

	satisfied := work.ExpectedArtifactReadModelProjector{}.Project(declarations, nil, inputs, work.ExpectedArtifactObservation{Verified: true})
	if len(satisfied) != 3 || satisfied[0].Verification != work.ExpectedArtifactVerificationSatisfied ||
		satisfied[1].Verification != work.ExpectedArtifactVerificationSatisfied {
		t.Fatalf("satisfied projection = %#v", satisfied)
	}

	failed := work.ExpectedArtifactReadModelProjector{}.Project(declarations, nil, inputs, work.ExpectedArtifactObservation{
		Verified: true,
		Entries: []work.ExpectedArtifactVerificationEntry{{
			Name: "manifest", Pattern: "reports/manifest.json", Reason: work.ExpectedArtifactVerificationReasonEmpty,
		}},
	})
	if len(failed) != 3 || failed[0].Verification != work.ExpectedArtifactVerificationSatisfied ||
		failed[1].Verification != work.ExpectedArtifactVerificationFailed || failed[1].Reason == nil ||
		*failed[1].Reason != work.ExpectedArtifactVerificationReasonEmpty ||
		failed[2].Verification != work.ExpectedArtifactVerificationSatisfied {
		t.Fatalf("mixed projection = %#v", failed)
	}

	if got := (work.ExpectedArtifactReadModelProjector{}).Project(nil, nil, inputs, work.ExpectedArtifactObservation{Verified: true}); got != nil {
		t.Fatalf("no-declaration projection = %#v, want nil", got)
	}
}

func TestProjectExpectedArtifactReadModels_RedactsUnsafeRenderedPatterns(t *testing.T) {
	t.Parallel()

	got := work.ExpectedArtifactReadModelProjector{}.Project(
		[]work.ExpectedArtifactDeclaration{{Name: "unsafe", Pattern: "{{ (index .Inputs 0).Name }}"}},
		nil,
		[]work.ExpectedArtifactInput{{Name: "C:\\workspace\\report.json"}},
		work.ExpectedArtifactObservation{},
	)
	if len(got) != 1 || got[0].Pattern != "<invalid>" || got[0].Verification != work.ExpectedArtifactVerificationPending {
		t.Fatalf("unsafe projection = %#v, want redacted pending artifact", got)
	}
}

func TestProjectExpectedArtifactReadModels_UsesRecordedContextAndDeclarationIdentity(t *testing.T) {
	t.Parallel()
	declarations := []work.ExpectedArtifactDeclaration{
		{Name: "same", Pattern: "{{ .Context.Project }}/{{ .Context.SessionID }}/one.txt"},
		{Name: "same", Pattern: "{{ .Context.Project }}/{{ .Context.SessionID }}/two.txt"},
	}
	got := work.ExpectedArtifactReadModelProjector{}.Project(
		declarations,
		nil,
		[]work.ExpectedArtifactInput{{Name: "input", Project: "input-project"}},
		work.ExpectedArtifactObservation{
			Verified: true,
			Entries: []work.ExpectedArtifactVerificationEntry{{
				DeclarationIndex: 2,
				Name:             "same",
				Pattern:          "project-7/session-9/two.txt",
				Reason:           work.ExpectedArtifactVerificationReasonMissing,
			}},
		},
		work.ExpectedArtifactTemplateContext{Project: "project-7", SessionID: "session-9"},
	)
	if len(got) != 2 || got[0].Pattern != "project-7/session-9/one.txt" ||
		got[0].Verification != work.ExpectedArtifactVerificationSatisfied ||
		got[1].Pattern != "project-7/session-9/two.txt" ||
		got[1].Verification != work.ExpectedArtifactVerificationFailed || got[1].Reason == nil ||
		*got[1].Reason != work.ExpectedArtifactVerificationReasonMissing {
		t.Fatalf("context and identity projection = %#v", got)
	}
}

func TestProjectExpectedArtifactReadModels_UsesRecordedReplaySafeInputs(t *testing.T) {
	t.Parallel()
	declarations := []work.ExpectedArtifactDeclaration{{
		Name:    "payload",
		Pattern: "{{ (index .Inputs 0).Project }}/{{ (index .Inputs 0).Payload }}.txt",
	}}
	got := work.ExpectedArtifactReadModelProjector{}.Project(
		declarations,
		nil,
		[]work.ExpectedArtifactInput{{Project: "live-project", Payload: "live-payload"}},
		work.ExpectedArtifactObservation{Verified: true},
		work.ExpectedArtifactTemplateContext{
			Project: "project-7",
			Inputs:  []work.ExpectedArtifactInput{{Project: "recorded-project", Payload: "recorded-payload"}},
		},
	)
	if len(got) != 1 || got[0].Pattern != "recorded-project/recorded-payload.txt" ||
		got[0].Verification != work.ExpectedArtifactVerificationSatisfied {
		t.Fatalf("recorded input projection = %#v", got)
	}
}

func TestProjectExpectedArtifactReadModels_RedactedDuplicateEntriesUseDeclarationIdentity(t *testing.T) {
	t.Parallel()
	declarations := []work.ExpectedArtifactDeclaration{
		{Name: "same", Pattern: "{{ (index .Inputs 9).Name }}.one"},
		{Name: "same", Pattern: "{{ (index .Inputs 8).Name }}.two"},
	}
	got := work.ExpectedArtifactReadModelProjector{}.Project(
		declarations,
		nil,
		[]work.ExpectedArtifactInput{{Name: "input"}},
		work.ExpectedArtifactObservation{
			Entries: []work.ExpectedArtifactVerificationEntry{{
				DeclarationIndex: 2,
				Name:             "same",
				Pattern:          "<unrenderable>",
				Reason:           work.ExpectedArtifactVerificationReasonMissing,
			}},
		},
	)
	if len(got) != 2 || got[0].Verification != work.ExpectedArtifactVerificationPending ||
		got[1].Verification != work.ExpectedArtifactVerificationFailed || got[1].Pattern != "<unrenderable>" {
		t.Fatalf("redacted duplicate projection = %#v", got)
	}
}
