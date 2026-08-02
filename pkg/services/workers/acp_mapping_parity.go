package workers

import (
	"fmt"
	"sort"
	"strings"
)

// ACPMappingOutcome names the declared future ACP session-update projection
// outcome for one (Kind, Phase) response draft pair. This packet declares the
// outcome only; it implements no ACP projection, transport, or Worker Session
// behavior.
type ACPMappingOutcome string

const (
	// ACPMappingOutcomeAgentMessageChunk is the declared outcome for MESSAGE
	// drafts: projects onto ACP agent_message_chunk.
	ACPMappingOutcomeAgentMessageChunk ACPMappingOutcome = "AGENT_MESSAGE_CHUNK"
	// ACPMappingOutcomeAgentThoughtChunk is the declared outcome for
	// REASONING drafts: projects onto ACP agent_thought_chunk.
	ACPMappingOutcomeAgentThoughtChunk ACPMappingOutcome = "AGENT_THOUGHT_CHUNK"
	// ACPMappingOutcomeUsageUpdate is the declared outcome for USAGE drafts
	// carrying a meaningful primary context: projects onto ACP usage_update.
	ACPMappingOutcomeUsageUpdate ACPMappingOutcome = "USAGE_UPDATE"
	// ACPMappingOutcomeGapNotice is the declared outcome for STREAM_GAP
	// drafts: surfaced as an explicit ACP gap notice, content is never
	// fabricated to fill it.
	ACPMappingOutcomeGapNotice ACPMappingOutcome = "GAP_NOTICE"
	// ACPMappingOutcomeOutOfBandError is the declared outcome for ERROR
	// drafts. Errors surface through the JSON-RPC error channel (proposal
	// section 6.4), not the ACP SessionUpdate stream, so this is declared as
	// an explicit non-stream outcome rather than silently reusing NoOutput.
	ACPMappingOutcomeOutOfBandError ACPMappingOutcome = "OUT_OF_BAND_ERROR"
	// ACPMappingOutcomeNoOutput is the declared "no output in L1" outcome:
	// declared, not dropped, distinct from an undeclared/missing pair.
	ACPMappingOutcomeNoOutput ACPMappingOutcome = "NO_OUTPUT"
)

// WorkerDraftACPMappingOutcome names one legal (Kind, Phase) response draft
// pair together with its declared ACP mapping outcome and supporting
// evidence.
type WorkerDraftACPMappingOutcome struct {
	Kind     Kind
	Phase    Phase
	Outcome  ACPMappingOutcome
	Evidence string
}

// kindACPMappingOutcome declares the ACP mapping outcome per Kind. Phase does
// not change the projection target for any declared Kind per proposal
// section 6.2, so one entry per Kind covers every legal phase for that Kind.
var kindACPMappingOutcome = map[Kind]struct {
	Outcome  ACPMappingOutcome
	Evidence string
}{
	KindMessage: {
		Outcome:  ACPMappingOutcomeAgentMessageChunk,
		Evidence: "docs/internal/projects/acp-client/final-proposal.md section 6.2: Factory response MESSAGE -> agent_message_chunk",
	},
	KindReasoning: {
		Outcome:  ACPMappingOutcomeAgentThoughtChunk,
		Evidence: "final-proposal.md section 6.2: Factory response REASONING -> agent_thought_chunk",
	},
	KindUsage: {
		Outcome:  ACPMappingOutcomeUsageUpdate,
		Evidence: "final-proposal.md section 6.2: USAGE with meaningful primary context -> usage_update",
	},
	KindStreamGap: {
		Outcome:  ACPMappingOutcomeGapNotice,
		Evidence: "final-proposal.md section 6.2: STREAM_GAP -> surfaced as an explicit gap notice, never fabricated content",
	},
	KindError: {
		Outcome:  ACPMappingOutcomeOutOfBandError,
		Evidence: "final-proposal.md section 6.4: errors surface via the JSON-RPC error channel, not the ACP SessionUpdate stream; declared explicitly so ERROR is never conflated with an undeclared or dropped pair",
	},
	KindTool: {
		Outcome:  ACPMappingOutcomeNoOutput,
		Evidence: "final-proposal.md section 6.2: TOOL -> no output in L1 — declared, not dropped",
	},
	KindFileChange: {
		Outcome:  ACPMappingOutcomeNoOutput,
		Evidence: "final-proposal.md section 6.2: FILE_CHANGE -> no output in L1 — declared, not dropped",
	},
	KindPlan: {
		Outcome:  ACPMappingOutcomeNoOutput,
		Evidence: "final-proposal.md section 6.2: PLAN -> no output in L1 — declared, not dropped",
	},
	KindProgress: {
		Outcome:  ACPMappingOutcomeNoOutput,
		Evidence: "final-proposal.md section 6.2: PROGRESS -> no output in L1 — declared, not dropped",
	},
	KindSession: {
		Outcome:  ACPMappingOutcomeNoOutput,
		Evidence: "final-proposal.md section 6.2: SESSION -> no output in L1 — declared, not dropped",
	},
	KindRun: {
		Outcome:  ACPMappingOutcomeNoOutput,
		Evidence: "final-proposal.md section 6.2: RUN -> no output in L1 — declared, not dropped",
	},
	KindTurn: {
		Outcome:  ACPMappingOutcomeNoOutput,
		Evidence: "final-proposal.md section 6.2: TURN -> no output in L1 — declared, not dropped. Distinct from the Chat Sessions turn-terminal ACP projection (\"Factory turn terminal\" -> prompt result), which is not a workers.Draft Kind.",
	},
}

