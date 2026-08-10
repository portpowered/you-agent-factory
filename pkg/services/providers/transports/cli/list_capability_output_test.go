package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerscli "github.com/portpowered/infinite-you/pkg/services/providers/transports/cli"
)

func TestListOutputIncludesCompleteCapabilityFactsAndStableNestedOrdering(t *testing.T) {
	t.Parallel()

	service := constructedProvidersCLIService(t, completeCapabilityRoot())

	var human bytes.Buffer
	if err := service.List(providerscli.ListConfig{Context: context.Background(), Output: &human}); err != nil {
		t.Fatalf("List() human error = %v", err)
	}
	assertCompleteHumanOutput(t, human.String())

	var jsonOutput bytes.Buffer
	if err := service.List(providerscli.ListConfig{Context: context.Background(), JSON: true, Output: &jsonOutput}); err != nil {
		t.Fatalf("List() JSON error = %v", err)
	}
	assertCompleteJSONOutput(t, jsonOutput.Bytes())
}

func completeCapabilityRoot() providers.Service {
	maximum := int64(5)
	defaultTimeout := int64(300)
	codex := providers.Descriptor{
		ID:                         providers.IDCodex,
		DisplayName:                "Codex",
		Availability:               providers.AvailabilitySelectable,
		Readiness:                  providers.ReadinessReady,
		TechnicalSupportLevel:      providers.TechnicalSupportProduction,
		ImplementationAvailability: providers.ImplementationBundled,
		Models: []providers.ModelDescriptor{{
			ID: "gpt-5.6", Efforts: []providers.ReasoningEffort{"high", "low"}, Modalities: []providers.Modality{
				{Direction: providers.ModalityDirectionInput, Kind: providers.ModalityVideo, Support: providers.ModalityUnsupported, Transport: providers.ModalityTransportNone},
				{Direction: providers.ModalityDirectionInput, Kind: providers.ModalityText, Support: providers.ModalitySupported, Transport: providers.ModalityTransportInline},
				{Direction: providers.ModalityDirectionInput, Kind: providers.ModalityAudio, Support: providers.ModalityUnsupported, Transport: providers.ModalityTransportNone},
			}}},
		Tools:        []providers.Tool{{Name: "shell", Support: providers.ToolSupported, Description: "Run shell commands."}},
		KnownLimits:  []providers.KnownLimit{{Name: "referenced_image_paths", Kind: providers.KnownLimitMaximum, Unit: "paths", Description: "Image generation accepts at most five referenced image paths.", Maximum: &maximum}},
		Capabilities: []providers.Capability{providers.CapabilityStructuredOutput, providers.CapabilityPromptSubmission},
	}
	agy := providers.Descriptor{
		ID:                         providers.IDAntigravity,
		DisplayName:                "Antigravity",
		Availability:               providers.AvailabilitySelectable,
		Readiness:                  providers.ReadinessReady,
		TechnicalSupportLevel:      providers.TechnicalSupportExperimental,
		ImplementationAvailability: providers.ImplementationBundled,
		Prerequisites:              []providers.Prerequisite{{Kind: providers.PrerequisiteExecutable, Name: "agy", Status: providers.PrerequisiteSatisfied, Description: "The AGY executable is required."}},
		Models: []providers.ModelDescriptor{{
			ID: "claude-opus-4-6-thinking", Efforts: []providers.ReasoningEffort{"medium", "low"}, Modalities: []providers.Modality{
				{Direction: providers.ModalityDirectionInput, Kind: providers.ModalityVideo, Support: providers.ModalitySupported, Transport: providers.ModalityTransportFilePath},
				{Direction: providers.ModalityDirectionInput, Kind: providers.ModalityAudio, Support: providers.ModalitySupported, Transport: providers.ModalityTransportFilePath},
			}}},
		KnownLimits: []providers.KnownLimit{
			{Name: "print_timeout", Kind: providers.KnownLimitDefault, Unit: "seconds", Description: "AGY uses a five-minute default.", Default: &defaultTimeout},
			{Name: "add_dir_workspace", Kind: providers.KnownLimitBehavior, Unit: "flag", Description: "AGY extends the workspace.", Value: "--add-dir"},
		},
	}
	return &recordingProvidersRoot{listResult: providers.ListProvidersResult{Providers: []providers.Descriptor{codex, agy}}}
}

