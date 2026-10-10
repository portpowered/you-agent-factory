package addressing_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// CLI/HTTP parity is the owned contract here. The retained equal-ID records
// enter through the public production replay reader, never registry mutation.
func TestLegacyWorkerAddressing(t *testing.T) {
	t.Parallel()
	f := newReplayFixture(t)
	t.Run("F2-01 exact ambiguity", func(t *testing.T) {
		t.Parallel()
		status, raw := f.http(t, "GET", "/worker-sessions/"+f.worker, nil)
		if status != http.StatusConflict {
			t.Fatalf("show status = %d: %s", status, raw)
		}
		assertAmbiguity(t, f, raw, false)
		assertAmbiguity(t, f, f.cli(t, true, "show", "--worker-session-id", f.worker), false)
	})
	t.Run("F2-02 exact selected observation", func(t *testing.T) {
		t.Parallel()
		for _, owner := range f.owners {
			status, raw := f.http(t, "GET", "/worker-sessions/"+f.worker+"?factorySessionId="+owner.session, nil)
			if status != http.StatusOK {
				t.Fatalf("scoped show = %d: %s", status, raw)
			}
			assertOwner(t, f, owner, raw)
			assertOwner(t, f, owner, f.cli(t, false, "show", "--worker-session-id", f.worker, "--session", owner.session))
			status, scoped := f.http(t, "GET", "/factory-sessions/"+owner.session+"/worker-sessions/"+f.worker, nil)
			if status != http.StatusOK {
				t.Fatalf("existing scoped GET = %d: %s", status, scoped)
			}
			assertOwner(t, f, owner, scoped)
		}
	})
	for _, operation := range []string{"continue", "interrupt"} {
		t.Run("ambiguous "+operation+" has no effects", func(t *testing.T) {
			t.Parallel()
			body, flags := controlInput(operation)
			status, raw := f.http(t, "POST", "/worker-sessions/"+f.worker+"/"+operation, body)
			if status != http.StatusConflict {
				t.Fatalf("control = %d: %s", status, raw)
			}
			assertAmbiguity(t, f, raw, operation == "interrupt")
			assertAmbiguity(t, f, f.cli(t, true, append([]string{operation, f.worker}, flags...)...), operation == "interrupt")
			assertNoEffects(t, f, body["successorWorkerSessionId"].(string))
		})
	}
	// These legacy records have no live execution or provider association. Scope
	// disambiguates the observation but must not make a replay controllable.
	t.Run("selected replay without live execution has no control effects", func(t *testing.T) {
		t.Parallel()
		for _, owner := range f.owners {
			for _, operation := range []string{"continue", "interrupt"} {
				body, flags := controlInput(operation)
				body["factorySessionId"] = owner.session
				path := "/worker-sessions/" + f.worker + "/" + operation
				status, raw := f.http(t, "POST", path, body)
				assertNotFound(t, status, raw)
				cli := f.cli(t, true, append(append([]string{operation, f.worker}, flags...), "--session", owner.session)...)
				assertErrorCode(t, cli, "NOT_FOUND")
				if operation == "interrupt" {
					assertValidationPhase(t, raw)
					assertValidationPhase(t, cli)
				}
				assertNoEffects(t, f, body["successorWorkerSessionId"].(string))
			}
		}
	})
	t.Run("empty explicit owner is invalid before ambiguous controls", func(t *testing.T) {
		t.Parallel()
		assertEmptyLegacyOwner(t, f)
	})
	t.Run("F2-09 F2-10 foreign and unknown IDs", func(t *testing.T) {
		t.Parallel()
		assertUnknownLegacyAddresses(t, f)
	})
	t.Run("AM-T3 explicit head keeps exact scope refusal", func(t *testing.T) {
		t.Parallel()
		for _, cell := range []struct{ id, scope string }{
			{f.worker, ""}, {uuid.NewString(), ""}, {f.worker, uuid.NewString()},
		} {
			body, flags := controlInput("continue")
			body["resolveHead"] = true
			flags = append(flags, "--head")
			if cell.scope != "" {
				body["factorySessionId"] = cell.scope
				flags = append(flags, "--session", cell.scope)
			}
			status, raw := f.http(t, "POST", "/worker-sessions/"+cell.id+"/continue", body)
			cli := f.cli(t, true, append([]string{"continue", cell.id}, flags...)...)
			if cell.id == f.worker && cell.scope == "" {
				if status != http.StatusConflict {
					t.Fatalf("ambiguous head status = %d: %s", status, raw)
				}
				assertAmbiguity(t, f, raw, false)
				assertAmbiguity(t, f, cli, false)
			} else {
				assertNotFound(t, status, raw)
				assertErrorCode(t, cli, "NOT_FOUND")
			}
			assertNoEffects(t, f, body["successorWorkerSessionId"].(string))
		}
	})
}

