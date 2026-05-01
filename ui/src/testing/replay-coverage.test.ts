import { describe, expect, it } from "vitest";

import {
  buildReplayCoverageReport,
  formatReplayCoverageReportMarkdown,
  listBrowserIntegrationReplayScenarios,
  replayFixtureCatalog,
  validateReplayCoverageReport,
} from "./replay-fixture-catalog";

describe("replay coverage reporting", () => {
  it("builds a first-pass surface baseline with explicit remaining gaps", () => {
    const report = buildReplayCoverageReport();

    expect(report.scenarioCount).toBe(Object.keys(replayFixtureCatalog).length);
    expect(report.coveredSurfaceCount).toBe(10);
    expect(report.gapSurfaceCount).toBe(3);
    expect(report.surfaces.filter((surface) => surface.status === "gap").map((surface) => surface.id)).toEqual([
      "terminal-summary",
      "script-request-history",
      "resource-counts",
    ]);
  });

  it("formats the report with scenario visibility and per-surface ownership", () => {
    const markdown = formatReplayCoverageReportMarkdown(buildReplayCoverageReport());

    expect(markdown).toContain("## Baseline");
    expect(markdown).toContain("| `baseline` | `event-stream-replay.jsonl` | app-smoke, browser-integration | dashboard-shell, selected-work, trace-drilldown |");
    expect(markdown).toContain("| `resource-counts` | gap | none yet | Replay-driven resource-count assertions against backend world-view counts. |");
  });

  it("reuses the replay coverage catalog for browser integration scenarios", () => {
    expect(listBrowserIntegrationReplayScenarios().map((scenario) => scenario.id)).toEqual([
      "baseline",
      "runtimeConfigInterfaceConsolidation",
    ]);
  });

  it("keeps replay coverage metadata internally consistent", () => {
    expect(validateReplayCoverageReport(buildReplayCoverageReport())).toEqual([]);
  });
});
