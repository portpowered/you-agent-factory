package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Validate actual persisted envelopes against the authored storage contract;
// this tests encoding compatibility, not a source/registration inventory.
func TestControlOperationJournalMatchesAuthoredSchemas(t *testing.T) {
	t.Parallel()
	compiler := jsonschema.NewCompiler()
	for _, name := range []string{"control-operation.v1.schema.json", "control-envelope.v2.schema.json"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "schemas", name))
		if err != nil {
			t.Fatal(err)
		}
		var document map[string]any
		if err := json.Unmarshal(data, &document); err != nil {
			t.Fatal(err)
		}
		if err := compiler.AddResource(name, document); err != nil {
			t.Fatal(err)
		}
	}
	schema, err := compiler.Compile("control-envelope.v2.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	writer := journalWriter(t, platformreplay.NewLocal(runtime.GOOS))
	intent := controlIntent(t, writer, "recording", "worker", "request")
	if _, _, err := writer.BeginWorkerControlOperation(t.Context(), intent); err != nil {
		t.Fatal(err)
	}
	next := intent
	next.Revision, next.Operation.Phase = 2, "COMPLETED"
	next.Result = json.RawMessage(`{"outcome":"APPLIED"}`)
	if _, err := writer.AdvanceWorkerControlOperation(t.Context(), next, 1); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(writer.path("recording") + "l")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte{'\n'})[1:] {
		var document any
		if err := json.Unmarshal(line, &document); err != nil {
			t.Fatal(err)
		}
		if err := schema.Validate(document); err != nil {
			t.Fatalf("stored envelope: %v", err)
		}
	}
}

func TestControlOperationJournalRejectsMalformedOrUnknownVersion(t *testing.T) {
	t.Parallel()
	for _, cell := range []struct {
		name   string
		mutate func(map[string]any)
		cause  error
	}{
		{"unknown-version", func(d map[string]any) { d["version"] = 3 }, recordings.ErrWorkerRecordingCompatibility},
		{"unknown-field", func(d map[string]any) { d["handle"] = "foreign" }, recordings.ErrWorkerRecordingReplay},
		{"wrong-epoch", func(d map[string]any) { d["ownerEpoch"] = "other" }, recordings.ErrWorkerRecordingReplay},
		{"missing-payload", func(d map[string]any) { delete(d, "controlOperation") }, recordings.ErrWorkerRecordingReplay},
	} {
		t.Run(cell.name, func(t *testing.T) {
			t.Parallel()
			local := platformreplay.NewLocal(runtime.GOOS)
			writer := journalWriter(t, local)
			intent := controlIntent(t, writer, "recording", "worker", "request")
			if _, _, err := writer.BeginWorkerControlOperation(t.Context(), intent); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(writer.path("recording") + "l")
			if err != nil {
				t.Fatal(err)
			}
			lines := bytes.Split(bytes.TrimSpace(data), []byte{'\n'})
			var delta map[string]any
			if err := json.Unmarshal(lines[1], &delta); err != nil {
				t.Fatal(err)
			}
			cell.mutate(delta)
			mutated, err := json.Marshal(delta)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(writer.path("recording")+"l", append(append(append([]byte(nil), lines[0]...), '\n'), append(mutated, '\n')...), 0600); err != nil {
				t.Fatal(err)
			}
			reopened, err := newTestFileWriter(local, writer.root)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := reopened.LoadWorkerControlOperation(t.Context(), operationKey(intent)); !errors.Is(err, cell.cause) {
				t.Fatalf("malformed journal accepted: %v", err)
			}
		})
	}
}

func TestControlOperationTornTailRetainsCommittedPrefixAndRefusesAppend(t *testing.T) {
	t.Parallel()
	local := platformreplay.NewLocal(runtime.GOOS)
	writer := journalWriter(t, local)
	intent := controlIntent(t, writer, "recording", "worker", "request")
	if _, _, err := writer.BeginWorkerControlOperation(t.Context(), intent); err != nil {
		t.Fatal(err)
	}
	if err := local.AppendFile(writer.path("recording")+"l", []byte(`{"version":2`)); err != nil {
		t.Fatal(err)
	}
	reopened, err := newTestFileWriter(local, writer.root)
	if err != nil {
		t.Fatal(err)
	}
	latest, err := reopened.LoadWorkerControlOperation(t.Context(), operationKey(intent))
	if err != nil || latest.Revision != 1 {
		t.Fatalf("lost committed prefix: %+v %v", latest, err)
	}
	next := intent
	next.Revision, next.Operation.Phase = 2, "COMPLETED"
	next.Result = json.RawMessage(`{"outcome":"APPLIED"}`)
	if _, err := reopened.AdvanceWorkerControlOperation(t.Context(), next, 1); !errors.Is(err, recordings.ErrWorkerRecordingReplay) {
		t.Fatalf("appended to torn tail: %v", err)
	}
}
