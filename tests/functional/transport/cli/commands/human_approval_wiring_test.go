package commands_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func testCLIWorkApprovalListAndShowExposePendingApprovalAndSafeEmptyErrors(t *testing.T, remote *sharedRemoteCLI) {
	factoryDir := support.ScaffoldFactory(t, humanApprovalCLIWorkFactoryConfig())
	sessionID := remote.openSession(t, factoryDir)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	emptyOut, err := remote.run(ctx, factoryDir, sessionID, "--json", "work", "approval", "list")
	if err != nil {
		t.Fatalf("empty you work approval list: %v\noutput:\n%s", err, emptyOut)
	}
	var empty factoryapi.ListHumanApprovalsResponse
	if err := json.Unmarshal(bytesTrimSpace(emptyOut), &empty); err != nil {
		t.Fatalf("decode empty approval list: %v\noutput:\n%s", err, emptyOut)
	}
	if len(empty.Approvals) != 0 {
		t.Fatalf("empty approval list = %#v, want no pending approvals", empty.Approvals)
	}

	unknownOut, err := remote.run(ctx, factoryDir, sessionID, "--json", "work", "approval", "show", "approval-does-not-exist")
	if err == nil {
		t.Fatalf("unknown approval unexpectedly succeeded: %s", unknownOut)
	}
	if strings.Contains(string(unknownOut), "release") || strings.Contains(string(unknownOut), "secret") {
		t.Fatalf("unknown approval output leaked Work content: %s", unknownOut)
	}
	invalidOut, err := remote.run(ctx, factoryDir, sessionID, "--json", "work", "approval", "show")
	if err == nil {
		t.Fatalf("approval show without an ID unexpectedly succeeded: %s", invalidOut)
	}
	if strings.Contains(string(invalidOut), "release") || strings.Contains(string(invalidOut), "secret") {
		t.Fatalf("argument error leaked Work content: %s", invalidOut)
	}
	remote.assertHealthy(t, remote.hostFactoryDir)
}

func humanApprovalCLIWorkFactoryConfig() map[string]any {
	return map[string]any{
		"name": "cli-human-approval-wiring",
		"workTypes": []map[string]any{{
			"name":   "task",
			"states": []map[string]string{{"name": "init", "type": "INITIAL"}, {"name": "approved", "type": "TERMINAL"}, {"name": "rejected", "type": "TERMINAL"}},
		}},
		"workstations": []map[string]any{{
			"id": "release-approval", "name": "Release Approval", "type": "HUMAN_APPROVAL",
			"description": map[string]any{"type": "LOCALIZABLE_ASSET", "value": "Approve the release"},
			"inputs":      []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":     []map[string]string{{"workType": "task", "state": "approved"}},
			"onRejection": []map[string]string{{"workType": "task", "state": "rejected"}},
		}},
	}
}
