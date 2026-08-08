package acpbaseline

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// ExitOperatorAction is returned when a capture cannot run until a human does
// something -- installing a CLI, or authenticating one. It is distinct from a
// genuine failure so a runbook can tell them apart.
const ExitOperatorAction = 3

// OperatorActionError reports that a human must act before capture can run.
type OperatorActionError struct{ Detail string }

func (e *OperatorActionError) Error() string { return e.Detail }

// CaptureRequest fully describes one capture run.
type CaptureRequest struct {
	// AgentCommand is the target agent's argv, e.g. "cursor-agent acp".
	AgentCommand []string
	// AgentName labels the capture.
	AgentName string
	// OutputDir receives the raw transcript and manifest.
	OutputDir string
	// Scenarios to run; empty means all embedded scenarios.
	Scenarios []string
	// StepTimeout bounds one request.
	StepTimeout time.Duration
	// Env allowlist keys forwarded to the agent, in addition to the base set.
	ExtraEnvKeys []string
}

// Manifest describes one capture so it can be re-run and read later.
type Manifest struct {
	Agent           string              `json:"agent"`
	Command         []string            `json:"command"`
	CapturedAtUTC   string              `json:"capturedAtUtc"`
	Scenarios       []string            `json:"scenarios"`
	EnvKeysProvided map[string]bool     `json:"envKeysProvided"`
	Permissions     []string            `json:"permissionChoices"`
	UnknownMethods  []string            `json:"unknownClientMethods"`
	Observation     *Observation        `json:"observation"`
	Errors          []string            `json:"errors,omitempty"`
	TranscriptFiles map[string]string   `json:"transcriptFiles"`
	StepNotes       map[string][]string `json:"stepNotes,omitempty"`
}

type fixedClock struct{ at time.Time }

func (c fixedClock) Now() time.Time { return c.at }

// Capture runs every requested scenario against one agent, recording the wire
// traffic verbatim and returning a manifest describing what was observed.
func Capture(request CaptureRequest, now func() time.Time) (*Manifest, error) {
	if len(request.AgentCommand) == 0 {
		return nil, fmt.Errorf("agent command is required")
	}
	if request.StepTimeout <= 0 {
		request.StepTimeout = 60 * time.Second
	}
	if err := preflight(request.AgentCommand); err != nil {
		return nil, err
	}

	scenarios, err := selectScenarios(request.Scenarios)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(request.OutputDir, 0o700); err != nil {
		return nil, err
	}

	manifest := &Manifest{
		Agent:           request.AgentName,
		Command:         request.AgentCommand,
		CapturedAtUTC:   now().UTC().Format(time.RFC3339),
		EnvKeysProvided: map[string]bool{},
		Observation:     newObservation(),
		TranscriptFiles: map[string]string{},
		StepNotes:       map[string][]string{},
	}
	for _, key := range append(baseEnvKeys(), request.ExtraEnvKeys...) {
		manifest.EnvKeysProvided[key] = os.Getenv(key) != ""
	}

	for _, scenario := range scenarios {
		manifest.Scenarios = append(manifest.Scenarios, scenario.Name)
		path, runErr := runScenario(request, scenario, manifest, now)
		manifest.TranscriptFiles[scenario.Name] = path
		if runErr != nil {
			manifest.Errors = append(manifest.Errors,
				fmt.Sprintf("%s: %v", scenario.Name, runErr))
		}
	}
	return manifest, nil
}

func selectScenarios(names []string) ([]Scenario, error) {
	if len(names) == 0 || (len(names) == 1 && names[0] == "all") {
		return LoadScenarios()
	}
	selected := make([]Scenario, 0, len(names))
	for _, name := range names {
		scenario, err := LookupScenario(name)
		if err != nil {
			return nil, err
		}
		selected = append(selected, scenario)
	}
	return selected, nil
}

// preflight fails with an operator-action error rather than an obscure exec
// failure when the target agent is simply not installed.
func preflight(command []string) error {
	if _, err := exec.LookPath(command[0]); err != nil {
		return &OperatorActionError{Detail: fmt.Sprintf(
			"%s is not on PATH. Install it, then re-run. "+
				"cursor-agent: see cursor.com/install, then `cursor-agent login`. "+
				"claude-code-acp: `npm i -g @zed-industries/claude-code-acp@<pinned>`.",
			command[0])}
	}
	return nil
}

// baseEnvKeys is the allowlist forwarded to a captured agent. Everything else
// is dropped, both for determinism and to reduce what a capture can leak.
func baseEnvKeys() []string {
	return []string{
		"PATH", "HOME", "USER", "SHELL", "TMPDIR",
		"CURSOR_API_KEY", "CURSOR_API_ENDPOINT",
		"ANTHROPIC_API_KEY", "ANTHROPIC_BASE_URL",
	}
}

func captureEnv(extra []string) []string {
	env := []string{"TERM=dumb", "NO_COLOR=1"}
	for _, key := range append(baseEnvKeys(), extra...) {
		if value := os.Getenv(key); value != "" {
			env = append(env, key+"="+value)
		}
	}
	return env
}

// materializeWorkspace copies the fixed fixture tree so every scenario starts
// from identical content and a captured tool call is comparable across runs.
func materializeWorkspace(parent string) (string, error) {
	root := filepath.Join(parent, "workspace")
	err := fs.WalkDir(WorkspaceFS(), "workspace", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		target := filepath.Join(parent, path)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, readErr := fs.ReadFile(WorkspaceFS(), path)
		if readErr != nil {
			return readErr
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		return "", err
	}
	return root, nil
}
