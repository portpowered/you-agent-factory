package cli_test

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const workListFiltersCountsRequestID = "work-list-filters-counts-batch"

type workListFiltersCountsWork struct {
	Name         string
	WorkID       string
	WorkTypeName string
}

type workListFiltersCountsBatch struct {
	RequestID string                         `json:"requestId"`
	Type      string                         `json:"type"`
	Works     []workListFiltersCountsRequest `json:"works"`
}

type workListFiltersCountsRequest struct {
	Name         string            `json:"name"`
	WorkID       string            `json:"workId"`
	WorkTypeName string            `json:"workTypeName"`
	Payload      map[string]string `json:"payload"`
}

type workListFiltersCountsSubmitResponse struct {
	WorkCount int `json:"workCount"`
	Works     []struct {
		WorkID string `json:"workId"`
	} `json:"works"`
}

type workListFiltersCountsMoveResponse struct {
	WorkID   string `json:"workId"`
	NewState string `json:"newState"`
}

// TestWorkListFiltersAndCounts proves the production monitoring path through
// root.BuildProcess and Process.Execute. A single session contains every
// customer-visible state category, then the public CLI proves composition,
// failed-state terminality, zero matches, and count stability across pages.
func TestWorkListFiltersAndCounts(t *testing.T) {
	t.Parallel()
	factoryDir := support.ScaffoldFactory(t, workListFiltersCountsFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
	})

	process := workCLIProcess
	home := t.TempDir()

	works := []workListFiltersCountsWork{
		{Name: "task-initial", WorkID: "work-task-initial", WorkTypeName: "task"},
		{Name: "task-processing", WorkID: "work-task-processing", WorkTypeName: "task"},
		{Name: "task-terminal", WorkID: "work-task-terminal", WorkTypeName: "task"},
		{Name: "task-failed", WorkID: "work-task-failed", WorkTypeName: "task"},
		{Name: "other-initial", WorkID: "work-other-initial", WorkTypeName: "other"},
		{Name: "other-terminal", WorkID: "work-other-terminal", WorkTypeName: "other"},
	}
	submitWorkListFiltersCountsBatch(t, process, home, server.URL(), works)

	for _, move := range []struct {
		workID string
		state  string
	}{
		{workID: "work-task-processing", state: "processing"},
		{workID: "work-task-terminal", state: "complete"},
		{workID: "work-task-failed", state: "failed"},
		{workID: "work-other-terminal", state: "complete"},
	} {
		moveWorkListFiltersCountsWork(t, process, home, server.URL(), move.workID, move.state)
	}

	filtered := listWorkListFiltersCounts(t, process, home, server.URL(),
		"--non-terminal", "--work-type", "task", "--counts")
	assertWorkListFiltersCountsSummary(t, filtered, 2, 2)
	assertWorkListFiltersCountsIDs(t, filtered, map[string]bool{
		"work-task-initial":    true,
		"work-task-processing": true,
	})
	for _, item := range filtered.Results {
		if item.State == nil ||
			(item.State.Type != factoryapi.WorkStateTypeINITIAL &&
				item.State.Type != factoryapi.WorkStateTypePROCESSING) {
			t.Fatalf("non-terminal result has terminal state: %#v", item)
		}
	}

	page := listWorkListFiltersCounts(t, process, home, server.URL(),
		"--non-terminal", "--work-type", "task", "--counts", "--max-results", "1")
	assertWorkListFiltersCountsSummary(t, page, 2, 2)
	assertWorkListFiltersCountsIDs(t, page, map[string]bool{
		"work-task-initial":    true,
		"work-task-processing": true,
	})
	if page.PaginationContext != nil && page.PaginationContext.NextToken != nil {
		t.Fatalf("aggregate filtered list retained a continuation token: %#v", page.PaginationContext)
	}

	terminal := listWorkListFiltersCounts(t, process, home, server.URL(),
		"--terminal", "--work-type", "task", "--counts")
	assertWorkListFiltersCountsSummary(t, terminal, 2, 2)
	assertWorkListFiltersCountsIDs(t, terminal, map[string]bool{
		"work-task-terminal": true,
		"work-task-failed":   true,
	})

	byState := listWorkListFiltersCounts(t, process, home, server.URL(),
		"--state", "init", "--work-type", "task", "--counts")
	assertWorkListFiltersCountsSummary(t, byState, 1, 1)
	assertWorkListFiltersCountsIDs(t, byState, map[string]bool{"work-task-initial": true})

	zero := listWorkListFiltersCounts(t, process, home, server.URL(),
		"--non-terminal", "--work-type", "other", "--state", "complete", "--counts")
	assertWorkListFiltersCountsSummary(t, zero, 0, 0)

	human := executeWorkListFiltersCountsCLI(t, process, home,
		"--server", server.URL(), "work", "list", "--non-terminal", "--work-type", "task", "--counts")
	if !strings.Contains(human, "Total: 2") {
		t.Fatalf("human filtered list missing stable total:\n%s", human)
	}
	zeroHuman := executeWorkListFiltersCountsCLI(t, process, home,
		"--server", server.URL(), "work", "list", "--non-terminal", "--work-type", "other", "--state", "complete", "--counts")
	if !strings.Contains(zeroHuman, "Total: 0") || !strings.Contains(zeroHuman, "No work found.") {
		t.Fatalf("human zero-match list missing total or empty treatment:\n%s", zeroHuman)
	}
}

