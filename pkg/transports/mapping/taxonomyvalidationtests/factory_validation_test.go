package apisurface_test

import (
	"bytes"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func TestFactoryValidationTargetToAPI_PreservesOperationalTargetAndSafeDefaults(t *testing.T) {
	t.Parallel()

	operational := apisurface.FactoryValidationTargetToAPI(
		factorydefinitions.ValidationTarget{
			Code:     "factory.session.field.required",
			Severity: factorydefinitions.ValidationSeverityError,
			Message:  "folderPath is required",
			Subject: factorydefinitions.ValidationSubject{
				Type: factorydefinitions.ValidationSubjectTypeFactory, ID: "folderPath",
				Location: factorydefinitions.ValidationSubjectLocationReference,
			},
		},
	)
	if operational.Code != "factory.session.field.required" ||
		operational.Severity != factoryapi.FactoryValidationSeverityError ||
		operational.Subject.Type != factoryapi.FactoryValidationSubjectTypeFactory ||
		operational.Subject.Id != "folderPath" ||
		operational.Subject.Location != factoryapi.FactoryValidationSubjectLocationReference {
		t.Fatalf("mapped operational target = %#v", operational)
	}

	unknown := apisurface.FactoryValidationTargetToAPI(factorydefinitions.ValidationTarget{
		Severity: "future-severity",
		Subject: factorydefinitions.ValidationSubject{
			Type:     "FUTURE_SUBJECT",
			Location: "FUTURE_LOCATION",
		},
	})
	if unknown.Severity != factoryapi.FactoryValidationSeverityError ||
		unknown.Subject.Type != factoryapi.FactoryValidationSubjectTypeFactory ||
		unknown.Subject.Location != factoryapi.FactoryValidationSubjectLocationDefinition {
		t.Fatalf("mapped unknown target defaults = %#v", unknown)
	}
}

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

func TestFactoryRuntimeTaxonomySummary_LegacyPollerBehaviorWithoutType(t *testing.T) {
	pollerBehavior := factoryapi.WorkstationKindPoller
	factory := factoryapi.Factory{
		Workstations: &[]factoryapi.Workstation{{
			Name:     "poll-tasks",
			Behavior: &pollerBehavior,
			Worker:   "script-poller",
		}},
	}

	entries := apisurface.FactoryRuntimeTaxonomySummary(factory)
	if len(entries) != 1 {
		t.Fatalf("entries = %#v, want one workstation taxonomy line", entries)
	}
	if entries[0].Type != "legacy poller kind" {
		t.Fatalf("workstation entry = %#v, want legacy poller kind summary", entries[0])
	}
}

func TestRenderFactoryValidationHuman_LegacyPollerBehaviorWithoutType(t *testing.T) {
	pollerBehavior := factoryapi.WorkstationKindPoller
	factory := factoryapi.Factory{
		Workstations: &[]factoryapi.Workstation{{
			Name:     "poll-tasks",
			Behavior: &pollerBehavior,
			Worker:   "script-poller",
		}},
	}

	var out bytes.Buffer
	if err := apisurface.RenderFactoryValidationHuman(factory, factoryapi.FactoryValidationResult{}, &out); err != nil {
		t.Fatalf("RenderFactoryValidationHuman: %v", err)
	}

	text := out.String()
	if !strings.Contains(text, "workstation poll-tasks: legacy poller kind (worker=script-poller)") {
		t.Fatalf("output = %q, want legacy poller taxonomy summary line", text)
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
