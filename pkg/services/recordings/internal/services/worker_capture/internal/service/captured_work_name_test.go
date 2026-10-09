package service

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestCapturedWorkNameOpeningRoundTrip(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"", "Build API"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			local := platformreplay.NewLocal(runtime.GOOS)
			writer := journalWriter(t, local)
			opening := journalRecord(t, "names", "worker")
			opening.WorkName = name
			if err := writer.PersistWorkerRecord(t.Context(), opening); err != nil {
				t.Fatal(err)
			}
			duplicate := opening
			duplicate.WorkName = "duplicate cannot rename"
			if err := writer.PersistWorkerRecord(t.Context(), duplicate); err != nil {
				t.Fatal(err)
			}
			output := opening
			output.WorkName = "later cannot rename"
			output.Record = mustRecord(t, workerOutputAppend(opening.Record.ID.Topic, "worker", 1, "output"), 2)
			if err := writer.PersistWorkerRecord(t.Context(), output); err != nil {
				t.Fatal(err)
			}
			data, err := local.ReadFile(writer.path("names") + "l")
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			assertCapturedWorkNameEnvelopes(t, lines, name)
			reopened, err := newTestFileWriter(local, writer.root)
			if err != nil {
				t.Fatal(err)
			}
			page, err := reopened.ReadWorkerCapturedActivity(t.Context(), recordings.WorkerCapturedActivityRequest{WorkerSessionID: "worker"})
			if err != nil || page.Catalog.WorkName != name || page.Catalog.CommittedPosition != 2 {
				t.Fatalf("durable name=%+v err=%v", page.Catalog, err)
			}
			catalog, err := reopened.ListWorkerSessionCaptures(t.Context(), recordings.WorkerCapturedCatalogRequest{})
			if err != nil || len(catalog.Items) != 1 || catalog.Items[0].Catalog.WorkName != name {
				t.Fatalf("catalog=%+v %v", catalog, err)
			}
			assertCapturedWorkNameCatalogSchema(t, page.Catalog)
			assertCapturedWorkNameRejectsLaterMetadata(t, local, writer, lines)
		})
	}
}

func TestCapturedWorkNameUncertainOpeningPreservesCommittedFact(t *testing.T) {
	t.Parallel()
	probe := &journalProbe{Local: platformreplay.NewLocal(runtime.GOOS), failAfterSync: true}
	writer := journalWriter(t, probe)
	opening := journalRecord(t, "names", "worker")
	opening.WorkName = "Committed"
	if err := writer.PersistWorkerRecord(t.Context(), opening); err == nil {
		t.Fatal("uncertain opening acknowledged")
	}
	opening.WorkName = "Retry cannot rename"
	if err := writer.PersistWorkerRecord(t.Context(), opening); err != nil {
		t.Fatal(err)
	}
	page, err := writer.ReadWorkerCapturedActivity(t.Context(), recordings.WorkerCapturedActivityRequest{WorkerSessionID: "worker"})
	if err != nil || page.Catalog.WorkName != "Committed" || len(probe.suffixes) != 1 {
		t.Fatalf("uncertain name=%+v err=%v appends=%d", page.Catalog, err, len(probe.suffixes))
	}
}

func assertCapturedWorkNameCatalogSchema(t *testing.T, catalog recordings.WorkerSessionCatalogEntry) {
	t.Helper()
	schemaBytes, err := os.ReadFile(filepath.Join("..", "..", "schemas", "session-catalog.v1.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schemaDoc, document any
	if err := json.Unmarshal(schemaBytes, &schemaDoc); err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("catalog", schemaDoc); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile("catalog")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(document); err != nil {
		t.Fatal(err)
	}
}

func assertCapturedWorkNameEnvelopes(t *testing.T, lines []string, name string) {
	t.Helper()
	for index, line := range lines {
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			t.Fatal(err)
		}
		_, present := envelope["workName"]
		if present != (index == 0 && name != "") {
			t.Fatalf("opening-only omission: index=%d envelope=%s", index, line)
		}
	}
}

func assertCapturedWorkNameRejectsLaterMetadata(t *testing.T, local platformreplay.Local, writer *FileWriter, lines []string) {
	t.Helper()
	// A later on-disk name is malformed capture metadata, never a rename.
	var later workerJournalEntry
	if err := json.Unmarshal([]byte(lines[1]), &later); err != nil {
		t.Fatal(err)
	}
	later.WorkName = "foreign"
	encoded, err := json.Marshal(later)
	if err != nil {
		t.Fatal(err)
	}
	damaged := []byte(lines[0] + "\n" + string(encoded) + "\n")
	if err := local.WriteFile(writer.path("names")+"l", damaged); err != nil {
		t.Fatal(err)
	}
	invalid, err := newTestFileWriter(local, writer.root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := invalid.ReadWorkerCapturedActivity(t.Context(), recordings.WorkerCapturedActivityRequest{WorkerSessionID: "worker"}); !errors.Is(err, recordings.ErrWorkerRecordingReplay) {
		t.Fatalf("non-opening name=%v", err)
	}
}
