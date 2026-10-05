package service

import (
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestEmptyPortableArtifactRejectsInvalidSummary(t *testing.T) {
	for _, tc := range []struct {
		name    string
		summary recordings.PortableArtifactSummary
		want    error
	}{
		{name: "unavailable", want: recordings.ErrInvalidPortableArtifact},
		{name: "active", summary: recordings.PortableArtifactSummary{RecordingID: "recording", Available: true, State: recordings.RecordingActive}, want: recordings.ErrPortableArtifactUnavailable},
		{name: "unexpected cursor", summary: recordings.PortableArtifactSummary{RecordingID: "recording", Available: true, State: recordings.RecordingFinalized, FirstCursor: &recordings.CanonicalEventCursor{Sequence: 1}}, want: recordings.ErrInvalidPortableArtifactOrder},
		{name: "empty finalized", summary: recordings.PortableArtifactSummary{RecordingID: "recording", Available: true, State: recordings.RecordingFinalized}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validatePortableArtifactSummary(recordings.PortableArtifact{Summary: tc.summary}); !errors.Is(err, tc.want) {
				t.Fatalf("summary error = %v, want %v", err, tc.want)
			}
		})
	}
}
