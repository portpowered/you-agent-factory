package wire

import (
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/work/internal/invocationreturnpolicy"
	policywire "github.com/portpowered/infinite-you/pkg/services/work/internal/invocationreturnpolicy/wire"
	"github.com/portpowered/infinite-you/pkg/services/work/internal/requestadmission"
	admissionwire "github.com/portpowered/infinite-you/pkg/services/work/internal/requestadmission/wire"
	invocationwire "github.com/portpowered/infinite-you/pkg/services/work/internal/services/invocation_preparation/wire"
	requestwire "github.com/portpowered/infinite-you/pkg/services/work/internal/services/request_preparation/wire"
	stateaccess "github.com/portpowered/infinite-you/pkg/services/work/internal/services/state_access"
	statewire "github.com/portpowered/infinite-you/pkg/services/work/internal/services/state_access/wire"
)

type InvocationInputPolicy = invocationreturnpolicy.InvocationInputPreparation

func NewInvocationInputPolicy(readFile work.SubmittedFileReader, inspectPath work.SubmittedFilePathInspector) InvocationInputPolicy {
	return policywire.NewInvocationInputPolicy(invocationreturnpolicy.InvocationInputFileReader(readFile), invocationreturnpolicy.InvocationInputPathInspector(inspectPath))
}
func NewInvocationInputAdapter(inner InvocationInputPolicy) work.InvocationInputPreparation {
	return invocationwire.NewInvocationInputAdapter(inner)
}

type ContentPolicy = requestadmission.ContentPreparation
type RequestContentBridge interface {
	requestadmission.ContentPreparation
}
type RequestPolicy = requestadmission.RequestPreparationService

func NewContentPolicy() ContentPolicy { return admissionwire.NewContentPolicy() }
func NewContentPreparation(inner ContentPolicy) work.ContentPreparation {
	return requestwire.NewContentPreparation(inner)
}
func NewRequestContentBridge(content work.ContentPreparation) RequestContentBridge {
	return requestwire.NewRequestContentBridge(content)
}
func NewRequestPolicy(content RequestContentBridge) RequestPolicy {
	return admissionwire.NewRequestPolicy(content)
}
func NewRequestPreparationService(inner RequestPolicy) work.RequestPreparationService {
	return requestwire.NewRequestPreparationService(inner)
}

type StateAccess = stateaccess.Service
type RuntimeSessionResolver = stateaccess.SessionResolver
type SnapshotReader = stateaccess.SnapshotReader

func NewRuntimeSessionResolver(runtimes work.RuntimeResolver) RuntimeSessionResolver {
	return statewire.NewRuntimeSessionResolver(runtimes)
}
func NewStateAccess(sessions RuntimeSessionResolver, snapshots SnapshotReader, durability work.CompletedFlushSequenceReader) StateAccess {
	return statewire.NewService(sessions, snapshots, durability)
}
