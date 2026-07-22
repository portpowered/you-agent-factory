// Package observations retains same-service compatibility names for the
// Factory Sessions root progress-observation contract.
package observations

import factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"

const (
	ProgressFragmentKind  = factorysessions.ProgressFragmentKind
	ResponseFragmentKind  = factorysessions.ResponseFragmentKind
	CompletedFragmentKind = factorysessions.CompletedFragmentKind
	FailedFragmentKind    = factorysessions.FailedFragmentKind
)

type ProgressFragment = factorysessions.ProgressFragment
type ProgressPublisher = factorysessions.ProgressPublisher
