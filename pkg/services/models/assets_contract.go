package models

import (
	"errors"
	"fmt"
)

// ErrNotAvailable reports that a discovered local model exists but its
// required local assets are not present in the managed cache.
var ErrNotAvailable = errors.New("model not available")

// ErrPullUnsupported reports that the requested model does not support
// managed local asset pulls in the current runtime or platform.
var ErrPullUnsupported = errors.New("model pull is not supported")

// ErrSourceFetchFailed reports that required managed runtime assets could not
// be fetched from the configured backend source.
var ErrSourceFetchFailed = errors.New("managed runtime source fetch failed")

// DownloadedFile describes one cached artifact materialized by a managed
// local-model asset pull.
type DownloadedFile struct {
	Path   string
	Bytes  int64
	SHA256 string
}

// PullResult carries the model-owned outcome of pulling one model into the
// managed local cache. Transport packages map it to public response contracts.
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
