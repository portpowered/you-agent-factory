// Package acpbaseline captures reference recordings of an ACP agent's wire
// behavior and turns them into a comparable capability matrix.
//
// It exists to answer "are we correct?" with a computed diff rather than an
// opinion. To do that it drives a target agent with a raw JSON-RPC client --
// deliberately not the ACP SDK -- so an undocumented field or an unrecognized
// method survives into the transcript instead of being decoded away. The same
// scenarios drive our own `you server acp`, so a matrix row can never compare
// "what we asked them" against "what we asked ourselves".
package acpbaseline

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed scenarios/*.json
var scenarioFiles embed.FS

//go:embed workspace
var workspaceFiles embed.FS

// StepKind names what one scenario step does.
type StepKind string

const (
	// StepRequest sends a JSON-RPC request and waits for its response.
	StepRequest StepKind = "request"
	// StepExpectError sends a request that is expected to fail, recording the
	// error rather than aborting. Discovering that a method is unsupported is
	// itself baseline data.
	StepExpectError StepKind = "expect_error"
	// StepAwait waits for session/update notifications to settle.
	StepAwait StepKind = "await"
	// StepSnapshot marks a named point in the transcript.
	StepSnapshot StepKind = "snapshot"
)

// Step is one action in a scenario.
type Step struct {
	Kind   StepKind        `json:"kind"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	// Name labels a snapshot.
	Name string `json:"name,omitempty"`
	// AwaitMillis bounds an await step.
	AwaitMillis int `json:"awaitMillis,omitempty"`
	// Note explains why the step exists; it is carried into the manifest so a
	// reader of a capture knows what it was probing.
	Note string `json:"note,omitempty"`
}

// Scenario is one reproducible experiment against an agent.
type Scenario struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// ClientCapabilities is sent verbatim in initialize. Agents behave very
	// differently depending on what a client claims it can do, so this is
	// explicit per scenario rather than fixed.
	ClientCapabilities json.RawMessage `json:"clientCapabilities"`
	Steps              []Step          `json:"steps"`
}

// Validate reports whether a scenario is well formed enough to run.
func (s Scenario) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("scenario name is required")
	}
	if len(s.Steps) == 0 {
		return fmt.Errorf("scenario %q has no steps", s.Name)
	}
	for index, step := range s.Steps {
		switch step.Kind {
		case StepRequest, StepExpectError:
			if strings.TrimSpace(step.Method) == "" {
				return fmt.Errorf("scenario %q step %d: method is required", s.Name, index)
			}
		case StepAwait, StepSnapshot:
		default:
			return fmt.Errorf("scenario %q step %d: unknown kind %q", s.Name, index, step.Kind)
		}
	}
	return nil
}

// LoadScenarios returns every embedded scenario, ordered by name so a capture
// run is reproducible.
func LoadScenarios() ([]Scenario, error) {
	entries, err := fs.ReadDir(scenarioFiles, "scenarios")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	scenarios := make([]Scenario, 0, len(names))
	for _, name := range names {
		data, readErr := scenarioFiles.ReadFile("scenarios/" + name)
		if readErr != nil {
			return nil, readErr
		}
		var scenario Scenario
		if decodeErr := json.Unmarshal(data, &scenario); decodeErr != nil {
			return nil, fmt.Errorf("decode scenario %s: %w", name, decodeErr)
		}
		if validateErr := scenario.Validate(); validateErr != nil {
			return nil, validateErr
		}
		scenarios = append(scenarios, scenario)
	}
	if len(scenarios) == 0 {
		return nil, fmt.Errorf("no embedded scenarios")
	}
	return scenarios, nil
}

// LookupScenario returns one embedded scenario by name.
func LookupScenario(name string) (Scenario, error) {
	scenarios, err := LoadScenarios()
	if err != nil {
		return Scenario{}, err
	}
	for _, scenario := range scenarios {
		if scenario.Name == name {
			return scenario, nil
		}
	}
	return Scenario{}, fmt.Errorf("unknown scenario %q", name)
}

// WorkspaceFS exposes the fixed fixture tree scenarios operate on.
func WorkspaceFS() fs.FS { return workspaceFiles }
