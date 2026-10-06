package isolation_and_recovery_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func testRestartProbeDAG(t *testing.T, process support.Process, dir string, apis []*support.ProcessAPIServer, runner *restartProbeUnexpectedRunner) {
	t.Helper()
	testRestartProbeDAGWithInputs(t, process, dir, apis, runner, restartProbeInputs, 23101)
}

func testRestartProbeDAGWithInputs(t *testing.T, process support.Process, dir string, apis []*support.ProcessAPIServer, runner *restartProbeUnexpectedRunner, inputs func(*testing.T, string) *support.CapturedInputs, port int) {
	t.Helper()
	batch := `{"requestId":"restart-dag","type":"FACTORY_REQUEST_BATCH","works":[{"workId":"restart-A","name":"A","workTypeName":"task","state":"init","payload":"restart A"},{"workId":"restart-B","name":"B","workTypeName":"task","state":"waiting","tags":{"witness":"§ —"},"payload":"restart § — B"},{"workId":"restart-C","name":"C","workTypeName":"task","state":"init","tags":{"witness":"§ —"},"payload":"restart § — C"}],"relations":[{"type":"DEPENDS_ON","sourceWorkName":"B","targetWorkName":"A","requiredState":"complete"},{"type":"DEPENDS_ON","sourceWorkName":"C","targetWorkName":"B","requiredState":"complete"}]}`
	workPath := filepath.Join(dir, "dag.json")
	writeRestartProbeFile(t, workPath, []byte(batch))
	sentinel := filepath.Join(dir, "worktrees", "sentinel.txt")
	writeRestartProbeFile(t, sentinel, []byte("worktree § —"))
	first := inputs(t, dir)
	if port != 0 {
		first.Input.Args = append(first.Input.Args, "--listen", "127.0.0.1:"+strconv.Itoa(port), "--work", workPath)
	}
	command := support.StartProcessCommand(t, process, first.Input)
	url := restartProbeReadyURL(t, apis[0], command)
	if port == 0 {
		// The exact launch starts empty; Work enters through the public API
		// only after readiness, without adding a selector or startup input.
		works := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(url, "~default", "/work"))
		if len(works.Results) != 0 {
			t.Fatal("first plain launch was not empty")
		}
		request, err := http.NewRequestWithContext(t.Context(), http.MethodPut, url+"/factory-sessions/~default/work-requests/restart-dag", bytes.NewReader([]byte(batch)))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			t.Fatalf("batch admission returned %d", response.StatusCode)
		}
	}
	// Public status observes the asynchronous projection after all three
	// admissions and A's completion, rather than padding with a sleep.
	support.WaitForStatus(t, url, 15*time.Second, func(status factoryapi.StatusResponse) bool {
		return status.TotalTokens == 3 && status.Categories.Terminal == 1
	})
	waitForRestartProbeConfirmed(t, url)
	before := restartProbeBoardReads(t, url)
	events := support.GetFactoryEventsForSessionAt(t, url, "~default")
	assertRestartProbeStates(t, before)
	restartProbeShutdown(t, url, command)
	second := inputs(t, dir)
	if port != 0 {
		second.Input.Args = append(second.Input.Args, "--listen", "127.0.0.1:"+strconv.Itoa(port+1))
	}
	reopened := support.StartProcessCommand(t, process, second.Input)
	url = restartProbeReadyURL(t, apis[1], reopened)
	after := restartProbeBoardReads(t, url)
	assertRestartProbeStates(t, after)
	for i := range before {
		if !reflect.DeepEqual(before[i].Content, after[i].Content) || !reflect.DeepEqual(before[i].Tags, after[i].Tags) || !reflect.DeepEqual(before[i].Relations, after[i].Relations) || !reflect.DeepEqual(before[i].WorkId, after[i].WorkId) {
			t.Fatalf("restart changed content, tags, relations or identity: before=%#v after=%#v", before[i], after[i])
		}
	}
	recoveredEvents := support.GetFactoryEventsForSessionAt(t, url, "~default")
	assertRestartProbeEventFacts(t, events, recoveredEvents)
	if runner.calls.Load() != 1 {
		t.Fatal("restart dispatched terminal A or blocked descendants")
	}
	request, _ := json.Marshal(factoryapi.MoveWorkRequest{StateName: "init"})
	restartProbePost(t, support.SessionWorkURL(url, "~default", "/work/restart-B/move"), request)
	support.WaitForStatus(t, url, 15*time.Second, func(status factoryapi.StatusResponse) bool { return status.Categories.Terminal == 3 })
	assertRestartProbeCompletions(t, url, runner)
	waitForRestartProbeConfirmed(t, url)
	completed := restartProbeBoardReads(t, url)
	completedEvents := support.GetFactoryEventsForSessionAt(t, url, "~default")
	restartProbeShutdown(t, url, reopened)
	if len(apis) > 2 {
		// Reopen the same repository/reference/recording again after every
		// Work is terminal. Reuse is the invariant, so this journey is ordered.
		third := inputs(t, dir)
		repeated := support.StartProcessCommand(t, process, third.Input)
		url = restartProbeReadyURL(t, apis[2], repeated)
		for i, work := range restartProbeBoardReads(t, url) {
			if work.State == nil || work.State.Name != "complete" || !reflect.DeepEqual(work.Content, completed[i].Content) || !reflect.DeepEqual(work.WorkId, completed[i].WorkId) || !reflect.DeepEqual(work.Relations, completed[i].Relations) {
				t.Fatalf("terminal restart changed Work: before=%#v after=%#v", completed[i], work)
			}
		}
		assertRestartProbeEventFacts(t, completedEvents, support.GetFactoryEventsForSessionAt(t, url, "~default"))
		if runner.calls.Load() != 3 {
			t.Fatal("terminal restart dispatched completed Work")
		}
		restartProbeShutdown(t, url, repeated)
	}
	if !bytes.Equal(mustReadSeededReplayArtifact(t, sentinel), []byte("worktree § —")) || !bytes.Equal(mustReadSeededReplayArtifact(t, workPath), []byte(batch)) {
		t.Fatal("restart mutated worktree sentinel or request source")
	}
}

