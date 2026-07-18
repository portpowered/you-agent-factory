package smoke

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

const javascriptFactoryRunTimeout = 30 * time.Second

func TestJavaScriptFactoryRun_RealCLIUsesMockWorkersAndReturnsPrimaryResult(t *testing.T) {
	t.Parallel()

	workingRoot := support.LegacyFixtureDir(t, "dynamic")
	isolatedRoot := testutil.CopyFixtureDir(t, workingRoot)
	binaryPath := buildYouCLIBinary(t)
	mockWorkersPath := writeDefaultMockWorkersConfig(t)

	port, err := reserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve loopback port: %v", err)
	}
	serverURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	ctx, cancel := context.WithTimeout(context.Background(), javascriptFactoryRunTimeout)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(
		ctx,
		binaryPath,
		"--json",
		"run",
		"--factory", "./basic.js",
		"--with-mock-workers",
		"--no-record",
		"--server", serverURL,
		mockWorkersPath,
	)
	cmd.Dir = isolatedRoot
	cmd.Env = javascriptFactoryRunEnvironment(t)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("you run --factory timed out after %s: %v\nstdout:\n%s\nstderr:\n%s", javascriptFactoryRunTimeout, ctx.Err(), stdout.String(), stderr.String())
	}
	if runErr != nil {
		t.Fatalf("you run --factory exited non-zero: %v\nstdout:\n%s\nstderr:\n%s", runErr, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty stderr on successful JSON invocation", stderr.String())
	}

	result := decodeSingleJavaScriptFactoryRunResult(t, stdout.String())
	if result.SyncOutcome != factoryapi.FactorySessionSyncExecutionOutcomeCompleted ||
		result.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("terminal outcome = %s (%s), want exactly one COMPLETED (SUCCEEDED) outcome", result.SyncOutcome, result.Status)
	}
	if result.Result == nil || result.Result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("result = %#v, want one FINAL Factory Session result", result.Result)
	}
	if result.Result.PrimaryResult == nil || len(*result.Result.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want exactly one content part", result.Result.PrimaryResult)
	}
	part, err := (*result.Result.PrimaryResult)[0].AsWorkJsonContentPart()
	if err != nil {
		t.Fatalf("decode primary result content part: %v", err)
	}
	if got, ok := part.Json.(string); !ok || got != "<SUCCESS>" {
		t.Fatalf("primary result = %#v, want exact string %q", part.Json, "<SUCCESS>")
	}
	if result.EffectivePolicy == nil || result.EffectivePolicy.AdditionalProperties["allowNetwork"] != false {
		t.Fatalf("effective policy = %#v, want public network disabled", result.EffectivePolicy)
	}

	functionalevidence.Covers(t, "cli/you.run")
}

func decodeSingleJavaScriptFactoryRunResult(t *testing.T, stdout string) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()

	decoder := json.NewDecoder(strings.NewReader(stdout))
	var result factoryapi.FactorySessionSyncExecutionResponse
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("decode CLI stdout as Factory Session result: %v\nstdout:\n%s", err, stdout)
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("stdout contained more than one terminal JSON result: %v\nstdout:\n%s", err, stdout)
	}
	return result
}

func javascriptFactoryRunEnvironment(t *testing.T) []string {
	t.Helper()

	isolatedHome := t.TempDir()
	environment := make([]string, 0, len(os.Environ())+3)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		upperName := strings.ToUpper(name)
		if strings.EqualFold(name, "HOME") || strings.EqualFold(name, "USERPROFILE") ||
			strings.HasSuffix(upperName, "_API_KEY") || strings.HasSuffix(upperName, "_AUTH_TOKEN") ||
			strings.HasPrefix(upperName, "AWS_") {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment,
		"HOME="+isolatedHome,
		"USERPROFILE="+isolatedHome,
		"XDG_CONFIG_HOME="+filepath.Join(isolatedHome, ".config"),
	)
}
