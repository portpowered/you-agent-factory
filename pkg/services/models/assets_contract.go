package models

import (
	"errors"
	"fmt"
	"strings"
)

// ErrNotAvailable reports that a discovered local model exists but its
// required local assets are not present in the managed cache. It is distinct
// from ErrPullUnsupported and ErrSourceFetchFailed so peers can branch on
// typed pull outcomes through the root contract.
var ErrNotAvailable = errors.New("model not available")

// ErrPullUnsupported reports that the requested model does not support
// managed local asset pulls in the current runtime or platform. It is distinct
// from ErrNotAvailable and ErrSourceFetchFailed.
var ErrPullUnsupported = errors.New("model pull is not supported")

// ErrSourceFetchFailed reports that required managed runtime assets could not
// be fetched from the configured backend source. Classified pull failures may
// wrap this cause in PullError while carrying ManagedPullOutcome vocabulary.
var ErrSourceFetchFailed = errors.New("managed runtime source fetch failed")

// PullModelRequest is the plain assets pull request. Peers identify a model by
// Name without importing models/internal/assets or nested puller/cache types.
type PullModelRequest struct {
	Name string
}

// ValidatePullModelRequest checks the plain pull-model request. Empty names
// fail closed as ErrNotFound without touching nested asset-puller packages.
func ValidatePullModelRequest(request PullModelRequest) error {
	if strings.TrimSpace(request.Name) == "" {
		return fmt.Errorf("%w: empty model name", ErrNotFound)
	}
	return nil
}

// DownloadedFile describes one cached artifact materialized by a managed
// local-model asset pull. Peers consume this Models-owned vocabulary without
// nested assets puller or cache implementation types.
type DownloadedFile struct {
	Path   string
	Bytes  int64
	SHA256 string
}

// PullResult carries the Models-owned outcome of pulling one model into the
// managed local cache, including downloaded-file and pull-outcome vocabulary
// peers need. Transport packages map it to public response contracts. Asset
// operations stay on the singular root Service; peers must not import a nested
// asset-gateway interface for this slice.
type PullResult struct {
	ModelName          string
	ProviderLocality   string
	Outcome            string
	CachePath          string
	Revision           string
	DownloadedFiles    []DownloadedFile
	ManagedPullOutcome string
	ReadinessState     string
	LifecycleState     string
	SourceKind         string
	SourceID           string
	ResolverNotes      string
}

// PullError preserves a classified pull result while retaining its cause.
type PullError struct {
	Result PullResult
	Cause  error
}

func (e *PullError) Error() string {
	if e == nil {
		return ""
	}
	if e.Result.ManagedPullOutcome == "" {
		return fmt.Sprintf("managed runtime pull failed for %q", e.Result.ModelName)
	}
	return fmt.Sprintf(
		"managed runtime pull for %q failed with outcome %s (readiness %s)",
		e.Result.ModelName,
		e.Result.ManagedPullOutcome,
		e.Result.ReadinessState,
	)
}

func (e *PullError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}
