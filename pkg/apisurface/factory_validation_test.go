package apisurface_test

import (
	"bytes"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
)

func TestFactoryRuntimeTaxonomySummary_PreservesAuthoredAndLegacyValues(t *testing.T) {
	inference := factoryapi.WorkerTypeInferenceWorker
	legacyWorker := factoryapi.WorkerTypeModelWorker
	inferenceRun := factoryapi.WorkstationTypeInferenceRun
	legacyRun := factoryapi.WorkstationTypeModelInvoke

	factory := factoryapi.Factory{
		Workers: &[]factoryapi.Worker{
			{Name: "infer", Type: &inference},
			{Name: "legacy", Type: &legacyWorker},
		},
		Workstations: &[]factoryapi.Workstation{
			{Name: "infer-run", Type: &inferenceRun, Worker: "infer"},
			{Name: "legacy-run", Type: &legacyRun, Worker: "legacy"},
		},
	}

	entries := apisurface.FactoryRuntimeTaxonomySummary(factory)
	if len(entries) != 4 {
		t.Fatalf("entries = %#v, want four taxonomy lines", entries)
	}
	if entries[0].Type != "INFERENCE_WORKER" || entries[1].Type != "MODEL_WORKER" {
		t.Fatalf("worker entries = %#v, want authored worker taxonomy values", entries[:2])
	}
	if entries[2].Type != "INFERENCE_RUN" || entries[3].Type != "MODEL_INVOKE" {
		t.Fatalf("workstation entries = %#v, want authored workstation taxonomy values", entries[2:])
	}
}

func TestRenderFactoryValidationHuman_IncludesTaxonomyAndBlockingTargets(t *testing.T) {
	inference := factoryapi.WorkerTypeInferenceWorker
	inferenceRun := factoryapi.WorkstationTypeInferenceRun
	factory := factoryapi.Factory{
		Workers:      &[]factoryapi.Worker{{Name: "infer", Type: &inference}},
		Workstations: &[]factoryapi.Workstation{{Name: "infer-run", Type: &inferenceRun, Worker: "infer"}},
	}
	result := factoryapi.FactoryValidationResult{
		Targets: []factoryapi.FactoryValidationTarget{{
			Code:     "workstation-worker-behavior-compatibility",
			Severity: factoryapi.FactoryValidationSeverityError,
			Message:  `workstation "agent-with-infer" (AGENT_RUN) is an agent-run workstation but worker "infer" (INFERENCE_WORKER) is an inference worker`,
			Subject: factoryapi.FactoryValidationSubject{
				Type:     factoryapi.FactoryValidationSubjectTypeWorkstation,
				Id:       "agent-with-infer",
				Location: factoryapi.FactoryValidationSubjectLocationReference,
			},
		}},
	}

	var out bytes.Buffer
	err := apisurface.RenderFactoryValidationHuman(factory, result, &out)
	if err == nil {
		t.Fatal("expected validation failure error")
	}

	text := out.String()
	for _, want := range []string{
		"Factory validation failed.",
		"Runtime taxonomy:",
		"worker infer: INFERENCE_WORKER",
		"workstation infer-run: INFERENCE_RUN (worker=infer)",
		"Blocking targets:",
		"agent-run",
		"INFERENCE_WORKER",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output = %q, want substring %q", text, want)
		}
	}
}
