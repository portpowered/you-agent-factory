package acp_test

import (
	"encoding/json"
	"os"
	"os/exec"
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

			dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
			testutil.WriteSeedRequest(t, dir, work.SubmitRequest{Name: "packaged ACP conformance", WorkTypeID: "task"})
			writeACPWorker(t, dir, entry.Name)
			t.Setenv(acpHelperEnvironment, "package-conformance")

			var starts atomic.Int32
			_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
				PlatformProcessCommandFactory: packagedACPCommandFactory(catalog.ACP, &starts),
				ProvidersExecutableLocator:    availableExecutableLocator{},
			}, 20*time.Second)
			if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
				t.Fatalf("completed work = %d, want 1; events=%#v", got, events)
			}
			if got := starts.Load(); got != 1 {
				t.Fatalf("%s ACP process starts = %d, want 1", entry.Name, got)
			}
			assertProviderSessionID(t, events, entry.Name, "acp-session-functional-1")
			assertPackagedACPResponse(t, events, entry.Name)
		})
	}
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

func packagedACPCommandFactory(entries []packagedACPConformanceEntry, starts *atomic.Int32) platformprocess.CommandFactory {
	return func(name string, args ...string) *exec.Cmd {
		for _, entry := range entries {
			parts := strings.Fields(entry.Command)
			if len(parts) == 0 || parts[0] != name || !sameStringSlice(args, entry.Arguments) {
				continue
			}
			starts.Add(1)
			return exec.Command(os.Args[0], "-test.run=^TestACPAgentHelperProcess$")
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

func TestPackagedACPUnexpectedCommand(t *testing.T) {}
