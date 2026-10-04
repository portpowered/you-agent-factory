package wire

import (
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/work/internal/invocationreturnpolicy"
	owner "github.com/portpowered/infinite-you/pkg/services/work/internal/services/invocation_preparation/internal/service"
)

func NewInvocationInputAdapter(inner invocationreturnpolicy.InvocationInputPreparation) work.InvocationInputPreparation {
	return owner.InvocationInputPreparationAdapter{Inner: inner}
}