func assertEmptyLegacyOwner(t *testing.T, f *replayFixture) {
	t.Helper()
	for _, scope := range []string{"", "   "} {
		status, raw := f.http(t, "GET", "/worker-sessions/"+f.worker+"?factorySessionId="+strings.ReplaceAll(scope, " ", "%20"), nil)
		assertBadRequest(t, status, raw)
		for _, operation := range []string{"continue", "interrupt"} {
			body, _ := controlInput(operation)
			body["factorySessionId"] = scope
			status, raw = f.http(t, "POST", "/worker-sessions/"+f.worker+"/"+operation, body)
			assertBadRequest(t, status, raw)
			if operation == "interrupt" {
				assertValidationPhase(t, raw)
			}
			assertNoEffects(t, f, body["successorWorkerSessionId"].(string))
		}
	}
}

func assertUnknownLegacyAddresses(t *testing.T, f *replayFixture) {
	t.Helper()
	for _, scoped := range []bool{false, true} {
		id, scope := uuid.NewString(), ""
		if scoped {
			id, scope = f.worker, uuid.NewString()
		}
		query := ""
		if scoped {
			query = "?factorySessionId=" + scope
		}
		status, raw := f.http(t, "GET", "/worker-sessions/"+id+query, nil)
		assertNotFound(t, status, raw)
		args := []string{"show", "--worker-session-id", id}
		if scoped {
			args = append(args, "--session", scope)
		}
		assertErrorCode(t, f.cli(t, true, args...), "WORKER_SESSION_NOT_FOUND")
		for _, operation := range []string{"continue", "interrupt"} {
			body, flags := controlInput(operation)
			if scoped {
				body["factorySessionId"] = scope
				flags = append(flags, "--session", scope)
			}
			status, raw = f.http(t, "POST", "/worker-sessions/"+id+"/"+operation, body)
			assertNotFound(t, status, raw)
			assertErrorCode(t, f.cli(t, true, append([]string{operation, id}, flags...)...), "NOT_FOUND")
			assertNoEffects(t, f, body["successorWorkerSessionId"].(string))
		}
	}
}

func assertBadRequest(t *testing.T, status int, raw []byte) {
	t.Helper()
	if status != http.StatusBadRequest {
		t.Fatalf("invalid scope = %d: %s", status, raw)
	}
	assertErrorCode(t, raw, "BAD_REQUEST")
}

func assertValidationPhase(t *testing.T, raw []byte) {
	t.Helper()
	var response struct{ Phase string }
	if err := json.Unmarshal(raw, &response); err != nil || response.Phase != "VALIDATION" {
		t.Fatalf("want VALIDATION: %s (%v)", raw, err)
	}
}

func controlInput(operation string) (map[string]any, []string) {
	request, successor := uuid.NewString(), uuid.NewString()
	field, flag := "followUpInput", "--user-message"
	if operation == "interrupt" {
		field, flag = "replacementMessage", "--replacement-message"
	}
	body := map[string]any{"requestId": request, "successorWorkerSessionId": successor, field: "replacement"}
	if operation == "interrupt" {
		body["resumeMode"] = "provider"
	}
	return body, []string{"--request-id", request, "--successor-worker-session-id", successor, flag, "replacement", "--async"}
}

