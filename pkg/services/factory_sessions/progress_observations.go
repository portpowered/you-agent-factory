package factorysessions

import "github.com/portpowered/infinite-you/pkg/services/workers"

const (
	ProgressFragmentKind  = workers.ProgressFragmentKind
	ResponseFragmentKind  = workers.ResponseFragmentKind
	CompletedFragmentKind = workers.CompletedFragmentKind
	FailedFragmentKind    = workers.FailedFragmentKind
)

type ProgressFragment = workers.ProgressFragment

type ProgressPublisher = workers.ProgressPublisher