// TestWorkListPublicCLITraversesThreeRESTPages proves the customer-facing CLI
// exhausts a three-page REST collection while an independent direct REST walk
// remains page-shaped and canonical.
func TestWorkListPublicCLITraversesThreeRESTPages(t *testing.T) {
	t.Parallel()
	factoryDir := support.ScaffoldFactory(t, workListFiltersCountsFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
	})
	process := workCLIProcess
	home := t.TempDir()

	works := make([]workListFiltersCountsWork, 0, 51)
	wantIDs := make([]string, 0, 51)
	for index := 1; index <= 51; index++ {
		workID := fmt.Sprintf("work-page-%02d", index)
		works = append(works, workListFiltersCountsWork{
			Name:         fmt.Sprintf("page-work-%02d", index),
			WorkID:       workID,
			WorkTypeName: "task",
		})
		wantIDs = append(wantIDs, workID)
	}
	submitWorkListFiltersCountsBatchWithRequestID(t, process, home, server.URL(), "work-list-three-pages", works)

	manualPages := manualWorkListRESTWalk(t, server.URL(), 17)
	if len(manualPages) != 3 {
		t.Fatalf("manual REST page count = %d, want three", len(manualPages))
	}
	manualIDs := make([]string, 0, 51)
	for pageIndex, page := range manualPages {
		if len(page.Results) != 17 {
			t.Fatalf("manual REST page %d size = %d, want 17", pageIndex+1, len(page.Results))
		}
		for _, item := range page.Results {
			manualIDs = append(manualIDs, support.StringPointerValue(item.WorkId))
		}
	}
	expectedIDCounts := make(map[string]int, len(wantIDs))
	for _, workID := range wantIDs {
		expectedIDCounts[workID] = 1
	}
	if len(manualIDs) != len(wantIDs) {
		t.Fatalf("manual REST ID count = %d, want %d: %#v", len(manualIDs), len(wantIDs), manualIDs)
	}
	for _, workID := range manualIDs {
		if expectedIDCounts[workID] != 1 {
			t.Fatalf("manual REST IDs contain unexpected or duplicate Work %q: %#v", workID, manualIDs)
		}
		expectedIDCounts[workID] = 0
	}
	for workID, count := range expectedIDCounts {
		if count != 0 {
			t.Fatalf("manual REST IDs are missing Work %q: %#v", workID, manualIDs)
		}
	}

	jsonOutput := executeWorkListFiltersCountsCLI(t, process, home, "--server", server.URL(),
		"--json", "work", "list", "--counts", "--max-results", "17")
	var listed factoryapi.ListWorkResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(jsonOutput)), &listed); err != nil {
		t.Fatalf("decode aggregate Work list: %v\noutput:\n%s", err, jsonOutput)
	}
	if listed.Counts == nil || listed.Counts.Total != 51 || len(listed.Results) != 51 {
		t.Fatalf("aggregate Work list summary = counts=%#v results=%d, want total/results 51", listed.Counts, len(listed.Results))
	}
	cliIDs := make([]string, 0, len(listed.Results))
	for _, item := range listed.Results {
		cliIDs = append(cliIDs, support.StringPointerValue(item.WorkId))
	}
	if !equalWorkIDs(cliIDs, manualIDs) {
		t.Fatalf("CLI IDs = %#v, want independent REST walk IDs %#v", cliIDs, manualIDs)
	}
	if listed.PaginationContext == nil || listed.PaginationContext.MaxResults != 17 || listed.PaginationContext.NextToken != nil {
		t.Fatalf("aggregate pagination = %#v, want maxResults=17 and exhausted continuation", listed.PaginationContext)
	}

	humanOutput := executeWorkListFiltersCountsCLI(t, process, home, "--server", server.URL(),
		"work", "list", "--max-results", "17")
	if strings.Count(humanOutput, "WORK ID\tNAME\tWORK TYPE") != 1 {
		t.Fatalf("human header count = %d, want one", strings.Count(humanOutput, "WORK ID\tNAME\tWORK TYPE"))
	}
	for _, workID := range wantIDs {
		if strings.Count(humanOutput, workID) != 1 {
			t.Fatalf("human output occurrence count for %q = %d, want one", workID, strings.Count(humanOutput, workID))
		}
	}
}

