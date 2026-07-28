package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerscli "github.com/portpowered/infinite-you/pkg/services/providers/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
)

type catalogPeerFake struct {
	providers map[providers.ID]providers.Descriptor
}

func newCatalogPeerFake(entries ...providers.Descriptor) *catalogPeerFake {
	catalog := make(map[providers.ID]providers.Descriptor, len(entries))
	for _, entry := range entries {
		catalog[entry.ID] = entry.Clone()
	}
	return &catalogPeerFake{providers: catalog}
}

func (fake *catalogPeerFake) ListProviders(
	_ context.Context,
	_ providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	ids := make([]providers.ID, 0, len(fake.providers))
	for id := range fake.providers {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return ids[i].String() < ids[j].String()
	})

	results := make([]providers.Descriptor, 0, len(ids))
	for _, id := range ids {
		results = append(results, fake.providers[id].Clone())
	}
	return providers.ListProvidersResult{Providers: results}, nil
}

func (fake *catalogPeerFake) GetProvider(
	_ context.Context,
	request providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	if err := request.Validate(); err != nil {
		return providers.GetProviderResult{}, err
	}
	descriptor, ok := fake.providers[request.ID]
	if !ok {
		return providers.GetProviderResult{}, providers.ErrUnknownProvider
	}
	if descriptor.Availability != providers.AvailabilitySelectable ||
		descriptor.Readiness != providers.ReadinessReady ||
		hasMissingPrerequisite(descriptor.Prerequisites) {
		return providers.GetProviderResult{}, providers.ErrProviderUnavailable
	}
	return providers.GetProviderResult{Provider: descriptor.Clone()}, nil
}

func (fake *catalogPeerFake) Execute(
	context.Context,
	providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	return providers.ExecuteResult{}, nil
}

func hasMissingPrerequisite(prerequisites []providers.Prerequisite) bool {
	for _, prerequisite := range prerequisites {
		if prerequisite.Status == providers.PrerequisiteMissing {
			return true
		}
	}
	return false
}

func representativeCatalogRoot() providers.Service {
	codex := providers.Descriptor{
		ID:           providers.IDCodex,
		Aliases:      []string{"openai-codex"},
		DisplayName:  "Codex",
		Availability: providers.AvailabilitySelectable,
		Readiness:    providers.ReadinessReady,
		Capabilities: []providers.Capability{
			providers.CapabilityPromptSubmission,
			providers.CapabilityNativeStreaming,
		},
	}
	cursor := providers.Descriptor{
		ID:           providers.IDCursor,
		DisplayName:  "Cursor",
		Availability: providers.AvailabilitySupportedButUnavailable,
		Readiness:    providers.ReadinessUnavailable,
		Prerequisites: []providers.Prerequisite{{
			Kind:        providers.PrerequisiteConfiguration,
			Name:        "executable",
			Status:      providers.PrerequisiteMissing,
			Description: "cursor-agent must be installed",
		}},
		Capabilities: []providers.Capability{providers.CapabilityPromptSubmission},
	}
	return newCatalogPeerFake(codex, cursor)
}

func TestAcceptedCLIContract_PSSI03SurfacesRemainOutOfScope(t *testing.T) {
	t.Parallel()

	repoRoot := testutil.MustRepoPath(t, ".")
	for _, relativePath := range []string{
		"pkg/services/providers/transports/cli/composition.go",
		"pkg/services/providers/transports/http",
		"pkg/services/providers/transports/mcp",
	} {
		relativePath := relativePath
		t.Run(relativePath, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(repoRoot, filepath.FromSlash(relativePath))
			_, err := os.Stat(path)
			if err == nil {
				t.Fatalf(
					"%s exists; this packet must not add PSS-I03 composition, HTTP-PROV, or MCP-PROV surfaces",
					relativePath,
				)
			}
			if !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("stat %s = %v", relativePath, err)
			}
		})
	}
}

func TestAcceptedCLIContract_ExportsNoCompositionBindings(t *testing.T) {
	t.Parallel()

	output, err := exec.Command("go", "doc", "-all", providersCLIImportPath).CombinedOutput()
	if err != nil {
		t.Fatalf("go doc -all: %v\n%s", err, output)
	}
	doc := string(output)
	if strings.Contains(doc, "func Bind") {
		t.Fatalf("Providers CLI adapter exports composition Bind helpers:\n%s", doc)
	}
	for _, forbidden := range []string{"BindList", "BindShow", "BindService", "NewComposition"} {
		if strings.Contains(doc, forbidden) {
			t.Fatalf("Providers CLI adapter exports %s; PSS-I03 composition stays out of scope", forbidden)
		}
	}
}

