package acpbaseline

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/wiretranscript"
)

// runScenario spawns the agent, tees both pipes into a transcript, and drives
// the scenario to completion. It returns the transcript path.
//
// The tee is the same one the ACP server uses, so a captured third party and
// our own server produce transcripts in one comparable format.
func runScenario(
	request CaptureRequest,
	scenario Scenario,
	manifest *Manifest,
	now func() time.Time,
) (string, error) {
	sandbox, err := os.MkdirTemp("", "acpbaseline-")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(sandbox) }()

	workspace, err := materializeWorkspace(sandbox)
	if err != nil {
		return "", err
	}

	transcriptPath := filepath.Join(request.OutputDir,
		fmt.Sprintf("%s.raw.jsonl", scenario.Name))
	file, err := os.OpenFile(transcriptPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	transcript := wiretranscript.NewWriter(file, fixedClock{at: now()})

	command := exec.Command(request.AgentCommand[0], request.AgentCommand[1:]...)
	command.Dir = workspace
	command.Env = captureEnv(request.ExtraEnvKeys)

	stdin, err := command.StdinPipe()
	if err != nil {
		return transcriptPath, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return transcriptPath, err
	}
	stderrPath := filepath.Join(request.OutputDir, scenario.Name+".stderr.log")
	stderrFile, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return transcriptPath, err
	}
	defer func() { _ = stderrFile.Close() }()
	command.Stderr = stderrFile

	if err := command.Start(); err != nil {
		return transcriptPath, err
	}
	defer func() {
		_ = stdin.Close()
		_ = command.Process.Kill()
		_ = command.Wait()
	}()

	conn := scenario.Name
	tapIn := wiretranscript.TeeReader(stdout, transcript, conn,
		wiretranscript.PeerAgent, wiretranscript.StreamStdout)
	tapOut := wiretranscript.TeeWriter(stdin, transcript, conn,
		wiretranscript.PeerClient, wiretranscript.StreamStdin)

	handlers := newClientHandlers(workspace)
	driver := newDriver(tapOut, tapIn, handlers, manifest.Observation)
	readDone := make(chan struct{})
	go driver.readLoop(readDone)

	runErr := driveScenario(driver, scenario, workspace, request.StepTimeout, manifest)

	permissions, unknown := handlers.snapshot()
	manifest.Permissions = append(manifest.Permissions, permissions...)
	manifest.UnknownMethods = append(manifest.UnknownMethods, unknown...)
	return transcriptPath, runErr
}

// driveScenario executes each step in order. An expect_error step records its
// rejection and continues, because an unsupported method is a finding.
func driveScenario(
	d *driver,
	scenario Scenario,
	workspace string,
	timeout time.Duration,
	manifest *Manifest,
) error {
	state := map[string]string{"cwd": workspace}

	for index, step := range scenario.Steps {
		if step.Note != "" {
			manifest.StepNotes[scenario.Name] = append(manifest.StepNotes[scenario.Name], step.Note)
		}
		switch step.Kind {
		case StepSnapshot:
			d.observation.Snapshots = append(d.observation.Snapshots, scenario.Name+"/"+step.Name)
		case StepAwait:
			wait := time.Duration(step.AwaitMillis) * time.Millisecond
			if wait <= 0 {
				wait = 500 * time.Millisecond
			}
			time.Sleep(wait)
		case StepRequest, StepExpectError:
			if err := runRequestStep(d, scenario, step, index, state, timeout); err != nil {
				return err
			}
		}
	}
	return nil
}

func runRequestStep(
	d *driver,
	scenario Scenario,
	step Step,
	index int,
	state map[string]string,
	timeout time.Duration,
) error {
	params := substitute(step.Params, state, scenario)
	reply, err := d.call(step.Method, params, timeout)
	if err != nil {
		if step.Kind == StepExpectError {
			d.observation.AgentMethodsRejected[step.Method] = 0
			return nil
		}
		return fmt.Errorf("step %d (%s): %w", index, step.Method, err)
	}

	if reply.Error != nil {
		d.observation.AgentMethodsRejected[step.Method] = reply.Error.Code
		if step.Kind == StepExpectError {
			return nil
		}
		return fmt.Errorf("step %d (%s): rpc error %d %s",
			index, step.Method, reply.Error.Code, reply.Error.Message)
	}

	d.observation.AgentMethodsAccepted[step.Method]++
	d.observation.Results[step.Method] = reply.Result
	captureState(step.Method, reply.Result, state)
	return nil
}

// captureState retains the few values later steps must reference, notably the
// session id and the first two config-option ids a model switch needs.
func captureState(method string, result json.RawMessage, state map[string]string) {
	if method != "session/new" && method != "session/load" {
		return
	}
	var response struct {
		SessionID     string `json:"sessionId"`
		ConfigOptions []struct {
			ID     string `json:"id"`
			Select *struct {
				Category string `json:"category"`
				Options  []struct {
					Value string `json:"value"`
					ID    string `json:"id"`
				} `json:"options"`
			} `json:"select"`
			Category string `json:"category"`
			Options  []struct {
				Value string `json:"value"`
				ID    string `json:"id"`
			} `json:"options"`
		} `json:"configOptions"`
	}
	if err := json.Unmarshal(result, &response); err != nil {
		return
	}
	state["sessionId"] = response.SessionID
	for _, option := range response.ConfigOptions {
		values := option.Options
		if option.Select != nil {
			values = option.Select.Options
		}
		if len(values) == 0 {
			continue
		}
		state["configId"] = option.ID
		state["configValue0"] = firstNonEmpty(values[0].Value, values[0].ID)
		last := values[len(values)-1]
		state["configValue1"] = firstNonEmpty(last.Value, last.ID)
		return
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// substitute expands ${...} placeholders a scenario uses to reference values
// discovered at run time, so scripts stay declarative without hard-coding an
// agent's ids.
func substitute(params json.RawMessage, state map[string]string, scenario Scenario) json.RawMessage {
	if len(params) == 0 {
		return params
	}
	text := string(params)
	// Replace the quoted placeholder, not the bare token: the scenario writes
	// "${clientCapabilities}" as a JSON string, and the value substituted in
	// is an object, so the surrounding quotes have to go with it.
	text = strings.ReplaceAll(text, `"${clientCapabilities}"`, clientCapabilities(scenario))
	for key, value := range state {
		encoded, err := json.Marshal(value)
		if err != nil {
			continue
		}
		text = strings.ReplaceAll(text, `"${`+key+`}"`, string(encoded))
	}
	return json.RawMessage(text)
}

func clientCapabilities(scenario Scenario) string {
	if len(scenario.ClientCapabilities) == 0 {
		return "{}"
	}
	return string(scenario.ClientCapabilities)
}
