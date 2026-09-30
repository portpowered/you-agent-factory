package factorysession

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

const (
	mcpAdapterImportPath            = "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	mcpAdapterServiceRootImportPath = "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

type coverageMinimumManifest struct {
	Lane     string `json:"lane"`
	Packages []struct {
		Package string  `json:"package"`
		Minimum float64 `json:"minimum"`
	} `json:"packages"`
}

func TestManifestRegistration_MCPAdapterPackageIsRegistered(t *testing.T) {
	t.Helper()

	assertCoverageMinimumRegistration(t, "unit", "docs/internal/baselines/go-unit-coverage-package-minimums.json")
	assertCoverageMinimumRegistration(t, "functional", "docs/internal/baselines/go-functional-coverage-package-minimums.json")
}

func assertCoverageMinimumRegistration(t *testing.T, lane string, relativePath string) {
	t.Helper()

	data, err := os.ReadFile(testutil.MustRepoPath(t, relativePath))
	if err != nil {
		t.Fatalf("read %s coverage manifest: %v", lane, err)
	}

	var manifest coverageMinimumManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode %s coverage manifest: %v", lane, err)
	}
	if manifest.Lane != lane {
		t.Fatalf("%s coverage manifest lane = %q, want %q", relativePath, manifest.Lane, lane)
	}

	for _, entry := range manifest.Packages {
		if entry.Package != mcpAdapterServiceRootImportPath {
			continue
		}
		if entry.Minimum < 0 {
			t.Fatalf("%s coverage minimum for %q must be non-negative", lane, mcpAdapterServiceRootImportPath)
		}
		return
	}
	t.Fatalf("%s coverage manifest missing service root %q declaring %q", lane, mcpAdapterServiceRootImportPath, mcpAdapterImportPath)
}

type admissionTarget struct {
	factorysessions.Service
	started bool
}

func (target *admissionTarget) Start(context.Context, factorysessions.SessionStartRequest) (factorysessions.SessionStartResult, error) {
	target.started = true
	return factorysessions.SessionStartResult{}, nil
}

func TestBoundedSubagentCallRejectsFullCapacityWithoutStartingWork(t *testing.T) {
	slots := make(chan struct{}, 1)
	slots <- struct{}{}
	called := false
	_, err := boundedSubagentCall(context.Background(), slots, func() (int, error) {
		called = true
		return 1, nil
	})
	if !errors.Is(err, errSubagentCapacity) || called {
		t.Fatalf("full capacity returned err=%v, called=%t", err, called)
	}
}

func TestSubagentFullAdmissionCapacityDoesNotStartSession(t *testing.T) {
	for i := 0; i < cap(subagentAdmissionSlots); i++ {
		subagentAdmissionSlots <- struct{}{}
	}
	defer func() {
		for i := 0; i < cap(subagentAdmissionSlots); i++ {
			<-subagentAdmissionSlots
		}
	}()
	target := &admissionTarget{}
	response := Subagent(context.Background(), target, "", func() string { return "full-admission" }, SubagentInput{Prompt: "Do work"})
	if response.Error == nil || response.Error.Code != "factory_session.subagent.capacity_exhausted" || !response.Error.Retryable || target.started {
		t.Fatalf("full admission response = %#v, started = %t", response, target.started)
	}
	action, ok := response.Error.Details["suggestedAction"].(string)
	if !ok || !strings.Contains(action, "Wait for active calls to finish and retry") || !strings.Contains(action, "inspect or restart the MCP server") {
		t.Fatalf("capacity suggestedAction = %q", action)
	}
}
