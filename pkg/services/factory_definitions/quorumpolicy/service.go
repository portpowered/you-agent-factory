// Package quorumpolicy is a transitional re-export surface for packaged Quorum
// identity and lineage policy. Implementation is owned by nested
// internal/services/invocation_policy/quorumpolicy; deletion is deferred to DEL packets.
package quorumpolicy

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	invocationpolicyquorum "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/quorumpolicy"
)

func NewService() factorydefinitions.QuorumPolicyService {
	return invocationpolicyquorum.NewService()
}

var (
	IsPackagedQuorumFactory = invocationpolicyquorum.IsPackagedQuorumFactory
	WorkRelations           = invocationpolicyquorum.WorkRelations
)