// TestWorkShowPublicCLILooksUpWorkBeyondFirstRESTPage proves that Work detail
// lookup remains independent of collection pagination on a large board.
func TestWorkShowPublicCLILooksUpWorkBeyondFirstRESTPage(t *testing.T) {
	t.Parallel()
	factoryDir := support.ScaffoldFactory(t, workListFiltersCountsFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
	})
	process := workCLIProcess
	home := t.TempDir()

	works := make([]workListFiltersCountsWork, 0, 51)
	for index := 1; index <= 51; index++ {
		works = append(works, workListFiltersCountsWork{
			Name:         fmt.Sprintf("page-work-%02d", index),
			WorkID:       fmt.Sprintf("work-page-%02d", index),
			WorkTypeName: "task",
		})
	}
	submitWorkListFiltersCountsBatchWithRequestID(t, process, home, server.URL(), "work-show-large-board", works)

	output := executeWorkListFiltersCountsCLI(t, process, home, "--server", server.URL(),
		"--json", "work", "show", "work-page-51")
	var shown factoryapi.Work
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &shown); err != nil {
		t.Fatalf("decode Work show response: %v\noutput:\n%s", err, output)
	}
	if support.StringPointerValue(shown.WorkId) != "work-page-51" || shown.Name != "page-work-51" {
		t.Fatalf("shown Work = %#v, want work-page-51/page-work-51", shown)
	}
}

// TestWorkListAllAnnotatesSupersededSameName proves that the public CLI keeps
// the current same-name Work visible by default while --all restores the
// superseded attempt and identifies its successor in both JSON and human
// output.
func TestWorkListAllAnnotatesSupersededSameName(t *testing.T) {
	t.Parallel()
	factoryDir := support.ScaffoldFactory(t, workListFiltersCountsFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
	})

	process := workCLIProcess
	home := t.TempDir()
	oldWork := workListFiltersCountsWork{
		Name:         "retryable-task",
		WorkID:       "work-retryable-old",
		WorkTypeName: "task",
	}
	newWork := workListFiltersCountsWork{
		Name:         oldWork.Name,
		WorkID:       "work-retryable-new",
		WorkTypeName: oldWork.WorkTypeName,
	}
	submitWorkListFiltersCountsBatchWithRequestID(t, process, home, server.URL(),
		"work-list-superseded-old", []workListFiltersCountsWork{oldWork})
	moveWorkListFiltersCountsWork(t, process, home, server.URL(), oldWork.WorkID, "failed")
	submitWorkListFiltersCountsBatchWithRequestID(t, process, home, server.URL(),
		"work-list-superseded-new", []workListFiltersCountsWork{newWork})
	freshFailure := workListFiltersCountsWork{
		Name:         "fresh-failure",
		WorkID:       "work-fresh-failure",
		WorkTypeName: "task",
	}
	submitWorkListFiltersCountsBatchWithRequestID(t, process, home, server.URL(),
		"work-list-fresh-failure", []workListFiltersCountsWork{freshFailure})
	moveWorkListFiltersCountsWork(t, process, home, server.URL(), freshFailure.WorkID, "failed")
	assertWorkListSupersessionDefault(t, process, home, server.URL(), oldWork, newWork, freshFailure)
	assertWorkListSupersessionHistory(t, process, home, server.URL(), oldWork, newWork)
}

