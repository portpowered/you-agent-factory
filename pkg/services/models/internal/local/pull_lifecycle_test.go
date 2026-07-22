package local

import (
	"context"
	"errors"
	"fmt"
	"testing"

	apisurface "github.com/portpowered/infinite-you/pkg/services/models"
)

func TestClassifySuccessfulPull_MapsLegacyOutcomesToManagedContract(t *testing.T) {
	t.Parallel()
	t.Run("installed successfully with ready cache", func(t *testing.T) {
		outcome, readiness, lifecycle := classifySuccessfulPull(apisurface.PullResult{
			Outcome: legacyPullOutcomePulled,
		}, RuntimeCacheInspection{Supported: true, Installed: true})
		if outcome != managedPullOutcomeInstalledSuccessfully || readiness != managedReadinessReady || lifecycle != managedLifecycleInstalled {
			t.Fatalf("classified = (%s, %s, %s), want installed successfully READY INSTALLED", outcome, readiness, lifecycle)
		}
	})

	t.Run("already present with ready cache", func(t *testing.T) {
		outcome, readiness, lifecycle := classifySuccessfulPull(apisurface.PullResult{
			Outcome: legacyPullOutcomeAlreadyPresent,
		}, RuntimeCacheInspection{Supported: true, Installed: true})
		if outcome != managedPullOutcomeAlreadyReady || readiness != managedReadinessReady || lifecycle != managedLifecycleInstalled {
			t.Fatalf("classified = (%s, %s, %s), want already ready READY INSTALLED", outcome, readiness, lifecycle)
		}
	})

	t.Run("installed but cache still missing assets", func(t *testing.T) {
		outcome, readiness, lifecycle := classifySuccessfulPull(apisurface.PullResult{
			Outcome: legacyPullOutcomePulled,
		}, RuntimeCacheInspection{Supported: true, Installed: false, MissingAssets: []string{"model.gguf"}})
		if outcome != managedPullOutcomeStillLoading || readiness != managedReadinessLoading || lifecycle != managedLifecycleInstalling {
			t.Fatalf("classified = (%s, %s, %s), want still loading LOADING INSTALLING", outcome, readiness, lifecycle)
		}
	})
}

func TestClassifyPullFailure_MapsErrorsToManagedOutcomes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		err       error
		wantPull  string
		wantReady string
	}{
		{
			name:      "timeout",
			err:       context.DeadlineExceeded,
			wantPull:  managedPullOutcomeTimedOut,
			wantReady: managedReadinessFailed,
		},
		{
			name:      "unsupported runtime",
			err:       apisurface.ErrPullUnsupported,
			wantPull:  managedPullOutcomeUnsupportedRuntime,
			wantReady: managedReadinessUnsupported,
		},
		{
			name:      "source fetch sentinel",
			err:       fmt.Errorf("manifest lookup: %w", apisurface.ErrSourceFetchFailed),
			wantPull:  managedPullOutcomeSourceFetchFailed,
			wantReady: managedReadinessFailed,
		},
		{
			name:      "download failure message",
			err:       errors.New("download model asset \"model.gguf\" failed (502): upstream unavailable"),
			wantPull:  managedPullOutcomeSourceFetchFailed,
			wantReady: managedReadinessFailed,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pullOutcome, readiness := ClassifyPullFailure(tc.err)
			if pullOutcome != tc.wantPull || readiness != tc.wantReady {
				t.Fatalf("classified = (%s, %s), want (%s, %s)", pullOutcome, readiness, tc.wantPull, tc.wantReady)
			}
		})
	}
}

func TestEnrichPullResult_ProjectsSourceDiagnostics(t *testing.T) {
	t.Parallel()
	result := EnrichPullResult(apisurface.PullResult{
		ModelName: "OMNIVOICE_Q4_K_M",
		Outcome:   legacyPullOutcomeAlreadyPresent,
	}, RuntimeCacheInspection{Supported: true, Installed: true}, ManagedRuntimeSourceResolution{
		SourceKind:    ManagedRuntimeSourceKindManagedMirror,
		SourceID:      "managed-mirror:OMNIVOICE_Q4_K_M",
		ResolverNotes: "assets resolve through configured managed mirror source",
	})
	if result.ManagedPullOutcome != managedPullOutcomeAlreadyReady {
		t.Fatalf("managed pull outcome = %q, want ALREADY_READY", result.ManagedPullOutcome)
	}
	if result.SourceKind != ManagedRuntimeSourceKindManagedMirror || result.SourceID == "" {
		t.Fatalf("source diagnostics = (%q, %q), want managed mirror identity", result.SourceKind, result.SourceID)
	}
}
