package dispatch

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const (
	sharedPetriProcessName          = "shared-package-process"
	isolatedPetriPanicProcessName   = "isolated-executor-panic-process"
	expectedSharedPetriScenarioRows = 43
	expectedPetriScenarioRows       = 44
)

// petriDispatchRuntimeLedger is an evidence ledger for the package's public
// process/session lifecycle. It deliberately records only stable identities,
// not Work payloads or command output, so the emitted matrix is safe to retain
// as verification evidence.
type petriDispatchRuntimeLedger struct {
	mu        sync.Mutex
	processes map[string]*petriDispatchProcessRow
	scenarios map[string]*petriDispatchScenarioRow
}

type petriDispatchProcessRow struct {
	Process string `json:"process"`
	Starts  int    `json:"starts"`
	Stops   int    `json:"stops"`
}

type petriDispatchScenarioRow struct {
	Scenario    string `json:"scenario"`
	Process     string `json:"process"`
	FactoryDir  string `json:"factoryDir"`
	SessionID   string `json:"sessionId"`
	SessionOpen bool   `json:"sessionOpened"`
	SessionDone bool   `json:"sessionClosed"`
}

type petriDispatchRuntimeReport struct {
	Processes []petriDispatchProcessRow  `json:"processes"`
	Scenarios []petriDispatchScenarioRow `json:"scenarios"`
}

var dispatchRuntimeLedger = newPetriDispatchRuntimeLedger()

func newPetriDispatchRuntimeLedger() *petriDispatchRuntimeLedger {
	return &petriDispatchRuntimeLedger{
		processes: make(map[string]*petriDispatchProcessRow),
		scenarios: make(map[string]*petriDispatchScenarioRow),
	}
}

func (ledger *petriDispatchRuntimeLedger) processStarted(process string) error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	row := ledger.processes[process]
	if row == nil {
		row = &petriDispatchProcessRow{Process: process}
		ledger.processes[process] = row
	}
	limit := petriDispatchProcessOccurrenceLimit(process)
	if row.Starts >= limit {
		return fmt.Errorf(
			"Petri process %q was started more than %d times",
			process,
			limit,
		)
	}
	row.Starts++
	return nil
}

func (ledger *petriDispatchRuntimeLedger) processStopped(process string) error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	row := ledger.processes[process]
	if row == nil || row.Starts == 0 {
		return fmt.Errorf("Petri process %q stopped without a recorded start", process)
	}
	limit := petriDispatchProcessOccurrenceLimit(process)
	if row.Stops >= limit {
		return fmt.Errorf(
			"Petri process %q was stopped more than %d times",
			process,
			limit,
		)
	}
	row.Stops++
	return nil
}

func (ledger *petriDispatchRuntimeLedger) scenarioOpened(
	process,
	scenario,
	factoryDir,
	sessionID string,
) error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if strings.TrimSpace(scenario) == "" {
		return errors.New("Petri scenario name is required")
	}
	occurrences := 0
	for _, row := range ledger.scenarios {
		if row.Scenario == scenario {
			occurrences++
		}
	}
	if occurrences >= petriDispatchRepeatCount() {
		return fmt.Errorf(
			"Petri scenario %q was recorded more than %d times",
			scenario,
			petriDispatchRepeatCount(),
		)
	}
	for _, row := range ledger.scenarios {
		if row.FactoryDir == factoryDir {
			return fmt.Errorf("Petri Factory directory %q was reused", factoryDir)
		}
		if sessionID != "" && row.SessionID == sessionID {
			return fmt.Errorf("Petri Factory Session %q was reused", sessionID)
		}
	}
	rowKey := scenario
	if occurrences > 0 {
		rowKey = fmt.Sprintf("%s#%d", scenario, occurrences+1)
	}
	ledger.scenarios[rowKey] = &petriDispatchScenarioRow{
		Scenario:    scenario,
		Process:     process,
		FactoryDir:  factoryDir,
		SessionID:   sessionID,
		SessionOpen: true,
	}
	return nil
}

func (ledger *petriDispatchRuntimeLedger) scenarioCompleted(scenario, sessionID string) error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	var row *petriDispatchScenarioRow
	for _, candidate := range ledger.scenarios {
		if candidate.Scenario != scenario || candidate.SessionID != "" || candidate.SessionDone {
			continue
		}
		if row != nil {
			return fmt.Errorf("Petri scenario %q has multiple incomplete rows", scenario)
		}
		row = candidate
	}
	if row == nil {
		return fmt.Errorf("Petri scenario %q was completed without an open row", scenario)
	}
	if !row.SessionOpen || row.SessionDone {
		return fmt.Errorf("Petri scenario %q was completed more than once", scenario)
	}
	if strings.TrimSpace(sessionID) == "" {
		return fmt.Errorf("Petri scenario %q completed without a Factory Session ID", scenario)
	}
	for otherScenario, otherRow := range ledger.scenarios {
		if otherScenario != scenario && otherRow.SessionID == sessionID {
			return fmt.Errorf("Petri Factory Session %q was reused by %q", sessionID, otherScenario)
		}
	}
	row.SessionID = sessionID
	row.SessionDone = true
	return nil
}

func (ledger *petriDispatchRuntimeLedger) scenarioClosed(sessionID string) error {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	for _, row := range ledger.scenarios {
		if row.SessionID != sessionID {
			continue
		}
		if !row.SessionOpen || row.SessionDone {
			return fmt.Errorf("Petri Factory Session %q was closed more than once", sessionID)
		}
		row.SessionDone = true
		return nil
	}
	return fmt.Errorf("Petri Factory Session %q was closed without a recorded row", sessionID)
}