func assertWorkListSupersessionDefault(
	t *testing.T,
	process support.Process,
	home string,
	serverURL string,
	oldWork workListFiltersCountsWork,
	newWork workListFiltersCountsWork,
	freshFailure workListFiltersCountsWork,
) {
	t.Helper()

	defaultOutput := executeWorkListFiltersCountsCLI(t, process, home, "--server", serverURL,
		"--json", "work", "list", "--name", oldWork.Name, "--counts")
	var defaultList factoryapi.ListWorkResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(defaultOutput)), &defaultList); err != nil {
		t.Fatalf("decode default same-name Work list: %v\noutput:\n%s", err, defaultOutput)
	}
	assertWorkListFiltersCountsSummary(t, defaultList, 1, 1)
	assertWorkListFiltersCountsIDs(t, defaultList, map[string]bool{newWork.WorkID: true})
	if strings.Contains(defaultOutput, "supersededBy") || strings.Contains(defaultOutput, oldWork.WorkID) {
		t.Fatalf("default same-name list exposed superseded Work: %s", defaultOutput)
	}

	freshFailureList := listWorkListFiltersCounts(t, process, home, serverURL,
		"--terminal", "--name", freshFailure.Name, "--counts")
	assertWorkListFiltersCountsSummary(t, freshFailureList, 1, 1)
	assertWorkListFiltersCountsIDs(t, freshFailureList, map[string]bool{freshFailure.WorkID: true})
}

func assertWorkListSupersessionHistory(
	t *testing.T,
	process support.Process,
	home string,
	serverURL string,
	oldWork workListFiltersCountsWork,
	newWork workListFiltersCountsWork,
) {
	t.Helper()

	terminal := listWorkListFiltersCounts(t, process, home, serverURL,
		"--all", "--name", oldWork.Name, "--terminal", "--counts")
	assertWorkListFiltersCountsSummary(t, terminal, 1, 1)
	assertWorkListFiltersCountsIDs(t, terminal, map[string]bool{oldWork.WorkID: true})
	nonTerminal := listWorkListFiltersCounts(t, process, home, serverURL,
		"--all", "--name", oldWork.Name, "--non-terminal", "--counts")
	assertWorkListFiltersCountsSummary(t, nonTerminal, 1, 1)
	assertWorkListFiltersCountsIDs(t, nonTerminal, map[string]bool{newWork.WorkID: true})

	allPage := listWorkListFiltersCounts(t, process, home, serverURL,
		"--all", "--name", oldWork.Name, "--counts", "--max-results", "1")
	assertWorkListFiltersCountsSummary(t, allPage, 2, 2)
	assertWorkListFiltersCountsIDs(t, allPage, map[string]bool{oldWork.WorkID: true, newWork.WorkID: true})
	if allPage.PaginationContext != nil && allPage.PaginationContext.NextToken != nil {
		t.Fatalf("--all aggregate retained a continuation token: %#v", allPage.PaginationContext)
	}

	allOutput := executeWorkListFiltersCountsCLI(t, process, home, "--server", serverURL,
		"--json", "work", "list", "--all", "--name", oldWork.Name, "--counts")
	var allList factoryapi.ListWorkResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(allOutput)), &allList); err != nil {
		t.Fatalf("decode --all same-name Work list: %v\noutput:\n%s", err, allOutput)
	}
	assertWorkListFiltersCountsSummary(t, allList, 2, 2)
	seenOld := false
	seenNew := false
	for _, item := range allList.Results {
		workID := support.StringPointerValue(item.WorkId)
		switch workID {
		case oldWork.WorkID:
			seenOld = true
			if support.StringPointerValue(item.SupersededBy) != newWork.WorkID {
				t.Fatalf("superseded Work annotation = %q, want %q", support.StringPointerValue(item.SupersededBy), newWork.WorkID)
			}
		case newWork.WorkID:
			seenNew = true
			if item.SupersededBy != nil {
				t.Fatalf("current Work has successor annotation: %#v", item.SupersededBy)
			}
		default:
			t.Fatalf("unexpected same-name Work in --all response: %#v", item)
		}
	}
	if !seenOld || !seenNew || !strings.Contains(allOutput, `"supersededBy":"`+newWork.WorkID+`"`) {
		t.Fatalf("--all response missing old/new annotation: %s", allOutput)
	}

	human := executeWorkListFiltersCountsCLI(t, process, home, "--server", serverURL,
		"work", "list", "--all", "--name", oldWork.Name)
	if !strings.Contains(human, oldWork.WorkID) || !strings.Contains(human, newWork.WorkID) ||
		!strings.Contains(human, "Superseded by: "+newWork.WorkID) {
		t.Fatalf("human --all list missing successor annotation:\n%s", human)
	}
}

