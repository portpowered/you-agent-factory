package claude

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type haikuGoldenReplayCase struct {
	golden     haikuGoldenCase
	factoryDir string
	stdout     []byte
}

func prepareHaikuGoldenReplayCases(t *testing.T, manifest haikuGoldenManifest) []haikuGoldenReplayCase {
	t.Helper()
	cases := make([]haikuGoldenReplayCase, 0, len(manifest.Cases))
	for _, golden := range manifest.Cases {
		stdout := loadHaikuGoldenStdout(t, golden)
		assertHaikuGoldenNativeShape(t, stdout, golden)

		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
		support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
			modelprovider.ProviderClaude,
			golden.Selector,
		))
		support.WriteWorkstationConfig(t, dir, "process", haikuGoldenWorkstationConfig(dir))
		cases = append(cases, haikuGoldenReplayCase{
			golden:     golden,
			factoryDir: normalizeHaikuGoldenRouteDirectory(dir),
			stdout:     append([]byte(nil), stdout...),
		})
	}
	return cases
}

func haikuGoldenWorkstationConfig(factoryDir string) string {
	return fmt.Sprintf(`---
type: MODEL_WORKSTATION
workingDirectory: %s
---

Test workstation.
`, strconv.Quote(filepath.ToSlash(factoryDir)))
}

// haikuGoldenCommandRouter is constructed completely before the application
// process starts. Its directory map is immutable during dispatch; the mutex
// only protects request witnesses because the runtime owns asynchronous work.
type haikuGoldenCommandRouter struct {
	mu       sync.Mutex
	routes   map[string]haikuGoldenRoute
	requests []platformprocess.CommandRequest
	closed   bool
}

type haikuGoldenRoute struct {
	selector string
	result   platformprocess.CommandResult
}

func newHaikuGoldenCommandRouter(t *testing.T, cases []haikuGoldenReplayCase) *haikuGoldenCommandRouter {
	t.Helper()
	routes := make(map[string]haikuGoldenRoute, len(cases))
	selectors := make(map[string]struct{}, len(cases))
	for _, replayCase := range cases {
		if _, exists := routes[replayCase.factoryDir]; exists {
			t.Fatalf("duplicate Claude golden Factory directory route")
		}
		if _, exists := selectors[replayCase.golden.Selector]; exists {
			t.Fatalf("duplicate Claude golden selector route")
		}
		routes[replayCase.factoryDir] = haikuGoldenRoute{
			selector: replayCase.golden.Selector,
			result: platformprocess.CommandResult{
				Stdout: append([]byte(nil), replayCase.stdout...),
			},
		}
		selectors[replayCase.golden.Selector] = struct{}{}
	}
	return &haikuGoldenCommandRouter{routes: routes}
}

func (r *haikuGoldenCommandRouter) Run(_ context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return platformprocess.CommandResult{}, errors.New("Claude golden route is closed")
	}
	if request.Command != string(modelprovider.ProviderClaude) {
		return platformprocess.CommandResult{}, errors.New("Claude golden route rejected unexpected provider command")
	}
	route, ok := r.routes[normalizeHaikuGoldenRouteDirectory(request.WorkDir)]
	if !ok {
		return platformprocess.CommandResult{}, errors.New("Claude golden route unavailable")
	}
	if !haikuGoldenRequestSelects(request.Args, route.selector) {
		return platformprocess.CommandResult{}, errors.New("Claude golden route rejected selector")
	}
	r.requests = append(r.requests, cloneHaikuCommandRequest(request))
	return cloneHaikuCommandResult(route.result), nil
}

func haikuGoldenRequestSelects(args []string, selector string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == "--model" && args[index+1] == selector {
			return true
		}
	}
	return false
}

func (r *haikuGoldenCommandRouter) Requests() []platformprocess.CommandRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	requests := make([]platformprocess.CommandRequest, len(r.requests))
	for index, request := range r.requests {
		requests[index] = cloneHaikuCommandRequest(request)
	}
	return requests
}

func (r *haikuGoldenCommandRouter) CallsFor(factoryDir string) int {
	want := normalizeHaikuGoldenRouteDirectory(factoryDir)
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, request := range r.requests {
		if normalizeHaikuGoldenRouteDirectory(request.WorkDir) == want {
			count++
		}
	}
	return count
}

func (r *haikuGoldenCommandRouter) RequestFor(factoryDir string) (platformprocess.CommandRequest, bool) {
	want := normalizeHaikuGoldenRouteDirectory(factoryDir)
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, request := range r.requests {
		if normalizeHaikuGoldenRouteDirectory(request.WorkDir) == want {
			return cloneHaikuCommandRequest(request), true
		}
	}
	return platformprocess.CommandRequest{}, false
}

func (r *haikuGoldenCommandRouter) RouteCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.routes)
}

func (r *haikuGoldenCommandRouter) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	r.routes = nil
}

func normalizeHaikuGoldenRouteDirectory(directory string) string {
	absolute, err := filepath.Abs(directory)
	if err == nil {
		directory = absolute
	}
	return filepath.Clean(directory)
}

func cloneHaikuCommandRequest(request platformprocess.CommandRequest) platformprocess.CommandRequest {
	request.Args = append([]string(nil), request.Args...)
	request.Stdin = append([]byte(nil), request.Stdin...)
	request.Env = append([]string(nil), request.Env...)
	return request
}

func cloneHaikuCommandResult(result platformprocess.CommandResult) platformprocess.CommandResult {
	result.Stdout = append([]byte(nil), result.Stdout...)
	result.Stderr = append([]byte(nil), result.Stderr...)
	return result
}

func sessionWorkURL(baseURL, sessionID string) string {
	return strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/work"
}

var _ platformprocess.CommandRunner = (*haikuGoldenCommandRouter)(nil)
