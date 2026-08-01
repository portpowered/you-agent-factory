// Package managedruntime retains same-service compatibility names while the
// canonical managed-runtime vocabulary lives at pkg/services/models.
package managedruntime

import models "github.com/portpowered/infinite-you/pkg/services/models"

var (
	ErrNotFound    = models.ErrNotFound
	ErrMissing     = models.ErrMissing
	ErrLoading     = models.ErrLoading
	ErrFailed      = models.ErrFailed
	ErrUnsupported = models.ErrUnsupported
)

type ReadinessState = models.ReadinessState

const (
	ReadinessStateReady       = models.ReadinessStateReady
	ReadinessStateMissing     = models.ReadinessStateMissing
	ReadinessStateLoading     = models.ReadinessStateLoading
	ReadinessStateFailed      = models.ReadinessStateFailed
	ReadinessStateUnsupported = models.ReadinessStateUnsupported
)

type LifecycleState = models.LifecycleState

const (
	LifecycleStateNotInstalled  = models.LifecycleStateNotInstalled
	LifecycleStateInstalling    = models.LifecycleStateInstalling
	LifecycleStateInstalled     = models.LifecycleStateInstalled
	LifecycleStateLoading       = models.LifecycleStateLoading
	LifecycleStateLoaded        = models.LifecycleStateLoaded
	LifecycleStateNotApplicable = models.LifecycleStateNotApplicable
)

type PullOutcome = models.PullOutcome

const (
	PullOutcomeAlreadyPresent        = models.PullOutcomeAlreadyPresent
	PullOutcomeAlreadyReady          = models.PullOutcomeAlreadyReady
	PullOutcomeInstalledSuccessfully = models.PullOutcomeInstalledSuccessfully
	PullOutcomeSourceFetchFailed     = models.PullOutcomeSourceFetchFailed
	PullOutcomeStillLoading          = models.PullOutcomeStillLoading
	PullOutcomeTimedOut              = models.PullOutcomeTimedOut
	PullOutcomeUnsupportedRuntime    = models.PullOutcomeUnsupportedRuntime
)

type Locality = models.Locality

const (
	LocalityLocal = models.LocalityLocal
	LocalityCloud = models.LocalityCloud
)

type Operation = models.Operation
type OperationSlot = models.OperationSlot
type Runtime = models.Runtime
type InvocationError = models.InvocationError
