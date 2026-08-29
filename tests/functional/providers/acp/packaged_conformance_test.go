package acp_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	modelproviders "github.com/portpowered/infinite-you/packages/model-providers"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type packagedACPConformanceCatalog struct {
	ACP []packagedACPConformanceEntry `json:"acp"`
}

type packagedACPConformanceEntry struct {
	Name      string   `json:"name"`
	Arguments []string `json:"arguments"`
	Command   string   `json:"command"`
}

type packagedACPInitializeFixture struct {
	Protocol        string `json:"protocol"`
	ProtocolVersion string `json:"protocolVersion"`
	Provider        string `json:"provider"`
	Fixture         string `json:"fixture"`
}

// TestPackagedACPProfilesUseSharedConformanceBehavior exercises every
// package-owned ACP launch projection through the customer process. The
// package profiles intentionally share one typed ACP implementation today, so
// one sanitized fixture covers model selection, session identity, and
// normalized response fidelity for every package-owned command. Attachment
// mapping remains covered by the focused cursor ACP resource fixture because
// the other packaged profiles conservatively declare image input unsupported.
// Isolation: isolated-with-reason - packaged command projection; the runtime
// portion must launch the exact allowlisted executable boundary. The profile
// fixture checks remain root-free shareable asset evidence.
func TestPackagedACPProfilesUseSharedConformanceBehavior(t *testing.T) {
	var catalog packagedACPConformanceCatalog
	if err := json.Unmarshal(modelproviders.RuntimeACPJSON(), &catalog); err != nil {
		t.Fatalf("decode generated ACP runtime catalog: %v", err)
	}
	if len(catalog.ACP) != 20 {
		t.Fatalf("generated ACP runtime profile count = %d, want 20", len(catalog.ACP))
	}

	for _, entry := range catalog.ACP {
		entry := entry
		t.Run(entry.Name, func(t *testing.T) {
			fixture := readPackagedACPInitializeFixture(t, entry.Name)
			if fixture.Provider != entry.Name || fixture.Protocol != "acp" || fixture.ProtocolVersion != "1" || fixture.Fixture != "initialize-conformance" {
				t.Fatalf("package initialize fixture = %#v, want ACP v1 fixture for %q", fixture, entry.Name)
			}
		})
	}

	// Every generated profile above is schema-checked against its own pinned
	// fixture. Run the shared ACP implementation once: repeating the identical
	// process/session lifecycle for all 20 command projections adds no distinct
	// behavioral evidence and made this one test dominate the package runtime.
	entry := catalog.ACP[0]
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{Name: "packaged ACP conformance", WorkTypeID: "task"})
	writeACPWorker(t, dir, entry.Name)
	release := filepath.Join(t.TempDir(), "acp-peer-release")
	fixture := functionalACPFixture("package-conformance")
	fixture.PackageConformanceReleasePath = release

	var starts atomic.Int32
	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservationsStableBeforeClose(t, dir, serviceedges.Edges{
		PlatformProcessCommandFactory: packagedACPCommandFactory(catalog.ACP, &starts, fixture),
		ProvidersExecutableLocator:    availableExecutableLocator{},
	}, 20*time.Second, func() {
		if err := os.WriteFile(release, []byte("completed Work observed"), 0o600); err != nil {
			t.Fatalf("release packaged ACP peer: %v", err)
		}
	})
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1; %s", got, packagedACPConformanceDiagnostics(t, events, entry.Name))
	}
	if got := starts.Load(); got != 1 {
		t.Fatalf("%s ACP process starts = %d, want 1", entry.Name, got)
	}
	assertProviderSessionID(t, events, entry.Name, "acp-session-functional-1")
	assertPackagedACPResponse(t, events, entry.Name)
}

func readPackagedACPInitializeFixture(t *testing.T, providerID string) packagedACPInitializeFixture {
	t.Helper()
	path := testutil.MustRepoPath(t, "packages/model-providers/providers/"+providerID+"/testdata/initialize.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s initialize fixture: %v", providerID, err)
	}
	var fixture packagedACPInitializeFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode %s initialize fixture: %v", providerID, err)
	}
	return fixture
}

