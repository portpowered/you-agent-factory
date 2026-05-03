import { describe, expect, it } from "vitest";

import {
  buildReplayCoverageReport,
  formatReplayCoverageReportMarkdown,
  listBrowserIntegrationReplayScenarios,
  replayFixtureCatalog,
  supplementalReplayCoverageCatalog,
  validateReplayCoverageReport,
} from "./replay-fixture-catalog";

describe("replay coverage reporting", () => {
  it("builds a first-pass surface baseline with explicit remaining gaps", () => {
    const report = buildReplayCoverageReport();

    expect(report.scenarioCount).toBe(
      Object.keys(replayFixtureCatalog).length + Object.keys(supplementalReplayCoverageCatalog).length,
    );
    expect(report.coveredSurfaceCount).toBe(15);
    expect(report.gapSurfaceCount).toBe(1);
    expect(report.surfaces.filter((surface) => surface.status === "gap").map((surface) => surface.id)).toEqual([
      "script-request-history",
    ]);
  });

  it("formats the report with scenario visibility and per-surface ownership", () => {
    const markdown = formatReplayCoverageReportMarkdown(buildReplayCoverageReport());

    expect(markdown).toContain("## Baseline");
    expect(markdown).toContain("| `baseline` | `event-stream-replay.jsonl` | app-smoke, browser-integration | dashboard-shell, selected-work, trace-drilldown |");
    expect(markdown).toContain("| `pngRoundTrip` | `graph-state-smoke-replay.jsonl` | browser-integration, jsdom, unit | png-export, png-import-preview, png-import-activation |");
    expect(markdown).toContain("| `weirdNumberSummary` | `weird-number-summary-replay.jsonl` | app-smoke, projection-helper | failure-rendering, resource-counts, terminal-summary |");
    expect(markdown).toContain("| `resource-counts` | covered | `weirdNumberSummary` | Replay-driven resource-count assertions against backend world-view counts. |");
  });

  it("reuses the replay coverage catalog for browser integration scenarios", () => {
    expect(listBrowserIntegrationReplayScenarios().map((scenario) => scenario.id)).toEqual([
      "baseline",
      "runtimeConfigInterfaceConsolidation",
    ]);
  });

  it("tracks the PNG browser roundtrip as layered supplemental coverage", () => {
    const scenario = buildReplayCoverageReport().scenarios.find((entry) => entry.id === "pngRoundTrip");

    expect(scenario).toEqual({
      description: "Browser export/import PNG roundtrip smoke layered on top of existing jsdom and unit PNG coverage.",
      fileName: "graph-state-smoke-replay.jsonl",
      id: "pngRoundTrip",
      surfaces: ["png-export", "png-import-preview", "png-import-activation"],
      verificationLayers: ["browser-integration", "jsdom", "unit"],
    });
  });

  it("keeps replay coverage metadata internally consistent", () => {
    expect(validateReplayCoverageReport(buildReplayCoverageReport())).toEqual([]);
  });
});

