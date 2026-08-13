package authoredlayout

import (
	"path/filepath"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	factoryconfig "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
)

func TestFactorySourceLoaderPreservesHumanApprovalWorkstationAcrossJSONAndYAML(t *testing.T) {
	t.Parallel()

	documents := map[string]string{
		"json": `{
			"name":"human-approval",
			"workTypes":[{"name":"release","states":[{"name":"awaiting-approval","type":"INITIAL"},{"name":"approved","type":"TERMINAL"},{"name":"changes-requested","type":"PROCESSING"}]}],
			"workstations":[{
				"id":"release-approval",
				"name":"release-approval",
				"type":"HUMAN_APPROVAL",
				"description":{"type":"LOCALIZABLE_ASSET","value":"Confirm release","locales":["en-US","fr-FR"],"values":{"fr-FR":"Confirmer la version"}},
				"inputs":[{"workType":"release","state":"awaiting-approval"}],
				"outputs":[{"workType":"release","state":"approved"}],
				"onRejection":[{"workType":"release","state":"changes-requested"}]
			}]
		}`,
		"yaml": `name: human-approval
workTypes:
  - name: release
    states:
      - name: awaiting-approval
        type: INITIAL
      - name: approved
        type: TERMINAL
      - name: changes-requested
        type: PROCESSING
workstations:
  - id: release-approval
    name: release-approval
    type: HUMAN_APPROVAL
    description:
      type: LOCALIZABLE_ASSET
      value: Confirm release
      locales: [en-US, fr-FR]
      values:
        fr-FR: Confirmer la version
    inputs:
      - workType: release
        state: awaiting-approval
    outputs:
      - workType: release
        state: approved
    onRejection:
      - workType: release
        state: changes-requested
`,
	}

	for extension, document := range documents {
		t.Run(extension, func(t *testing.T) {
			t.Parallel()
			assertHumanApprovalSourceRoundTrip(t, extension, document)
		})
	}
}

func assertHumanApprovalSourceRoundTrip(t *testing.T, extension, document string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "factory."+extension)
	writeTestSource(t, path, document)
	source, err := NewFactorySourceLoader(localTestFileSystem{})(path)
	if err != nil {
		t.Fatalf("load %s source: %v", extension, err)
	}
	generated, err := factoryconfig.DecodeAuthoredFactoryAPI(source.Data)
	if err != nil {
		t.Fatalf("decode %s source: %v", extension, err)
	}
	if generated.Workstations == nil || len(*generated.Workstations) != 1 {
		t.Fatalf("generated workstations = %#v, want one workstation", generated.Workstations)
	}
	workstation := (*generated.Workstations)[0]
	if workstation.Type == nil || *workstation.Type != factoryapi.HUMANAPPROVAL {
		t.Fatalf("generated workstation type = %#v, want HUMAN_APPROVAL", workstation.Type)
	}
	if workstation.Worker != nil {
		t.Fatalf("generated human approval worker = %q, want omitted", *workstation.Worker)
	}
	cfg, err := factoryconfig.FactoryConfigFromOpenAPI(generated)
	if err != nil {
		t.Fatalf("map %s source: %v", extension, err)
	}
	assertHumanApprovalWorkstationRoundTrip(t, cfg.Workstations)
	flattened, err := factoryconfig.NewFactoryConfigMapper().Flatten(&cfg)
	if err != nil {
		t.Fatalf("flatten %s source: %v", extension, err)
	}
	if strings.Contains(string(flattened), `"worker"`) {
		t.Fatalf("flattened human approval unexpectedly declares a worker: %s", flattened)
	}
	if !strings.Contains(string(flattened), `"type":"HUMAN_APPROVAL"`) {
		t.Fatalf("flattened human approval lost its type: %s", flattened)
	}
	expanded, err := factoryconfig.NewFactoryConfigMapper().Expand(flattened)
	if err != nil {
		t.Fatalf("expand flattened %s source: %v", extension, err)
	}
	assertHumanApprovalWorkstationRoundTrip(t, expanded.Workstations)
}

func assertHumanApprovalWorkstationRoundTrip(t *testing.T, workstations []factorydefinitions.FactoryWorkstationConfig) {
	t.Helper()
	if len(workstations) != 1 {
		t.Fatalf("workstations = %#v, want one workstation", workstations)
	}
	workstation := workstations[0]
	assertHumanApprovalIdentity(t, workstation)
	assertHumanApprovalDescription(t, workstation)
	assertHumanApprovalRoutes(t, workstation)
}

func assertHumanApprovalIdentity(t *testing.T, workstation factorydefinitions.FactoryWorkstationConfig) {
	t.Helper()
	if workstation.ID != "release-approval" || workstation.Type != factorydefinitions.WorkstationTypeHumanApproval || workstation.WorkerTypeName != "" {
		t.Fatalf("workstation identity = %#v, want stable HUMAN_APPROVAL without worker", workstation)
	}
}

func assertHumanApprovalDescription(t *testing.T, workstation factorydefinitions.FactoryWorkstationConfig) {
	t.Helper()
	if workstation.Description == nil || workstation.Description.Type != factorydefinitions.NameValueTypeLocalizableAsset || workstation.Description.Value != "Confirm release" {
		t.Fatalf("workstation description = %#v, want localizable Confirm release", workstation.Description)
	}
	if len(workstation.Description.Locales) != 2 || workstation.Description.Locales[1] != "fr-FR" || workstation.Description.Values["fr-FR"] != "Confirmer la version" {
		t.Fatalf("workstation localized description = %#v, want en-US/fr-FR values", workstation.Description)
	}
}

func assertHumanApprovalRoutes(t *testing.T, workstation factorydefinitions.FactoryWorkstationConfig) {
	t.Helper()
	if len(workstation.Inputs) != 1 || workstation.Inputs[0].StateName != "awaiting-approval" {
		t.Fatalf("workstation inputs = %#v, want awaiting-approval", workstation.Inputs)
	}
	if len(workstation.Outputs) != 1 || workstation.Outputs[0].StateName != "approved" {
		t.Fatalf("workstation approval outputs = %#v, want approved", workstation.Outputs)
	}
	if len(workstation.OnRejection) != 1 || workstation.OnRejection[0].StateName != "changes-requested" {
		t.Fatalf("workstation rejection outputs = %#v, want changes-requested", workstation.OnRejection)
	}
}
