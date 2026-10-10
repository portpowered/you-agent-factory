package definitions

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Creation and session-owned replacement both preserve authored content and
// prune stale graph references without rewriting an unsupported layout version.
func TestDefinitionsAuthoredSavePreservesBodiesAndLayout(t *testing.T) {
	for _, schema := range []int32{1, 99} {
		t.Run(stringSchemaName(schema), func(t *testing.T) {
			t.Parallel()
			host := sharedDefinitionsValidationServer(t)
			process := buildDefinitionsProcess(t)
			cfg := validAPIValidationFactoryConfig()
			cfg["layout"] = authoredSaveLayout(schema)
			dir := support.ScaffoldFactory(t, cfg)
			root := t.TempDir()
			name := "authored-save"
			target := support.CreateAndActivateNamedFactoryAtRootWithProcess(t, process, host.env, dir, root, name, filepath.Join(dir, "factory.json"))
			id := openDefinitionsNamedSession(t, host.URL(), root, name)
			t.Cleanup(func() { closeDefinitionsFactorySession(t, host.URL(), id) })
			endpoint := host.URL() + "/factory-sessions/" + id + "/factory"
			before := support.GetJSON[factoryapi.Factory](t, endpoint)
			assertAuthoredSaveLayout(t, before, schema)

			workerBody := "Inline worker instructions survive split persistence."
			stationBody := "Inline workstation instructions survive split persistence."
			(*before.Workers)[0].Body = &workerBody
			(*before.Workstations)[0].Body = &stationBody
			before.Layout = authoredSaveLayout(schema)
			version := *before.Version
			version.Logical++
			version.Physical = version.Physical.Add(time.Nanosecond)
			before.Version = &version
			// Replacement must retain the selected target even when an imported
			// payload carries a different name.
			before.Name = "drifted-import"
			findings, status := postValidateFactory(t, host.URL(), before)
			if status != http.StatusOK {
				t.Fatalf("validate authored input = %d %#v", status, findings)
			}
			for _, finding := range findings.Targets {
				if finding.Severity == factoryapi.FactoryValidationSeverityError {
					t.Fatalf("authored input rejected: %#v", finding)
				}
			}
			payload, err := json.Marshal(factoryapi.SaveFactoryForSessionRequest{Factory: before})
			if err != nil {
				t.Fatal(err)
			}
			response, _, status := definitionsHTTPRequest(t, http.MethodPut, endpoint, payload)
			if status != http.StatusOK {
				t.Fatalf("save authored input = %d %s", status, response)
			}
			after := support.GetJSON[factoryapi.Factory](t, endpoint)
			if after.Name != factoryapi.FactoryName(name) {
				t.Fatalf("replacement selected name = %q, want %q", after.Name, name)
			}
			assertAuthoredSaveLayout(t, after, schema)
			if stringValue((*after.Workers)[0].Body) != workerBody || stringValue((*after.Workstations)[0].Body) != stationBody {
				t.Fatal("save/readback changed inline authored bodies")
			}
			assertAuthoredSaveFiles(t, target, after, workerBody, stationBody)
		})
	}
}