func TestAcceptedCLIContract_ProductionManifestDeclaresNoProvidersCommands(t *testing.T) {
	t.Parallel()

	manifest := loadCLICommandManifest(t, climanifest.ProductionManifestPath)
	for id, command := range manifest.Commands {
		if strings.HasPrefix(id, "you.providers.") || id == "you.providers" {
			t.Fatalf(
				"production manifest declares %s (%q); this packet must not edit top-level CLI composition",
				id,
				command.Path,
			)
		}
		if strings.HasPrefix(strings.TrimSpace(command.Path), "you providers ") {
			t.Fatalf(
				"production manifest declares providers path %q; this packet must not edit top-level CLI composition",
				command.Path,
			)
		}
	}
}

func TestAcceptedCLIContract_ListPreservesAcceptedHumanAndJSONOutput(t *testing.T) {
	t.Parallel()

	root := representativeCatalogRoot()
	service := constructedProvidersCLIService(t, root)

	var human bytes.Buffer
	if err := service.List(providerscli.ListConfig{
		Context: context.Background(),
		Output:  &human,
	}); err != nil {
		t.Fatalf("List() human error = %v", err)
	}
	wantHuman := strings.Join([]string{
		"ID\tDISPLAY NAME\tAVAILABILITY\tREADINESS\tALIASES",
		"agent\tCursor\tsupported-but-unavailable\tunavailable\tnone",
		"codex\tCodex\tselectable\tready\topenai-codex",
		"",
	}, "\n")
	if human.String() != wantHuman {
		t.Fatalf("List() human output = %q, want %q", human.String(), wantHuman)
	}

	var jsonOut bytes.Buffer
	if err := service.List(providerscli.ListConfig{
		Context: context.Background(),
		JSON:    true,
		Output:  &jsonOut,
	}); err != nil {
		t.Fatalf("List() JSON error = %v", err)
	}
	var got struct {
		Providers []struct {
			ID           string   `json:"id"`
			DisplayName  string   `json:"displayName"`
			Aliases      []string `json:"aliases"`
			Availability string   `json:"availability"`
			Readiness    string   `json:"readiness"`
			Capabilities []string `json:"capabilities"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(jsonOut.Bytes(), &got); err != nil {
		t.Fatalf("List() JSON invalid: %v\n%s", err, jsonOut.String())
	}
	if len(got.Providers) != 2 {
		t.Fatalf("providers = %d, want 2", len(got.Providers))
	}
	if got.Providers[0].ID != "agent" || got.Providers[1].ID != "codex" {
		t.Fatalf("providers order = %#v, want agent then codex", got.Providers)
	}
	if got.Providers[1].DisplayName != "Codex" ||
		got.Providers[1].Availability != "selectable" ||
		got.Providers[1].Readiness != "ready" {
		t.Fatalf("codex entry = %#v", got.Providers[1])
	}
	if len(got.Providers[1].Capabilities) != 2 {
		t.Fatalf("codex capabilities = %#v, want two entries", got.Providers[1].Capabilities)
	}
	if got.Providers[0].Availability != "supported-but-unavailable" {
		t.Fatalf("agent availability = %q", got.Providers[0].Availability)
	}
}

func TestAcceptedCLIContract_ShowPreservesAcceptedHumanOutput(t *testing.T) {
	t.Parallel()

	root := representativeCatalogRoot()
	service := constructedProvidersCLIService(t, root)

	var out bytes.Buffer
	if err := service.Show(providerscli.ShowConfig{
		Context:    context.Background(),
		ProviderID: "codex",
		Output:     &out,
	}); err != nil {
		t.Fatalf("Show() error = %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"ID:\tcodex\n",
		"Display name:\tCodex\n",
		"Availability:\tselectable\n",
		"Readiness:\tready\n",
		"Aliases:\topenai-codex\n",
		"Capabilities:\tnative_streaming, prompt_submission\n",
		"Prerequisites:\tnone\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Show() output missing %q:\n%s", want, got)
		}
	}
}

func TestAcceptedCLIContract_ShowTypedFailuresPreserveAcceptedErrors(t *testing.T) {
	t.Parallel()

	root := representativeCatalogRoot()
	service := constructedProvidersCLIService(t, root)

	assertShowErrorIs(t, service, "", providers.ErrInvalidID)
	assertShowErrorIs(t, service, "claude", providers.ErrUnknownProvider)
	assertShowErrorIs(t, service, "agent", providers.ErrProviderUnavailable)
}

func assertShowErrorIs(
	t *testing.T,
	service providerscli.Service,
	providerID string,
	want error,
) {
	t.Helper()

	var out bytes.Buffer
	err := service.Show(providerscli.ShowConfig{
		Context:    context.Background(),
		ProviderID: providerID,
		Output:     &out,
	})
	if !errors.Is(err, want) {
		t.Fatalf("Show(%q) error = %v, want %v", providerID, err, want)
	}
	if out.Len() != 0 {
		t.Fatalf("Show(%q) stdout = %q, want empty on failure", providerID, out.String())
	}
}
