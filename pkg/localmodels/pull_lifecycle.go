package localmodels

import (
	"context"
	"errors"
	"strings"

	"github.com/portpowered/infinite-you/pkg/apisurface"
)

const (
	legacyPullOutcomePulled         = "PULLED"
	legacyPullOutcomeAlreadyPresent = "ALREADY_PRESENT"

	managedPullOutcomeAlreadyReady          = "ALREADY_READY"
	managedPullOutcomeInstalledSuccessfully = "INSTALLED_SUCCESSFULLY"
	managedPullOutcomeAlreadyPresent        = "ALREADY_PRESENT"
	managedPullOutcomeStillLoading          = "STILL_LOADING"
	managedPullOutcomeTimedOut              = "TIMED_OUT"
	managedPullOutcomeSourceFetchFailed     = "SOURCE_FETCH_FAILED"
	managedPullOutcomeUnsupportedRuntime    = "UNSUPPORTED_RUNTIME"

	managedReadinessReady       = "READY"
	managedReadinessMissing     = "MISSING"
	managedReadinessLoading     = "LOADING"
	managedReadinessFailed      = "FAILED"
	managedReadinessUnsupported = "UNSUPPORTED"

	managedLifecycleInstalling   = "INSTALLING"
	managedLifecycleInstalled    = "INSTALLED"
	managedLifecycleNotInstalled = "NOT_INSTALLED"
)

// EnrichPullResult projects a service-owned pull result into managed-runtime
// readiness, lifecycle, and source diagnostics using post-pull cache inspection.
func EnrichPullResult(
	result apisurface.ModelPullResult,
	inspection RuntimeCacheInspection,
	resolution ManagedRuntimeSourceResolution,
) apisurface.ModelPullResult {
	outcome, readiness, lifecycle := classifySuccessfulPull(result, inspection)
	result.ManagedPullOutcome = outcome
	result.ReadinessState = readiness
	result.LifecycleState = lifecycle
	result.SourceKind = strings.TrimSpace(resolution.SourceKind)
	result.SourceID = strings.TrimSpace(resolution.SourceID)
	result.ResolverNotes = strings.TrimSpace(resolution.ResolverNotes)
	return result
}

// ClassifyPullFailure maps pull errors to managed-runtime pull outcomes and
// readiness states for logging, metrics, and stable customer-facing vocabulary.
func ClassifyPullFailure(err error) (pullOutcome string, readiness string) {
	if err == nil {
		return "", ""
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return managedPullOutcomeTimedOut, managedReadinessFailed
	case errors.Is(err, apisurface.ErrModelPullUnsupported):
		return managedPullOutcomeUnsupportedRuntime, managedReadinessUnsupported
	case errors.Is(err, apisurface.ErrManagedRuntimeSourceFetchFailed):
		return managedPullOutcomeSourceFetchFailed, managedReadinessFailed
	case isSourceFetchFailureMessage(err.Error()):
		return managedPullOutcomeSourceFetchFailed, managedReadinessFailed
	default:
		return managedPullOutcomeSourceFetchFailed, managedReadinessFailed
	}
}

func classifySuccessfulPull(result apisurface.ModelPullResult, inspection RuntimeCacheInspection) (pullOutcome, readiness, lifecycle string) {
	legacyOutcome := strings.ToUpper(strings.TrimSpace(result.Outcome))
	switch legacyOutcome {
	case legacyPullOutcomePulled:
		pullOutcome = managedPullOutcomeInstalledSuccessfully
	case legacyPullOutcomeAlreadyPresent:
		pullOutcome = managedPullOutcomeAlreadyPresent
	default:
		pullOutcome = managedPullOutcomeUnsupportedRuntime
	}

	if inspection.Supported {
		if inspection.Installed {
			readiness = managedReadinessReady
			lifecycle = managedLifecycleInstalled
			if pullOutcome == managedPullOutcomeAlreadyPresent {
				pullOutcome = managedPullOutcomeAlreadyReady
			}
			return pullOutcome, readiness, lifecycle
		}
		readiness = managedReadinessMissing
		lifecycle = managedLifecycleNotInstalled
		if pullOutcome == managedPullOutcomeInstalledSuccessfully {
			readiness = managedReadinessLoading
			lifecycle = managedLifecycleInstalling
			pullOutcome = managedPullOutcomeStillLoading
		}
		return pullOutcome, readiness, lifecycle
	}

	readiness = managedReadinessReady
	lifecycle = managedLifecycleInstalled
	if pullOutcome == managedPullOutcomeAlreadyPresent {
		pullOutcome = managedPullOutcomeAlreadyReady
	}
	return pullOutcome, readiness, lifecycle
}

func isSourceFetchFailureMessage(message string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(message))
	if trimmed == "" {
		return false
	}
	for _, fragment := range []string{
		"pull model manifest",
		"download model asset",
		"model asset request failed",
		"checksum verification",
	} {
		if strings.Contains(trimmed, fragment) {
			return true
		}
	}
	return false
}