// Explicit default targets own their folder and session; the shared host's
// Current Factory is unaffected by parallel default-target replacements.
func TestDefinitionsDefaultSavePersistsSplitBodies(t *testing.T) {
	t.Parallel()
	host := sharedDefinitionsValidationServer(t)
	cfg := validAPIValidationFactoryConfig()
	cfg["layout"] = authoredSaveLayout(1)
	cfg["workers"].([]map[string]string)[0]["type"] = "MODEL_WORKER"
	dir := support.ScaffoldFactory(t, cfg)
	payload, err := json.Marshal(factoryapi.OpenFactorySessionRequest{
		FolderPath: dir,
		Target:     &factoryapi.FactorySessionTargetRef{Kind: factoryapi.FactorySessionTargetRefKindDefault},
	})
	if err != nil {
		t.Fatal(err)
	}
	response, _, status := definitionsHTTPRequest(t, http.MethodPost, host.URL()+"/factory-sessions", payload)
	if status != http.StatusOK {
		t.Fatalf("open default target = %d %s", status, response)
	}
	var opened factoryapi.OpenFactorySessionResponse
	if err := json.Unmarshal(response, &opened); err != nil {
		t.Fatal(err)
	}
	if opened.Session == nil {
		t.Fatal("missing default session")
	}
	id := opened.Session.Id
	t.Cleanup(func() { closeDefinitionsFactorySession(t, host.URL(), id) })
	endpoint := host.URL() + "/factory-sessions/" + id + "/factory"
	before := support.GetJSON[factoryapi.Factory](t, endpoint)
	workerBody, stationBody := "Default worker split instructions.", "Default workstation split instructions."
	(*before.Workers)[0].Body = &workerBody
	(*before.Workstations)[0].Body = &stationBody
	before.Version.Logical++
	before.Version.Physical = before.Version.Physical.Add(time.Nanosecond)
	payload, err = json.Marshal(factoryapi.SaveFactoryForSessionRequest{Factory: before})
	if err != nil {
		t.Fatal(err)
	}
	response, _, status = definitionsHTTPRequest(t, http.MethodPut, endpoint, payload)
	if status != http.StatusOK {
		t.Fatalf("save default target = %d %s", status, response)
	}
	after := support.GetJSON[factoryapi.Factory](t, endpoint)
	if after.Name != before.Name || stringValue((*after.Workers)[0].Body) != workerBody || stringValue((*after.Workstations)[0].Body) != stationBody {
		t.Fatalf("default save/readback changed selected name or authored bodies: before=%#v after=%#v worker=%q station=%q", before.Name, after.Name, stringValue((*after.Workers)[0].Body), stringValue((*after.Workstations)[0].Body))
	}
	assertAuthoredSaveFiles(t, dir, after, workerBody, stationBody)
}

func stringSchemaName(schema int32) string {
	if schema == 1 {
		return "supported"
	}
	return "unsupported"
}

func authoredSaveLayout(schema int32) *factoryapi.FactoryLayout {
	return &factoryapi.FactoryLayout{
		SchemaVersion: schema,
		Nodes: &[]factoryapi.FactoryLayoutNode{
			{Id: "workstation:process", Position: factoryapi.FactoryLayoutPoint{X: 10, Y: 20}},
			{Id: "workstation:stale-node", Position: factoryapi.FactoryLayoutPoint{X: 30, Y: 40}},
		},
		Viewport: &factoryapi.FactoryLayoutViewport{Zoom: 1},
	}
}

func assertAuthoredSaveLayout(t *testing.T, factory factoryapi.Factory, schema int32) {
	t.Helper()
	if factory.Layout == nil || factory.Layout.SchemaVersion != schema || factory.Layout.Nodes == nil || len(*factory.Layout.Nodes) != 1 {
		t.Fatalf("persisted layout = %#v, want schema %d and one live node", factory.Layout, schema)
	}
	node := (*factory.Layout.Nodes)[0]
	if node.Id != "workstation:process" || node.Position.X != 10 || node.Position.Y != 20 {
		t.Fatalf("persisted live node = %#v", node)
	}
}

func assertAuthoredSaveFiles(t *testing.T, dir string, saved factoryapi.Factory, workerBody, stationBody string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "factory.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), workerBody) || strings.Contains(string(raw), stationBody) {
		t.Fatal("canonical factory.json retained inline authored bodies")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["layoutOutcomes"]; ok {
		t.Fatal("canonical factory.json retained retired layout metadata")
	}
	var durable factoryapi.Factory
	if err := json.Unmarshal(raw, &durable); err != nil {
		t.Fatal(err)
	}
	assertAuthoredSaveLayout(t, durable, saved.Layout.SchemaVersion)
	if !reflect.DeepEqual(durable.Version, saved.Version) {
		t.Fatalf("durable version = %#v, want readback %#v", durable.Version, saved.Version)
	}
	for path, body := range map[string]string{
		filepath.Join("workers", (*saved.Workers)[0].Name, "AGENTS.md"):           workerBody,
		filepath.Join("workstations", (*saved.Workstations)[0].Name, "AGENTS.md"): stationBody,
	} {
		content, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil || !strings.Contains(string(content), body) {
			t.Fatalf("split authored file %s = %s, %v", path, content, err)
		}
	}
}