func workListFiltersCountsFactoryConfig() map[string]any {
	states := []map[string]string{
		{"name": "init", "type": "INITIAL"},
		{"name": "processing", "type": "PROCESSING"},
		{"name": "complete", "type": "TERMINAL"},
		{"name": "failed", "type": "FAILED"},
	}
	return map[string]any{
		"name": "work-list-filters-counts",
		"workTypes": []map[string]any{
			{"name": "task", "states": states},
			{"name": "other", "states": states},
		},
		"workers": []map[string]string{{"name": "unused-worker"}},
	}
}

func submitWorkListFiltersCountsBatch(
	t *testing.T,
	process support.Process,
	home string,
	serverURL string,
	works []workListFiltersCountsWork,
) {
	submitWorkListFiltersCountsBatchWithRequestID(t, process, home, serverURL,
		workListFiltersCountsRequestID, works)
}

func submitWorkListFiltersCountsBatchWithRequestID(
	t *testing.T,
	process support.Process,
	home string,
	serverURL string,
	requestID string,
	works []workListFiltersCountsWork,
) {
	t.Helper()
	requests := make([]workListFiltersCountsRequest, 0, len(works))
	for _, item := range works {
		requests = append(requests, workListFiltersCountsRequest{
			Name:         item.Name,
			WorkID:       item.WorkID,
			WorkTypeName: item.WorkTypeName,
			Payload:      map[string]string{"scenario": item.Name},
		})
	}
	payload, err := json.Marshal(workListFiltersCountsBatch{
		RequestID: requestID,
		Type:      "FACTORY_REQUEST_BATCH",
		Works:     requests,
	})
	if err != nil {
		t.Fatalf("marshal Work list scenario batch: %v", err)
	}
	output := executeWorkListFiltersCountsCLI(t, process, home, "--server", serverURL,
		"--json", "submit", "batch", string(payload))
	var submitted workListFiltersCountsSubmitResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &submitted); err != nil {
		t.Fatalf("decode Work list scenario submission: %v\noutput:\n%s", err, output)
	}
	if submitted.WorkCount != len(works) || len(submitted.Works) != len(works) {
		t.Fatalf("submission acknowledgment = %#v, want %d works", submitted, len(works))
	}
	for index, item := range works {
		if submitted.Works[index].WorkID != item.WorkID {
			t.Fatalf("submitted work[%d] id = %q, want %q", index, submitted.Works[index].WorkID, item.WorkID)
		}
	}
}

func moveWorkListFiltersCountsWork(
	t *testing.T,
	process support.Process,
	home string,
	serverURL string,
	workID string,
	state string,
) {
	t.Helper()
	output := executeWorkListFiltersCountsCLI(t, process, home, "--server", serverURL,
		"--json", "work", "move", workID, state)
	var moved workListFiltersCountsMoveResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &moved); err != nil {
		t.Fatalf("decode move %s to %s: %v\noutput:\n%s", workID, state, err, output)
	}
	if moved.WorkID != workID || moved.NewState != state {
		t.Fatalf("move response = %#v, want work %q at %q", moved, workID, state)
	}
}

