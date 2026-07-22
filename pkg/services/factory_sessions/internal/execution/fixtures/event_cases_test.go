package fixtures_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	fse "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/fixtures"
)

type eventReconnectCase struct {
	name           string
	requestID      string
	sessionID      string
	sync           bool
	wantCount      int
	wantHash       string
	reconnectAfter string
	wantAfterCount int
}

func canonicalEventReconnectCases() []eventReconnectCase {
	return []eventReconnectCase{
		{
			name:           "running",
			requestID:      "req-js-run-n-001",
			sessionID:      "dur-sess-js-run-n-001",
			wantCount:      2,
			wantHash:       "sha256:11a22ce83ca44464c5a8d90062542e6bf9f16d4350005808795b95df7e461c65",
			reconnectAfter: "session-started/dur-sess-js-run-n-001",
			wantAfterCount: 1,
		},
		{
			name:      "terminal",
			requestID: "req-js-success-002",
			sessionID: "dur-sess-js-success-002",
			wantCount: 3,
			wantHash:  "sha256:956aeb10de9e9e3a8e5ced44d32e1a15c41d770359259ad148d446611e6fce5c",
		},
		{
			name:      "dispatch-inspection",
			requestID: "req-petri-success-001",
			sessionID: "dur-sess-petri-success-001",
			sync:      true,
			wantCount: 3,
			wantHash:  "sha256:9dbb55ddc666ebae19e02b67b3eab9e0e1916241a08341949dec6d5f11f49348",
		},
		{
			name:      "artifact-inspection",
			requestID: "req-js-paused-001",
			sessionID: "dur-sess-js-paused-001",
			wantCount: 3,
			wantHash:  "sha256:caf89d9e8075003dca69a6894a214f19d20e59105fff82fd8bedebb9f89ebc85",
		},
		{
			name:      "awaiting-approval",
			requestID: "req-js-awaiting-001",
			sessionID: "dur-sess-js-awaiting-001",
			wantCount: 2,
			wantHash:  "sha256:330aaa8847dbd0ef3e40b573fbda9354fbd38b075dfb7402360d82fd617f4a40",
		},
	}
}

func runCanonicalEventReconnectCase(t *testing.T, service *fse.FakeService, tc eventReconnectCase) {
	t.Helper()
	req := fse.StartRequest{
		RequestID: tc.requestID,
		Source: fse.Source{
			Kind:      factory.WorkflowSourceKindFactoryID,
			FactoryID: "customer-support-triage",
		},
	}
	if tc.requestID == "req-js-success-002" {
		req.Source = fse.Source{
			Kind:         factory.WorkflowSourceKindWorkflowFile,
			WorkflowFile: ".claude/workflows/docs-refresh.yaml",
		}
	}
	if tc.requestID == "req-js-awaiting-001" {
		req.Source = fse.Source{
			Kind:         factory.WorkflowSourceKindWorkflowFile,
			WorkflowFile: ".claude/workflows/policy-gated-release.yaml",
		}
	}
	if tc.sync {
		if _, err := service.StartSync(context.Background(), req); err != nil {
			t.Fatalf("StartSync: %v", err)
		}
	} else if _, err := service.StartAsync(context.Background(), req); err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	all, err := service.ReadEvents(context.Background(), tc.sessionID, fse.EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(all.Events) != tc.wantCount {
		t.Fatalf("events = %d, want %d", len(all.Events), tc.wantCount)
	}
	for _, raw := range all.Events {
		assertCanonicalEventEnvelope(t, raw, "", "")
	}
	if tc.wantHash != "" {
		hash, err := fixtures.EventReadResultHash(all)
		if err != nil {
			t.Fatalf("EventReadResultHash: %v", err)
		}
		if hash != tc.wantHash {
			t.Fatalf("event hash = %q, want %q", hash, tc.wantHash)
		}
	}
	if tc.reconnectAfter == "" {
		return
	}
	after, err := service.ReadEvents(context.Background(), tc.sessionID, fse.EventReconnectRequest{
		AfterEventID: tc.reconnectAfter,
	})
	if err != nil {
		t.Fatalf("ReadEvents reconnect: %v", err)
	}
	if len(after.Events) != tc.wantAfterCount {
		t.Fatalf("reconnect events = %d, want %d", len(after.Events), tc.wantAfterCount)
	}
}