func (ledger *petriDispatchRuntimeLedger) report() (petriDispatchRuntimeReport, error) {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()

	report := petriDispatchRuntimeReport{
		Processes: make([]petriDispatchProcessRow, 0, len(ledger.processes)),
		Scenarios: make([]petriDispatchScenarioRow, 0, len(ledger.scenarios)),
	}
	for _, row := range ledger.processes {
		report.Processes = append(report.Processes, *row)
		if row.Starts != row.Stops {
			return report, fmt.Errorf(
				"Petri process %q lifecycle starts=%d stops=%d",
				row.Process,
				row.Starts,
				row.Stops,
			)
		}
	}
	for _, row := range ledger.scenarios {
		report.Scenarios = append(report.Scenarios, *row)
		if !row.SessionDone {
			return report, fmt.Errorf("Petri scenario %q did not close its Factory Session", row.Scenario)
		}
	}
	sort.Slice(report.Processes, func(i, j int) bool {
		return report.Processes[i].Process < report.Processes[j].Process
	})
	sort.Slice(report.Scenarios, func(i, j int) bool {
		if report.Scenarios[i].Scenario != report.Scenarios[j].Scenario {
			return report.Scenarios[i].Scenario < report.Scenarios[j].Scenario
		}
		return report.Scenarios[i].FactoryDir < report.Scenarios[j].FactoryDir
	})
	return report, nil
}

func recordSharedPetriProcessStarted() error {
	return dispatchRuntimeLedger.processStarted(sharedPetriProcessName)
}

func recordSharedPetriProcessStopped() error {
	return dispatchRuntimeLedger.processStopped(sharedPetriProcessName)
}

func recordSharedPetriScenarioOpened(scenario, factoryDir, sessionID string) error {
	return dispatchRuntimeLedger.scenarioOpened(
		sharedPetriProcessName,
		scenario,
		factoryDir,
		sessionID,
	)
}

func recordSharedPetriScenarioClosed(sessionID string) error {
	return dispatchRuntimeLedger.scenarioClosed(sessionID)
}

func recordIsolatedPetriPanicProcessStarted(scenario, factoryDir string) error {
	if err := dispatchRuntimeLedger.processStarted(isolatedPetriPanicProcessName); err != nil {
		return err
	}
	return dispatchRuntimeLedger.scenarioOpened(
		isolatedPetriPanicProcessName,
		scenario,
		factoryDir,
		"",
	)
}

func recordIsolatedPetriPanicScenarioCompleted(scenario, sessionID string) error {
	if err := dispatchRuntimeLedger.scenarioCompleted(scenario, sessionID); err != nil {
		return err
	}
	return dispatchRuntimeLedger.processStopped(isolatedPetriPanicProcessName)
}

func emitPetriDispatchRuntimeReport() error {
	report, err := dispatchRuntimeLedger.report()
	encoded, marshalErr := json.Marshal(report)
	if marshalErr != nil {
		return fmt.Errorf("encode Petri dispatch runtime matrix: %w", marshalErr)
	}
	// The JSON line is consumed from `go test -json` output as external
	// evidence. It contains identities and lifecycle counts only.
	fmt.Fprintf(os.Stdout, "PETRI_DISPATCH_RUNTIME_MATRIX %s\n", encoded)
	if err != nil {
		return err
	}
	if isFullPetriDispatchRun() && len(report.Scenarios) != expectedPetriScenarioRows*petriDispatchRepeatCount() {
		return fmt.Errorf(
			"full Petri dispatch run recorded %d scenarios, want %d per count=%d",
			len(report.Scenarios),
			expectedPetriScenarioRows*petriDispatchRepeatCount(),
			petriDispatchRepeatCount(),
		)
	}
	if isFullPetriDispatchRun() {
		sharedRows := 0
		for _, row := range report.Scenarios {
			if row.Process == sharedPetriProcessName {
				sharedRows++
			}
		}
		if sharedRows != expectedSharedPetriScenarioRows*petriDispatchRepeatCount() {
			return fmt.Errorf(
				"full Petri dispatch run recorded %d shared scenarios, want %d per count=%d",
				sharedRows,
				expectedSharedPetriScenarioRows*petriDispatchRepeatCount(),
				petriDispatchRepeatCount(),
			)
		}
	}
	return err
}

func isFullPetriDispatchRun() bool {
	if isPetriDispatchDiscoveryRun() {
		return false
	}
	if runFlag := flag.Lookup("test.run"); runFlag != nil {
		return strings.TrimSpace(runFlag.Value.String()) == ""
	}
	return true
}

func isPetriDispatchDiscoveryRun() bool {
	if listFlag := flag.Lookup("test.list"); listFlag != nil {
		return strings.TrimSpace(listFlag.Value.String()) != ""
	}
	return false
}

func petriDispatchProcessOccurrenceLimit(process string) int {
	// The shared process is intentionally reused for the whole package, even
	// when -count repeats test execution. The isolated panic case owns a new
	// process for each repetition because its Go-level panic boundary cannot be
	// exercised through the shared host.
	if process == isolatedPetriPanicProcessName {
		return petriDispatchRepeatCount()
	}
	return 1
}

func petriDispatchRepeatCount() int {
	if countFlag := flag.Lookup("test.count"); countFlag != nil {
		if count, err := strconv.Atoi(strings.TrimSpace(countFlag.Value.String())); err == nil && count > 0 {
			return count
		}
	}
	return 1
}