// DeclaredWorkerDraftACPMappingOutcomes returns the declared ACP mapping
// outcome for every legal (Kind, Phase) response draft pair. This packet
// implements no ACP projection; the returned outcomes are contract evidence
// only.
func DeclaredWorkerDraftACPMappingOutcomes() []WorkerDraftACPMappingOutcome {
	var out []WorkerDraftACPMappingOutcome
	for _, kind := range KnownKinds() {
		declared, ok := kindACPMappingOutcome[kind]
		if !ok {
			continue
		}
		phases, ok := AllowedPhasesForKind(kind)
		if !ok {
			continue
		}
		for _, phase := range phases {
			out = append(out, WorkerDraftACPMappingOutcome{
				Kind:     kind,
				Phase:    phase,
				Outcome:  declared.Outcome,
				Evidence: declared.Evidence,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Phase < out[j].Phase
	})
	return out
}

// WorkerDraftACPMappingPair names one legal (Kind, Phase) response draft
// pair, independent of any declared outcome.
type WorkerDraftACPMappingPair struct {
	Kind  Kind
	Phase Phase
}

// WorkerDraftACPMappingParityDrift names legal (Kind, Phase) response draft
// pairs with no declared ACP mapping outcome. Any drift means the ACP
// mapping table silently dropped a case instead of declaring an explicit
// outcome, including "no output."
type WorkerDraftACPMappingParityDrift struct {
	UndeclaredPairs []WorkerDraftACPMappingPair
}

// CompareWorkerDraftACPMappingParity returns drift between every legal
// (Kind, Phase) response draft pair (from the existing draft validation
// allow-list) and the declared ACP mapping outcome table above.
func CompareWorkerDraftACPMappingParity() WorkerDraftACPMappingParityDrift {
	declared := make(map[WorkerDraftACPMappingPair]struct{})
	for _, entry := range DeclaredWorkerDraftACPMappingOutcomes() {
		declared[WorkerDraftACPMappingPair{Kind: entry.Kind, Phase: entry.Phase}] = struct{}{}
	}

	var undeclared []WorkerDraftACPMappingPair
	for _, kind := range KnownKinds() {
		phases, ok := AllowedPhasesForKind(kind)
		if !ok {
			continue
		}
		for _, phase := range phases {
			pair := WorkerDraftACPMappingPair{Kind: kind, Phase: phase}
			if _, ok := declared[pair]; !ok {
				undeclared = append(undeclared, pair)
			}
		}
	}
	sort.Slice(undeclared, func(i, j int) bool {
		if undeclared[i].Kind != undeclared[j].Kind {
			return undeclared[i].Kind < undeclared[j].Kind
		}
		return undeclared[i].Phase < undeclared[j].Phase
	})
	return WorkerDraftACPMappingParityDrift{UndeclaredPairs: undeclared}
}

// ValidateWorkerDraftACPMappingParity fails closed when any legal (Kind,
// Phase) response draft pair lacks a declared ACP mapping outcome.
func ValidateWorkerDraftACPMappingParity() error {
	drift := CompareWorkerDraftACPMappingParity()
	if len(drift.UndeclaredPairs) == 0 {
		return nil
	}
	return drift
}

func (d WorkerDraftACPMappingParityDrift) Error() string {
	pairs := make([]string, 0, len(d.UndeclaredPairs))
	for _, pair := range d.UndeclaredPairs {
		pairs = append(pairs, fmt.Sprintf("%s/%s", pair.Kind, pair.Phase))
	}
	return fmt.Sprintf("undeclared ACP mapping outcome for response draft pairs: %s", strings.Join(pairs, ", "))
}
