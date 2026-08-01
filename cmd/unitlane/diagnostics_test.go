package main

import (
	"bytes"
	"testing"
)

func TestDiagnosticOutputAggregatesPackageTimingAndStableSlowOrdering(t *testing.T) {
	collector := newUnitTimingCollector()
	var output bytes.Buffer
	writer := &unitDiagnosticOutput{collector: collector, output: &output}
	events := []byte("{" +
		`"Action":"run","Package":"pkg/z","Test":"TestZ"` + "}\n" +
		`{"Action":"pass","Package":"pkg/z","Test":"TestZ","Elapsed":0.5}` + "\n" +
		`{"Action":"pass","Package":"pkg/z","Elapsed":1.5}` + "\n" +
		`{"Action":"output","Package":"pkg/a","Output":"ok pkg/a (cached)\n"}` + "\n" +
		`{"Action":"pass","Package":"pkg/a","Elapsed":1.5}` + "\n")
	if _, err := writer.Write(events); err != nil {
		t.Fatalf("diagnostic output Write() error = %v", err)
	}
	if err := writer.flush(); err != nil {
		t.Fatalf("diagnostic output flush() error = %v", err)
	}

	report := collector.report()
	if len(report.Packages) != 2 || report.Packages[0].Package != "pkg/a" || report.Packages[1].TestCount != 1 {
		t.Fatalf("package report = %+v, want alphabetic packages and one test", report.Packages)
	}
	if !report.Packages[0].Cached || !report.Packages[0].CacheObserved {
		t.Fatalf("cached package = %+v, want observed cache state", report.Packages[0])
	}
	if output.String() != "ok pkg/a (cached)\n" {
		t.Fatalf("replayed output = %q, want readable cached output", output.String())
	}

	slow := slowPackages(report.Packages)
	if len(slow) != 2 || slow[0].Package != "pkg/a" || slow[1].Package != "pkg/z" {
		t.Fatalf("slow package order = %+v, want stable elapsed-descending/package-ascending order", slow)
	}
}
