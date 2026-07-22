package factorydefinitions

import "github.com/portpowered/infinite-you/pkg/services/work"

const (
	PackagedQuorumFactoryName          = "@you/quorum"
	PackagedQuorumFactoryProject       = "builtin-quorum"
	PackagedQuorumSplitWorkstationName = "split-quorum"
	PackagedQuorumMergeWorkstationName = "merge-quorum"
)

// QuorumLineageInput is the minimal Work identity used to derive packaged
// Quorum relations without exposing runtime token implementations.
type QuorumLineageInput struct {
	WorkID     string
	WorkTypeID string
}

// QuorumPolicyService owns the fixed identity and public lineage policy of the
// built-in @you/quorum Factory.
type QuorumPolicyService interface {
	IsPackagedQuorumFactory(*FactoryConfig) bool
	WorkRelations(string, string, string, []QuorumLineageInput) []work.Relation
}
