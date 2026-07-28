package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	packagedfactories "github.com/portpowered/infinite-you/packages/packaged-factories"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestPackagedFactoryCatalogListsEveryEmbeddedFactory proves runtime packaged
// Factory catalog discovery through GET /packaged-factories returns every
// Factory in the embedded authored inventory and does not invent names absent
// from that inventory.
func TestPackagedFactoryCatalogListsEveryEmbeddedFactory(t *testing.T) {

	embeddedNames, err := embeddedPackagedFactoryNames()
	if err != nil {
		t.Fatalf("embedded packaged Factory inventory: %v", err)
	}

	discoveredNames, err := discoveredPackagedFactoryNamesViaHTTP(t)
	if err != nil {
		t.Fatalf("runtime packaged Factory catalog discovery: %v", err)
	}

	if missing, extra := nameSetDiff(embeddedNames, discoveredNames); len(missing) > 0 || len(extra) > 0 {
		t.Fatalf(
			"catalog discovery drift: missing from discovery %v, invented by discovery %v; embedded=%v discovered=%v",
			missing,
			extra,
			embeddedNames,
			discoveredNames,
		)
	}
}

func embeddedPackagedFactoryNames() ([]string, error) {
	inventory, err := packagedfactorycatalog.Discover(
		context.Background(),
		packagedfactories.Source(),
		"factories",
	)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(inventory.Entries))
	for index, entry := range inventory.Entries {
		names[index] = entry.Factory.Name
	}
	slices.Sort(names)
	return names, nil
}

func discoveredPackagedFactoryNamesViaHTTP(t *testing.T) ([]string, error) {
	t.Helper()

	dir := support.ScaffoldFactory(t, packagedFactoryCatalogTestConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
	})

	response, err := http.Get(server.URL() + "/packaged-factories")
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET /packaged-factories status = %d, want %d", response.StatusCode, http.StatusOK)
	}

	var catalog factoryapi.PackagedFactoryCatalogResponse
	if err := json.NewDecoder(response.Body).Decode(&catalog); err != nil {
		return nil, err
	}
	names := make([]string, len(catalog.Factories))
	for index, factory := range catalog.Factories {
		names[index] = factory.Name
	}
	slices.Sort(names)
	return names, nil
}

func nameSetDiff(want, got []string) (missing, extra []string) {
	wantSet := make(map[string]struct{}, len(want))
	for _, name := range want {
		wantSet[name] = struct{}{}
	}
	gotSet := make(map[string]struct{}, len(got))
	for _, name := range got {
		gotSet[name] = struct{}{}
		if _, ok := wantSet[name]; !ok {
			extra = append(extra, name)
		}
	}
	for _, name := range want {
		if _, ok := gotSet[name]; !ok {
			missing = append(missing, name)
		}
	}
	slices.Sort(missing)
	slices.Sort(extra)
	return missing, extra
}

func packagedFactoryCatalogTestConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}
