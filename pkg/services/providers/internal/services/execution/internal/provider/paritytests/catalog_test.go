// Package providerparity provides sanitized deterministic provider transcripts
// for cross-provider CLI/API parity proofs across fidelity classes.
package providerparity

import (
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/provider/adapter"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
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
	FixtureAgyFinalOnly = "agy-final-only"
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
			ID:             FixtureAgyFinalOnly,
			FidelityClass:  FidelityFinalOnly,
			Provider:       adapter.Identity(modelprovider.ProviderAgy),
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
