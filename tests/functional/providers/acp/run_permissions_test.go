package acp_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// The peer verifies the client's answers before producing a successful Work
// result, so completion proves preferences and permissions need no interaction.
func TestYouRunAutomaticallyAnswersACPFormPreferences(t *testing.T) {
	t.Parallel()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"automatic form preferences"}`))
	writeACPWorker(t, dir, "cursor")
	var starts atomic.Int32
	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		PlatformProcessCommandFactory: acpHelperCommandFactory(&starts, functionalACPFixture("elicitation")),
		ProvidersExecutableLocator:    availableExecutableLocator{},
	}, 20*time.Second)
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1 after automatic schema answers; %s", got, acpFailureDiagnostics(events))
	}
}

func (p *functionalRPCPeer) assertElicitationChoices() error {
	cases := []struct{ params, result string }{
		{`{"mode":"form","message":"grant and choose preferences","requestedSchema":{"type":"object","required":["permission"],"properties":{"permission":{"type":"boolean"},"decision":{"enum":["deny","allow"]},"policy":{"oneOf":["ignored",{}, {"const":"reject"},{"const":"approve"}]},"level":{"enum":[1,2],"default":2},"fallback":{"enum":[4,5]},"fixed":{"const":"stable"},"theme":{"type":"string","default":"dark"},"optional":{"type":"string"},"malformed":"ignored","empty":{"enum":[]}}}}`, `{"action":"accept","content":{"permission":true,"decision":"allow","policy":"approve","level":2,"fallback":4,"fixed":"stable","theme":"dark"}}`},
		{`{"mode":"form","message":"boolean choices","requestedSchema":{"type":"object","properties":{"boolean":{"enum":[false,true]},"constant":{"oneOf":[{"const":3}]}}}}`, `{"action":"accept","content":{"boolean":true,"constant":3}}`},
		{`{"mode":"form","message":"missing required answer","requestedSchema":{"type":"object","required":["missing"],"properties":{}}}`, `{"action":"cancel"}`},
		{`{"mode":"form","message":"unsupported required answer","requestedSchema":{"type":"object","required":["text"],"properties":{"text":{"type":"string"}}}}`, `{"action":"cancel"}`},
		{`{"mode":"form","message":"malformed required property","requestedSchema":{"type":"object","required":["bad"],"properties":{"bad":"invalid"}}}`, `{"action":"cancel"}`},
		{`{"mode":"url","message":"external interaction","url":"https://example.invalid/authorize","elicitationId":"external"}`, `{"action":"cancel"}`},
	}
	for index, test := range cases {
		id := json.RawMessage(fmt.Sprintf(`"elicitation-%d"`, index))
		if err := p.write(rpcEnvelope{JSONRPC: "2.0", ID: id, Method: "elicitation/create", Params: json.RawMessage(test.params)}); err != nil {
			return err
		}
		if !p.scanner.Scan() {
			return fmt.Errorf("elicitation %d received no response", index)
		}
		var response rpcEnvelope
		if err := json.Unmarshal(p.scanner.Bytes(), &response); err != nil {
			return err
		}
		var got, want any
		if err := json.Unmarshal(response.Result, &got); err != nil {
			return fmt.Errorf("elicitation %d failed: %s", index, p.scanner.Bytes())
		}
		if err := json.Unmarshal([]byte(test.result), &want); err != nil {
			return err
		}
		if !reflect.DeepEqual(got, want) {
			return fmt.Errorf("elicitation %d = %s, want %s", index, response.Result, test.result)
		}
	}
	return nil
}

// Isolation: isolated-with-reason - pinned permission wire; noninteractive ACP
// turns select an advertised allow choice on a fresh peer.
func TestYouRunMapsSkipPermissionsToSDKGoldenPermissionSelection(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		skipPermissions bool
		mode            string
	}{
		{name: "default allows", mode: "permission-allow"},
		{name: "skipPermissions also allows", skipPermissions: true, mode: "permission-allow"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
			testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"golden ACP permission"}`))
			writeACPWorkerPolicy(t, dir, test.skipPermissions)
			writeGoldenSentinelWorkstation(t, dir)
			fixture := goldenACPFixture(test.mode)

			var starts atomic.Int32
			_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
				PlatformProcessCommandFactory: goldenACPCommandFactory(&starts, fixture),
				ProvidersExecutableLocator:    availableExecutableLocator{},
			}, 20*time.Second)
			if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
				t.Fatalf("completed work = %d, want 1", got)
			}
			if starts.Load() != 1 {
				t.Fatalf("ACP process starts = %d, want 1", starts.Load())
			}
		})
	}
}

func writeACPWorkerPolicy(t *testing.T, factoryDir string, skipPermissions bool) {
	t.Helper()
	value := "false"
	if skipPermissions {
		value = "true"
	}
	path := filepath.Join(factoryDir, "workers", "worker", "AGENTS.md")
	content := "---\n" +
		"executorProvider: ACP\n" +
		"modelProvider: cursor\n" +
		"model: test-model\n" +
		"skipPermissions: " + value + "\n" +
		"stopToken: COMPLETE\n" +
		"type: MODEL_WORKER\n" +
		"---\n\nTest golden ACP worker.\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write ACP worker policy: %v", err)
	}
}
