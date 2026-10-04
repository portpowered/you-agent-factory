package wire

import "github.com/portpowered/infinite-you/pkg/services/work/internal/invocationreturnpolicy"

func NewInvocationInputPolicy(readFile invocationreturnpolicy.InvocationInputFileReader, inspectPath invocationreturnpolicy.InvocationInputPathInspector) invocationreturnpolicy.InvocationInputPreparation {
	return invocationreturnpolicy.NewInvocationInputPreparation(readFile, inspectPath)
}