func packagedACPCommandFactory(
	entries []packagedACPConformanceEntry,
	starts *atomic.Int32,
	fixture acpFixtureConfig,
) platformprocess.CommandFactory {
	return func(name string, args ...string) *exec.Cmd {
		for _, entry := range entries {
			parts := strings.Fields(entry.Command)
			if len(parts) == 0 || parts[0] != name || !sameStringSlice(args, entry.Arguments) {
				continue
			}
			starts.Add(1)
			return exec.Command(os.Args[0], acpFixtureChildArgs("TestACPAgentHelperProcess", fixture)...)
		}
		// The generated projection is the complete allowlist for this fixture.
		// An unexpected command must fail inside the test binary rather than
		// launching an ambient provider installed on the developer machine.
		return exec.Command(os.Args[0], "-test.run=^TestPackagedACPUnexpectedCommand$")
	}
}

func assertPackagedACPResponse(t *testing.T, events []factoryapi.FactoryEvent, providerID string) {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeModelResponse {
			continue
		}
		payload, err := event.Payload.AsModelResponseEventPayload()
		if err != nil {
			t.Fatalf("decode %s model response: %v", providerID, err)
		}
		if payload.ProviderSession == nil || payload.ProviderSession.Provider == nil || *payload.ProviderSession.Provider != providerID {
			continue
		}
		observation, err := support.AsInferenceResponseObservation(event)
		if err != nil {
			t.Fatalf("decode %s normalized response: %v", providerID, err)
		}
		if observation.Outcome != factoryapi.InferenceOutcomeSucceeded {
			t.Fatalf("%s response outcome = %q, want succeeded", providerID, observation.Outcome)
		}
		if observation.Response == nil || !strings.Contains(*observation.Response, "execution COMPLETE") {
			t.Fatalf("%s normalized response = %#v, want completion text", providerID, observation.Response)
		}
		return
	}
	t.Fatalf("%s omitted a terminal model response", providerID)
}

func packagedACPConformanceDiagnostics(t *testing.T, events []factoryapi.FactoryEvent, providerID string) string {
	t.Helper()
	var modelResponses []string
	var dispatchResponses []string
	for _, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeModelResponse:
			payload, err := event.Payload.AsModelResponseEventPayload()
			if err != nil {
				modelResponses = append(modelResponses, fmt.Sprintf("model-response[%s] decode=%v", event.Id, err))
				continue
			}
			if payload.ProviderSession != nil && payload.ProviderSession.Provider != nil && *payload.ProviderSession.Provider != providerID {
				continue
			}
			detail := fmt.Sprintf(
				"model-response[%s] outcome=%q attempt=%d session=%s",
				event.Id,
				payload.Outcome,
				payload.Attempt,
				packagedACPProviderSession(payload.ProviderSession),
			)
			if payload.FailureDetail != nil {
				detail += fmt.Sprintf(" failure=%s:%q", payload.FailureDetail.Reason, payload.FailureDetail.Message)
			}
			modelResponses = append(modelResponses, detail)
		case factoryapi.FactoryEventTypeDispatchResponse:
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil {
				dispatchResponses = append(dispatchResponses, fmt.Sprintf("dispatch-response[%s] decode=%v", event.Id, err))
				continue
			}
			detail := fmt.Sprintf("dispatch-response[%s] outcome=%q transition=%q", event.Id, payload.Outcome, payload.TransitionId)
			if payload.FailureDetail != nil {
				detail += fmt.Sprintf(" failure=%s:%q", payload.FailureDetail.Reason, payload.FailureDetail.Message)
			}
			dispatchResponses = append(dispatchResponses, detail)
		}
	}
	if len(modelResponses) == 0 && len(dispatchResponses) == 0 {
		return fmt.Sprintf("ACP model-response/session diagnostics unavailable for %q (factory events=%d)", providerID, len(events))
	}
	parts := []string{}
	if len(modelResponses) > 0 {
		parts = append(parts, "ACP model-response/session diagnostics: "+strings.Join(modelResponses, "; "))
	}
	if len(dispatchResponses) > 0 {
		parts = append(parts, "dispatch diagnostics: "+strings.Join(dispatchResponses, "; "))
	}
	return strings.Join(parts, "; ")
}

func packagedACPProviderSession(session *factoryapi.ProviderSessionMetadata) string {
	if session == nil {
		return "<none>"
	}
	provider := "<unknown>"
	if session.Provider != nil {
		provider = *session.Provider
	}
	identifier := "<unknown>"
	if session.Id != nil {
		identifier = *session.Id
	}
	return provider + "/" + identifier
}

func sameStringSlice(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// Isolation: isolated-with-reason - helper/process command boundary; this
// child is an inert guard for an unexpected packaged executable projection.
func TestPackagedACPUnexpectedCommand(t *testing.T) {}