func (f *replayFixture) cli(t *testing.T, wantError bool, args ...string) []byte {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), append([]string{"you", "--remote", "--server", f.url, "--json", "worker-sessions"}, args...))
	inputs.Input.WorkingDirectory = t.TempDir()
	home := t.TempDir()
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	err := f.process.Execute(inputs.Input)
	if (err != nil) != wantError {
		t.Fatalf("CLI %v = %v; stdout=%s stderr=%s", args, err, inputs.Stdout(), inputs.Stderr())
	}
	if wantError {
		if inputs.Stdout() != "" {
			t.Fatalf("error emitted success: %s", inputs.Stdout())
		}
		return []byte(inputs.Stderr())
	}
	return []byte(inputs.Stdout())
}

func (f *replayFixture) http(t *testing.T, method, path string, body any) (int, []byte) {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, f.url+path, bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, raw
}

func assertAmbiguity(t *testing.T, f *replayFixture, raw []byte, interrupt bool) {
	t.Helper()
	var got struct {
		Code, Phase string
		Details     struct {
			Candidates []struct{ FactorySessionID, WorkerSessionID, WorkID, State string }
		}
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode: %v: %s", err, raw)
	}
	if got.Code != "WORKER_SESSION_AMBIGUOUS" || (interrupt && got.Phase != "VALIDATION") {
		t.Fatalf("diagnostic: %s", raw)
	}
	var tuples []string
	for _, candidate := range got.Details.Candidates {
		tuples = append(tuples, candidate.FactorySessionID+"/"+candidate.WorkerSessionID+"/"+candidate.WorkID+"/"+candidate.State)
	}
	want := []string{f.owners[0].session + "/" + f.worker + "/" + f.owners[0].work + "/COMPLETED", f.owners[1].session + "/" + f.worker + "/" + f.owners[1].work + "/COMPLETED"}
	sort.Strings(tuples)
	sort.Strings(want)
	if !reflect.DeepEqual(tuples, want) {
		t.Fatalf("candidates = %v, want %v; %s", tuples, want, raw)
	}
}

func assertOwner(t *testing.T, f *replayFixture, owner replayOwner, raw []byte) {
	t.Helper()
	var observation factoryapi.WorkerSessionObservation
	if err := json.Unmarshal(raw, &observation); err != nil {
		t.Fatal(err)
	}
	if observation.WorkerSessionId != f.worker || observation.FactorySessionId == nil || *observation.FactorySessionId != owner.session ||
		!reflect.DeepEqual(observation.WorkIds, []string{owner.work}) || observation.State != "COMPLETED" {
		t.Fatalf("wrong selected observation: %s", raw)
	}
}

func assertNotFound(t *testing.T, status int, raw []byte) {
	t.Helper()
	if status != http.StatusNotFound {
		t.Fatalf("not-found = %d: %s", status, raw)
	}
	assertErrorCode(t, raw, "NOT_FOUND")
}

func assertErrorCode(t *testing.T, raw []byte, code string) {
	t.Helper()
	var response struct{ Code string }
	if err := json.Unmarshal(raw, &response); err != nil || response.Code != code {
		t.Fatalf("want %s: %s (%v)", code, raw, err)
	}
}

func assertNoEffects(t *testing.T, f *replayFixture, successor string) {
	t.Helper()
	if f.calls.Load() != 0 {
		t.Fatal("refused control launched a provider")
	}
	status, raw := f.http(t, "GET", "/worker-sessions/"+successor, nil)
	assertNotFound(t, status, raw)
	for _, owner := range f.owners {
		status, raw = f.http(t, "GET", "/worker-sessions/"+f.worker+"?factorySessionId="+owner.session, nil)
		if status != http.StatusOK {
			t.Fatalf("source changed: %d %s", status, raw)
		}
		assertOwner(t, f, owner, raw)
	}
}