func waitForRestartProbeConfirmed(t *testing.T, url string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	// Public confirmation is the durability boundary. A terminal projection
	// alone does not establish that the next process can recover these facts.
	for ctx.Err() == nil {
		confirmed := true
		for _, work := range restartProbeBoardReads(t, url) {
			confirmed = confirmed && work.ConfirmationState != nil && *work.ConfirmationState == factoryapi.CONFIRMED
		}
		if confirmed {
			return
		}
	}
	t.Fatal("DAG Work did not reach CONFIRMED before shutdown")
}

func restartProbeReadyURL(t *testing.T, api *support.ProcessAPIServer, command *support.ProcessCommand) string {
	t.Helper()
	ready := make(chan string, 1)
	go func() {
		url, err := api.WaitForBaseURL(60 * time.Second)
		if err == nil {
			ready <- url
		}
	}()
	select {
	case url := <-ready:
		return url
	case <-command.Done():
		var causes []string
		for err := command.Err(); err != nil; err = errors.Unwrap(err) {
			causes = append(causes, err.Error())
		}
		t.Fatalf("restart command ended before readiness: %s", strings.Join(causes, "; "))
	case <-time.After(support.ScaledTimeout(60 * time.Second)):
		t.Fatal("restart command did not publish API readiness")
	}
	return ""
}

func assertRestartProbeEventFacts(t *testing.T, before, after []factoryapi.FactoryEvent) {
	t.Helper()
	facts := make(map[string]factoryapi.FactoryEvent)
	for _, event := range after {
		facts[event.Id] = event
	}
	proved := 0
	for _, event := range before {
		if event.Type != factoryapi.FactoryEventTypeDispatchRequest && event.Type != factoryapi.FactoryEventTypeDispatchResponse && event.Type != factoryapi.FactoryEventTypeWorkStateChange {
			continue
		}
		proved++
		recovered, ok := facts[event.Id]
		if !ok {
			t.Fatalf("restart lost canonical fact %s", event.Id)
		}
		// Public JSON union payloads can change object key order on recording
		// serialization; compare the canonical facts, including their context.
		oldBytes, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		newBytes, err := json.Marshal(recovered)
		if err != nil {
			t.Fatal(err)
		}
		var oldValue, newValue any
		if err := json.Unmarshal(oldBytes, &oldValue); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(newBytes, &newValue); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(oldValue, newValue) {
			t.Fatalf("canonical event %s changed: before=%s after=%s", event.Id, oldBytes, newBytes)
		}
	}
	if proved < 2 {
		t.Fatal("prior dispatch/state facts missing")
	}
}

func assertRestartProbeCompletions(t *testing.T, url string, runner *restartProbeUnexpectedRunner) {
	t.Helper()
	for _, id := range []string{"restart-A", "restart-B", "restart-C"} {
		w := support.GetDefaultSessionWorkByID(t, url, id)
		if w.WorkId == nil || *w.WorkId != id || w.State == nil || w.State.Name != "complete" {
			t.Fatalf("completion lost identity or state: %#v", w)
		}
	}
	for _, text := range []string{"restart A", "restart § — B", "restart § — C"} {
		select {
		case prompt := <-runner.requests:
			if !strings.Contains(strings.Join(prompt.Args, " ")+string(prompt.Stdin), text) {
				t.Fatalf("provider did not receive exact UTF-8 text %q: %s", text, strings.Join(prompt.Args, " ")+string(prompt.Stdin))
			}
		case <-time.After(15 * time.Second):
			t.Fatal("provider prompt missing")
		}
	}
	if runner.calls.Load() != 3 {
		t.Fatalf("DAG dispatched %d calls, want three", runner.calls.Load())
	}
}

func restartProbeBoardReads(t *testing.T, url string) []factoryapi.Work {
	t.Helper()
	var works []factoryapi.Work
	for _, id := range []string{"restart-A", "restart-B", "restart-C"} {
		works = append(works, support.GetDefaultSessionWorkByID(t, url, id))
	}
	return works
}

func assertRestartProbeStates(t *testing.T, works []factoryapi.Work) {
	t.Helper()
	for i, state := range []string{"complete", "waiting", "init"} {
		if works[i].State == nil || works[i].State.Name != state {
			t.Fatalf("Work state = %#v, want %s", works[i], state)
		}
		if i > 0 {
			content, err := json.Marshal(works[i].Content)
			if err != nil || !bytes.Contains(content, []byte("§ —")) || works[i].Relations == nil || len(*works[i].Relations) != 1 {
				t.Fatalf("missing UTF-8 or relationship: %#v", works[i])
			}
		}
	}
}

func restartProbePost(t *testing.T, endpoint string, body []byte) {
	t.Helper()
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		t.Fatalf("POST %s returned %d", endpoint, response.StatusCode)
	}
}

func restartProbeShutdown(t *testing.T, url string, command *support.ProcessCommand) {
	t.Helper()
	restartProbePost(t, url+"/shutdown", []byte(`{}`))
	select {
	case <-command.Done():
		if err := command.Err(); err != nil && !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("HTTP shutdown did not stop invocation")
	}
}

func writeRestartProbeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