func listWorkListFiltersCounts(
	t *testing.T,
	process support.Process,
	home string,
	serverURL string,
	flags ...string,
) factoryapi.ListWorkResponse {
	t.Helper()
	args := append([]string{"--server", serverURL, "--json", "work", "list"}, flags...)
	output := executeWorkListFiltersCountsCLI(t, process, home, args...)
	var listed factoryapi.ListWorkResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &listed); err != nil {
		t.Fatalf("decode Work list response: %v\noutput:\n%s", err, output)
	}
	return listed
}

func manualWorkListRESTWalk(
	t *testing.T,
	serverURL string,
	maxResults int,
) []factoryapi.ListWorkResponse {
	t.Helper()
	query := url.Values{
		"counts":     []string{"true"},
		"maxResults": []string{fmt.Sprintf("%d", maxResults)},
	}
	seenTokens := make(map[string]bool)
	pages := make([]factoryapi.ListWorkResponse, 0, 3)
	for {
		endpoint := support.DefaultSessionWorkURL(serverURL, "/work") + "?" + query.Encode()
		page := support.GetJSON[factoryapi.ListWorkResponse](t, endpoint)
		pages = append(pages, page)
		if page.PaginationContext == nil || page.PaginationContext.NextToken == nil ||
			strings.TrimSpace(*page.PaginationContext.NextToken) == "" {
			return pages
		}
		nextToken := *page.PaginationContext.NextToken
		if seenTokens[nextToken] {
			t.Fatalf("manual REST walk repeated next token after %d pages", len(pages))
		}
		seenTokens[nextToken] = true
		query.Set("nextToken", nextToken)
	}
}

func equalWorkIDs(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func executeWorkListFiltersCountsCLI(
	t *testing.T,
	process support.Process,
	home string,
	args ...string,
) string {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), append([]string{"you"}, args...))
	inputs.Input.Env = workListFiltersCountsEnvironment(home)
	inputs.Input.WorkingDirectory = home
	stdinIsTTY := true
	inputs.Input.StdinIsTTY = &stdinIsTTY
	inputs.Input.Stdin = strings.NewReader("")
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(%s) error = %v\nstdout:\n%s\nstderr:\n%s",
			strings.Join(args, " "), err, inputs.Stdout(), inputs.Stderr())
	}
	return inputs.Stdout()
}

func workListFiltersCountsEnvironment(home string) []string {
	result := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		name := strings.SplitN(entry, "=", 2)[0]
		if strings.EqualFold(name, "HOME") || strings.EqualFold(name, "USERPROFILE") {
			continue
		}
		result = append(result, entry)
	}
	return append(result, "HOME="+home, "USERPROFILE="+home)
}

func assertWorkListFiltersCountsSummary(
	t *testing.T,
	response factoryapi.ListWorkResponse,
	wantTotal int,
	wantPageSize int,
) {
	t.Helper()
	if response.Counts == nil || response.Counts.Total != wantTotal {
		t.Fatalf("Work list counts = %#v, want total %d", response.Counts, wantTotal)
	}
	if len(response.Results) != wantPageSize {
		t.Fatalf("Work list page size = %d, want %d: %#v", len(response.Results), wantPageSize, response.Results)
	}
}

func assertWorkListFiltersCountsIDs(
	t *testing.T,
	response factoryapi.ListWorkResponse,
	want map[string]bool,
) {
	t.Helper()
	seen := make(map[string]bool, len(response.Results))
	for _, item := range response.Results {
		workID := support.StringPointerValue(item.WorkId)
		if !want[workID] {
			t.Fatalf("unexpected Work %q in filtered response: %#v", workID, response.Results)
		}
		seen[workID] = true
	}
	if len(seen) != len(want) {
		t.Fatalf("filtered Work IDs = %#v, want all of %#v", seen, want)
	}
}
