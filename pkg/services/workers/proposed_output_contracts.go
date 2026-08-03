package workers

import (
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// ProposedOutput carries Worker-proposed content. Canonical Work materialization
// remains Work-owned.
type ProposedOutput struct {
	Primary        []work.WorkContentPart
	Feedback       string
	Classification string
	ProposedWork   []ProposedWork
	ArtifactRefs   []ArtifactRef
}

// ProposedWork is a non-canonical follow-up Work proposal.
type ProposedWork struct {
	WorkTypeID string
	Name       string
	State      string
	Content    []work.WorkContentPart
	Tags       map[string]string
	Relations  []work.Relation
}

// ArtifactRef is a detached artifact identity Workers observed or produced.
type ArtifactRef struct {
	ArtifactID string
	Label      string
	URI        string
}

// Clone returns a detached ProposedOutput copy.
func (output ProposedOutput) Clone() ProposedOutput {
	clone := output
	clone.Primary = work.CloneWorkContentParts(output.Primary)
	if len(output.ProposedWork) > 0 {
		clone.ProposedWork = make([]ProposedWork, len(output.ProposedWork))
		for i, item := range output.ProposedWork {
			clone.ProposedWork[i] = item.Clone()
		}
	}
	if len(output.ArtifactRefs) > 0 {
		clone.ArtifactRefs = append([]ArtifactRef(nil), output.ArtifactRefs...)
	}
	return clone
}

// Clone returns a detached ProposedWork copy.
func (item ProposedWork) Clone() ProposedWork {
	clone := item
	clone.Content = work.CloneWorkContentParts(item.Content)
	clone.Tags = cloneStringMap(item.Tags)
	if len(item.Relations) > 0 {
		clone.Relations = append([]work.Relation(nil), item.Relations...)
	}
	return clone
}
