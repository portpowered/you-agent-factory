package orchestration

import "github.com/portpowered/infinite-you/pkg/services/factory_runtime/state"

type compiledBinding struct {
	kind        Kind
	petriNet    *state.Net
	javascript  *javascriptCompiledSource
}

type javascriptCompiledSource struct {
	sourceRef  string
	sourceHash string
	inline     bool
}

func (b *compiledBinding) OrchestrationKind() Kind {
	if b == nil {
		return ""
	}
	return b.kind
}

func (b *compiledBinding) isOrchestrationBinding() {}

// NewPetriBinding constructs an opaque Petri orchestration binding.
func NewPetriBinding(net *state.Net) Binding {
	return &compiledBinding{kind: KindPetri, petriNet: net}
}

// NewJavaScriptBinding constructs an opaque JavaScript orchestration binding.
func NewJavaScriptBinding(sourceRef, sourceHash string, inline bool) Binding {
	return &compiledBinding{
		kind: KindJavaScript,
		javascript: &javascriptCompiledSource{
			sourceRef:  sourceRef,
			sourceHash: sourceHash,
			inline:     inline,
		},
	}
}

// PetriNet returns the compiled Petri net for orchestration-internal execute
// paths. Peers must not depend on this accessor.
func PetriNet(binding Binding) *state.Net {
	compiled, ok := binding.(*compiledBinding)
	if !ok || compiled == nil || compiled.kind != KindPetri {
		return nil
	}
	return compiled.petriNet
}
