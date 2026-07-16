// Package providerparity provides sanitized deterministic provider transcripts
// for cross-provider CLI/API parity proofs across fidelity classes.
package providerparity

import (
	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"
	"github.com/portpowered/infinite-you/pkg/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
)

// FidelityClass names one adapter streaming fidelity lane exercised by parity proofs.
type FidelityClass string

const (
	FidelityFullStream    FidelityClass = "full-stream"
	FidelityPartialStream FidelityClass = "partial-stream"
	FidelitySnapshotOnly  FidelityClass = "snapshot-only"
	FidelityFinalOnly     FidelityClass = "final-only"
)

const (
	FixtureFullStreamClaude     = "full-stream-claude"
	FixturePartialStreamCodex   = "partial-stream-codex"
	FixtureSnapshotOnlyOpenCode = "snapshot-only-opencode"
	FixtureFinalOnlyOpenCode    = "final-only-opencode"
	FixtureToolLifecycleClaude  = "tool-lifecycle-claude"
	FixtureAgyFinalOnly         = "agy-final-only"
)

// Fixture describes one sanitized parity transcript and its expected terminal outcome.
type Fixture struct {
	ID               string
	FidelityClass    FidelityClass
	Provider         adapter.Identity
	TranscriptFile   string
	Request          workerexecution.ProviderInferenceRequest
	WantContent      string
	WantCapabilities adapter.Capabilities
	ToolLifecycle    bool
	AgyFinalOnly     bool
}

// Catalog returns the canonical parity fixture matrix for Batch 09 proofs.
func Catalog() []Fixture {
	return []Fixture{
		{
			ID:             FixtureFullStreamClaude,
			FidelityClass:  FidelityFullStream,
			Provider:       adapter.Identity(modelprovider.Claude),
			TranscriptFile: "testdata/full_stream_claude.jsonl",
			Request: workerexecution.ProviderInferenceRequest{
				Dispatch: work.WorkDispatch{DispatchID: "dispatch-parity-full-stream"},
				Model:    "claude-sonnet-4", UserMessage: "parity fixture prompt",
			},
			WantContent: "Parity hello world",
			WantCapabilities: adapter.Capabilities{
				NativeStreaming: true, MessageDeltas: true, MessageSnapshots: true,
				ToolLifecycle: true, ToolOutputDeltas: true, StableItemIDs: true,
			},
		},
		{
			ID:             FixturePartialStreamCodex,
			FidelityClass:  FidelityPartialStream,
			Provider:       adapter.Identity(modelprovider.Codex),
			TranscriptFile: "testdata/partial_stream_codex.jsonl",
			Request: workerexecution.ProviderInferenceRequest{
				Dispatch: work.WorkDispatch{DispatchID: "dispatch-parity-partial-stream"},
				Model:    "gpt-test", UserMessage: "parity fixture prompt",
			},
			WantContent: "Parity codex answer",
			WantCapabilities: adapter.Capabilities{
				NativeStreaming: true, MessageSnapshots: true, ReasoningSummaries: true,
				ToolLifecycle: true, ToolOutputDeltas: true, StableItemIDs: true,
			},
		},
		{
			ID:             FixtureSnapshotOnlyOpenCode,
			FidelityClass:  FidelitySnapshotOnly,
			Provider:       adapter.Identity(modelprovider.OpenCode),
			TranscriptFile: "testdata/snapshot_only_opencode.jsonl",
			Request: workerexecution.ProviderInferenceRequest{
				Dispatch: work.WorkDispatch{DispatchID: "dispatch-parity-snapshot-only"},
				Model:    "openai/gpt-5", UserMessage: "parity fixture prompt",
			},
			WantContent: "Parity snapshot answer",
			WantCapabilities: adapter.Capabilities{
				NativeStreaming: true, MessageSnapshots: true, StableItemIDs: true,
			},
		},
		{
			ID:             FixtureFinalOnlyOpenCode,
			FidelityClass:  FidelityFinalOnly,
			Provider:       adapter.Identity(modelprovider.OpenCode),
			TranscriptFile: "testdata/final_only_opencode.txt",
			Request: workerexecution.ProviderInferenceRequest{
				Dispatch: work.WorkDispatch{DispatchID: "dispatch-parity-final-only"},
				Model:    "openai/gpt-5", UserMessage: "parity fixture prompt",
			},
			WantContent: "Parity final-only answer",
			WantCapabilities: adapter.Capabilities{
				MessageSnapshots: true, FinalOnly: true,
			},
		},
		{
			ID:             FixtureToolLifecycleClaude,
			FidelityClass:  FidelityFullStream,
			Provider:       adapter.Identity(modelprovider.Claude),
			TranscriptFile: "testdata/tool_lifecycle_claude.jsonl",
			Request: workerexecution.ProviderInferenceRequest{
				Dispatch: work.WorkDispatch{DispatchID: "dispatch-parity-tool-lifecycle"},
				Model:    "claude-sonnet-4", UserMessage: "parity fixture prompt",
			},
			WantContent: "Parity tool lifecycle complete",
			WantCapabilities: adapter.Capabilities{
				NativeStreaming: true, MessageDeltas: true, MessageSnapshots: true,
				ToolLifecycle: true, ToolOutputDeltas: true, StableItemIDs: true,
			},
			ToolLifecycle: true,
		},
		{
			ID:             FixtureAgyFinalOnly,
			FidelityClass:  FidelityFinalOnly,
			Provider:       adapter.Identity(modelprovider.Agy),
			TranscriptFile: "testdata/agy_final_only.txt",
			Request: workerexecution.ProviderInferenceRequest{
				Dispatch: work.WorkDispatch{DispatchID: "dispatch-parity-agy-final-only"},
				Model:    "gemini-pro", UserMessage: "parity fixture prompt",
			},
			WantContent: "Parity agy complete response",
			WantCapabilities: adapter.Capabilities{
				MessageSnapshots: true, FinalOnly: true,
			},
			AgyFinalOnly: true,
		},
	}
}

// FixtureByID returns one catalog fixture or false when id is unknown.
func FixtureByID(id string) (Fixture, bool) {
	for _, fixture := range Catalog() {
		if fixture.ID == id {
			return fixture, true
		}
	}
	return Fixture{}, false
}