func assertCompleteHumanOutput(t *testing.T, output string) {
	t.Helper()
	for _, want := range []string{
		"Technical support:\tproduction", "Implementation:\tbundled", "Models:", "Efforts:\tlow, high", "Input modalities:",
		"audio: unsupported (transport: none)", "video: unsupported (transport: none)", "video: supported (transport: file_path)",
		"Known limits:", "referenced_image_paths [maximum, paths] maximum=5", "add_dir_workspace [behavior, flag] value=--add-dir", "print_timeout [default, seconds] default=300",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("human output missing %q:\n%s", want, output)
		}
	}
	if strings.Index(output, "antigravity\t") > strings.Index(output, "codex\t") {
		t.Fatalf("human providers are not sorted by id:\n%s", output)
	}
}

func assertCompleteJSONOutput(t *testing.T, source []byte) {
	t.Helper()
	var got listCapabilityJSON
	if err := json.Unmarshal(source, &got); err != nil {
		t.Fatalf("JSON output is invalid: %v", err)
	}
	if len(got.Providers) != 2 {
		t.Fatalf("provider order = %#v, want antigravity then codex", got.Providers)
	}
	if got.Providers[0].ID != "antigravity" || got.Providers[1].ID != "codex" {
		t.Fatalf("provider order = %#v, want antigravity then codex", got.Providers)
	}
	assertAGYJSONFacts(t, got.Providers[0])
	assertCodexJSONFacts(t, got.Providers[1])
}

func assertAGYJSONFacts(t *testing.T, agy listCapabilityProviderJSON) {
	t.Helper()
	if agy.TechnicalSupportLevel != "experimental" || agy.ImplementationAvailability != "bundled" {
		t.Fatalf("AGY publication facts = %#v", agy)
	}
	assertAGYModelFacts(t, agy.Models)
	assertAGYLimitFacts(t, agy.KnownLimits)
}

func assertAGYModelFacts(t *testing.T, models []listModelJSON) {
	t.Helper()
	if len(models) != 1 {
		t.Fatalf("AGY models = %#v", models)
	}
	model := models[0]
	if len(model.Efforts) != 2 || model.Efforts[0] != "low" || model.Efforts[1] != "medium" {
		t.Fatalf("AGY effort order = %#v", model.Efforts)
	}
	if len(model.Modalities) != 2 {
		t.Fatalf("AGY modalities = %#v", model.Modalities)
	}
	if model.Modalities[0].Modality != "audio" || model.Modalities[1].Modality != "video" {
		t.Fatalf("AGY modality order = %#v", model.Modalities)
	}
}

func assertAGYLimitFacts(t *testing.T, limits []listKnownLimitJSON) {
	t.Helper()
	if len(limits) != 2 {
		t.Fatalf("AGY limits = %#v", limits)
	}
	if limits[0].Name != "add_dir_workspace" || limits[0].Value != "--add-dir" {
		t.Fatalf("AGY workspace limit = %#v", limits[0])
	}
	if limits[1].Name != "print_timeout" || limits[1].Default == nil || *limits[1].Default != 300 {
		t.Fatalf("AGY timeout limit = %#v", limits[1])
	}
}

func assertCodexJSONFacts(t *testing.T, codex listCapabilityProviderJSON) {
	t.Helper()
	if len(codex.Models) != 1 {
		t.Fatalf("Codex models = %#v", codex.Models)
	}
	modalities := codex.Models[0].Modalities
	if len(modalities) != 3 {
		t.Fatalf("Codex modalities = %#v", modalities)
	}
	if modalities[0].Modality != "audio" || modalities[0].Support != "unsupported" {
		t.Fatalf("Codex audio modality = %#v", modalities[0])
	}
	if modalities[2].Modality != "video" || modalities[2].Support != "unsupported" {
		t.Fatalf("Codex video modality = %#v", modalities[2])
	}
	if len(codex.KnownLimits) != 1 {
		t.Fatalf("Codex limits = %#v", codex.KnownLimits)
	}
	limit := codex.KnownLimits[0]
	if limit.Maximum == nil || *limit.Maximum != 5 {
		t.Fatalf("Codex image-path limit = %#v", limit)
	}
}

