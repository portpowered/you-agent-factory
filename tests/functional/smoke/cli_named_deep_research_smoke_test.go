package smoke

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/packages/definitions/deepresearch"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const packagedDeepResearchFactoryName = "@you/deep-research"

func TestNamedDeepResearchCLI_DefaultInvocationReturnsLeadSynthesis(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI named @you/deep-research smoke")
	}
	topic := fmt.Sprintf("functional default deep research topic %d", time.Now().UnixNano())
	response := runNamedDeepResearchCLI(t, topic)
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("CLI status = %q, want COMPLETED", response.Status)
	}
	primary, err := json.Marshal(response.PrimaryResult)
	if err != nil {
		t.Fatalf("marshal CLI primary result: %v", err)
	}
	for _, want := range []string{topic, `"researchDepth":2`, `"maxSubagents":2`} {
		if !strings.Contains(string(primary), want) {
			t.Fatalf("CLI primary result = %q, want %q", primary, want)
		}
	}
}

func TestNamedDeepResearchCLI_InvokesConfiguredBoundedResearchWithApprovedFlags(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI named @you/deep-research smoke")
	}

	response := runNamedDeepResearchCLI(t,
		"--researchDepth", "3", "--maxSubagents", "1", "--model-provider", "CODEX", "--model", "gpt-5", "--reasoning-effort", "medium",
		fmt.Sprintf("functional deep research CLI topic %d with enough breadth for specialist delegation", time.Now().UnixNano()),
	)
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("CLI status = %q, want COMPLETED", response.Status)
	}
	primary, err := json.Marshal(response.PrimaryResult)
	if err != nil {
		t.Fatalf("marshal CLI primary result: %v", err)
	}
	text := string(primary)
	for _, want := range []string{`"researchDepth":3`, `"maxSubagents":1`, `"modelProvider":"CODEX"`, `"model":"gpt-5"`, `"reasoningEffort":"medium"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("CLI primary result = %q, want %q", text, want)
		}
	}
}

func runNamedDeepResearchCLI(t *testing.T, invocationArgs ...string) factoryapi.InvocationResponse {
	t.Helper()
	homeDir := t.TempDir()
	globalRoot := filepath.Join(homeDir, ".you-agent-factory", "you-agent-factories")
	if _, err := factoryconfig.PersistNamedFactory(globalRoot, packagedDeepResearchFactoryName, deepresearch.BuiltInFactoryJSON); err != nil {
		t.Fatalf("PersistNamedFactory(@you/deep-research): %v", err)
	}

	port, err := reserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, buildYouCLIBinary(t),
		"--json", "run", "--named", packagedDeepResearchFactoryName,
		"--with-mock-workers", "--no-record", "--server", fmt.Sprintf("http://127.0.0.1:%d", port),
		writeDefaultMockWorkersConfig(t),
		"--",
	)
	cmd.Args = append(cmd.Args, invocationArgs...)
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("you run --named %s: %v\nstdout:\n%s\nstderr:\n%s", packagedDeepResearchFactoryName, err, stdout.String(), stderr.String())
	}

	var response factoryapi.InvocationResponse
	if err := json.Unmarshal(bytes.TrimSpace([]byte(stdout.String())), &response); err != nil {
		t.Fatalf("decode CLI invocation response: %v\nstdout:\n%s", err, stdout.String())
	}
	return response
}
