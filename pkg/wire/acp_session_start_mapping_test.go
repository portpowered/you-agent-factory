package wire

import (
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

func TestMapACPFactorySessionStart(t *testing.T) {
	artifacts := factoryruntime.RuntimeArtifactRoots{Logs: "/home/op/logs", Metrics: "/home/op/metrics"}
	defaults := operatorsettings.ResolvedDefaults{WorkerModelProvider: "acme", WorkerModel: "widget", ConfigPath: "/home/op/config.yaml"}
	got := mapACPFactorySessionStart("req-1", "factory:review", "/work/root", "/factories/review", "/home/op", artifacts, defaults)

	if got.Mode != factorysessions.SessionOperationModeLive {
		t.Fatalf("Mode = %q, want live", got.Mode)
	}
	if !got.ActivationOnly {
		t.Fatal("ActivationOnly = false, want true")
	}
	if got.Correlation.RequestID != "req-1" {
		t.Fatalf("Correlation.RequestID = %q, want req-1", got.Correlation.RequestID)
	}
	if got.Definition.FactoryID != "factory:review" {
		t.Fatalf("Definition.FactoryID = %q", got.Definition.FactoryID)
	}
	if got.Source.FactoryID != "factory:review" || got.Source.Kind != factoryruntime.WorkflowSourceKindFactoryID {
		t.Fatalf("Source = %+v, want factory ID factory:review", got.Source)
	}
	if got.FolderPath != "/factories/review" {
		t.Fatalf("FolderPath = %q", got.FolderPath)
	}
	if got.Args["workingRoot"] != "/work/root" {
		t.Fatalf("Args = %#v, want workingRoot /work/root", got.Args)
	}
	if got.RuntimeSelection == nil {
		t.Fatal("RuntimeSelection is nil")
	}
	rs := got.RuntimeSelection
	if rs.SystemConfigHome != "/home/op" || rs.LogDirectory != "/home/op/logs" || rs.MetricsDirectory != "/home/op/metrics" {
		t.Fatalf("RuntimeSelection home/log/metrics = %+v", rs)
	}
	if rs.OperatorDefaults != defaults {
		t.Fatalf("OperatorDefaults = %+v, want %+v", rs.OperatorDefaults, defaults)
	}
	if rs.Mode != factorysessions.SessionRuntimeModeService {
		t.Fatalf("RuntimeSelection.Mode = %q, want SERVICE", rs.Mode)
	}
}

func TestMapACPFactorySessionStartStableRequestID(t *testing.T) {
	artifacts := factoryruntime.RuntimeArtifactRoots{}
	defaults := operatorsettings.ResolvedDefaults{}
	first := mapACPFactorySessionStart("req-stable", "factory:a", "/w", "/f", "/h", artifacts, defaults)
	second := mapACPFactorySessionStart("req-stable", "factory:a", "/w", "/f", "/h", artifacts, defaults)
	if first.Correlation.RequestID != second.Correlation.RequestID || first.Correlation.RequestID != "req-stable" {
		t.Fatalf("request IDs not stable: %q vs %q", first.Correlation.RequestID, second.Correlation.RequestID)
	}
}