func TestListOutputMakesEmptyCapabilityCollectionsExplicit(t *testing.T) {
	t.Parallel()

	root := &recordingProvidersRoot{listResult: providers.ListProvidersResult{Providers: []providers.Descriptor{{
		ID: providers.IDClaude, DisplayName: "Claude", Availability: providers.AvailabilitySelectable, Readiness: providers.ReadinessReady,
	}}}}
	service := constructedProvidersCLIService(t, root)
	var output bytes.Buffer
	if err := service.List(providerscli.ListConfig{Context: context.Background(), JSON: true, Output: &output}); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	var got listCapabilityJSON
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("JSON output is invalid: %v", err)
	}
	if len(got.Providers) != 1 {
		t.Fatalf("providers = %#v", got.Providers)
	}
	entry := got.Providers[0]
	if entry.Aliases == nil || entry.Prerequisites == nil || entry.Models == nil || entry.Tools == nil || entry.KnownLimits == nil || entry.Capabilities == nil {
		t.Fatalf("empty collections must be explicit arrays: %#v", entry)
	}
	var human bytes.Buffer
	if err := service.List(providerscli.ListConfig{Context: context.Background(), Output: &human}); err != nil {
		t.Fatalf("List() human error = %v", err)
	}
	for _, want := range []string{"Prerequisites:\n    - none", "Models:\n    - none", "Tools:\n    - none", "Known limits:\n    - none", "Capabilities:\tnone"} {
		if !strings.Contains(human.String(), want) {
			t.Fatalf("human output missing explicit empty value %q:\n%s", want, human.String())
		}
	}
}

func TestListOutputWriterFailureDoesNotReportSuccessOrLeakErrorText(t *testing.T) {
	t.Parallel()

	secret := errors.New("provider command /private/token=secret failed")
	root := &recordingProvidersRoot{listErr: secret}
	service := constructedProvidersCLIService(t, root)
	var output bytes.Buffer
	var diagnostics bytes.Buffer
	if err := service.List(providerscli.ListConfig{
		Context: context.Background(), Output: &output, Diagnostics: &diagnostics, Verbose: true,
	}); !errors.Is(err, secret) {
		t.Fatalf("List() error = %v, want service failure", err)
	}
	if strings.Contains(diagnostics.String(), secret.Error()) || strings.Contains(diagnostics.String(), "complete") {
		t.Fatalf("diagnostics leaked failure text or success:\n%s", diagnostics.String())
	}

	writerRoot := &recordingProvidersRoot{listResult: providers.ListProvidersResult{Providers: []providers.Descriptor{{
		ID: providers.IDCodex, DisplayName: "Codex", Availability: providers.AvailabilitySelectable, Readiness: providers.ReadinessReady,
	}}}}
	service = constructedProvidersCLIService(t, writerRoot)
	failingOutput := &failAfterWriter{limit: 1, err: errors.New("output secret /private/path")}
	diagnostics.Reset()
	err := service.List(providerscli.ListConfig{
		Context: context.Background(), Output: failingOutput, Diagnostics: &diagnostics, Verbose: true,
	})
	if err == nil || !strings.Contains(err.Error(), "output secret") {
		t.Fatalf("List() writer error = %v, want writer error", err)
	}
	if strings.Contains(diagnostics.String(), "complete") || strings.Contains(diagnostics.String(), "output secret") {
		t.Fatalf("writer diagnostics reported fabricated success or leaked text:\n%s", diagnostics.String())
	}
	if !strings.Contains(diagnostics.String(), "stage=write") {
		t.Fatalf("writer diagnostics missing safe stage:\n%s", diagnostics.String())
	}
}

type listCapabilityJSON struct {
	Providers []listCapabilityProviderJSON `json:"providers"`
}

type listCapabilityProviderJSON struct {
	ID                         string                 `json:"id"`
	TechnicalSupportLevel      string                 `json:"technicalSupportLevel"`
	ImplementationAvailability string                 `json:"implementationAvailability"`
	Aliases                    []string               `json:"aliases"`
	Prerequisites              []listPrerequisiteJSON `json:"prerequisites"`
	Models                     []listModelJSON        `json:"models"`
	Tools                      []listToolJSON         `json:"tools"`
	KnownLimits                []listKnownLimitJSON   `json:"knownLimits"`
	Capabilities               []string               `json:"capabilities"`
}

type listPrerequisiteJSON struct {
	Kind string `json:"kind"`
}

type listModelJSON struct {
	Efforts    []string           `json:"efforts"`
	Modalities []listModalityJSON `json:"modalities"`
}

type listModalityJSON struct {
	Modality  string `json:"modality"`
	Support   string `json:"support"`
	Transport string `json:"transport"`
}

type listToolJSON struct {
	Name string `json:"name"`
}

type listKnownLimitJSON struct {
	Name    string `json:"name"`
	Default *int64 `json:"default"`
	Maximum *int64 `json:"maximum"`
	Value   string `json:"value"`
}

type failAfterWriter struct {
	limit int
	count int
	err   error
}

func (writer *failAfterWriter) Write(value []byte) (int, error) {
	if writer.count >= writer.limit {
		return 0, writer.err
	}
	allowed := writer.limit - writer.count
	if allowed > len(value) {
		allowed = len(value)
	}
	writer.count += allowed
	return allowed, writer.err
}

var _ io.Writer = (*failAfterWriter)(nil)
