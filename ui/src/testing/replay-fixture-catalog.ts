export const replayCoverageSurfaceCatalog = [
  {
    description: "Dashboard-shell rendering from a live `/events` replay stream.",
    id: "dashboard-shell",
  },
  {
    description: "Current work-item selection and summary cards.",
    id: "selected-work",
  },
  {
    description: "Trace drill-down rendering after selecting replayed work.",
    id: "trace-drilldown",
  },
  {
    description: "Current-selection card rendering for replayed detail states.",
    id: "current-selection",
  },
  {
    description: "Failure-path rendering for replayed failed work and provider failures.",
    id: "failure-rendering",
  },
  {
    description: "Timeline slider history and fixed-tick replay navigation.",
    id: "timeline-history",
  },
  {
    description: "Graph-state markers and node rendering driven by replayed topology events.",
    id: "graph-state",
  },
  {
    description: "Selection-history behavior across replay tick changes.",
    id: "selection-history",
  },
  {
    description: "Workspace setup and runtime-config derived projections from replay data.",
    id: "workspace-setup",
  },
  {
    description: "Runtime workstation-request details for active, completed, and failed work.",
    id: "runtime-request-details",
  },
  {
    description: "Replay-driven terminal work summaries rather than direct seeded snapshots.",
    id: "terminal-summary",
  },
  {
    description: "Replay-driven mixed script and inference request-history rendering.",
    id: "script-request-history",
  },
  {
    description: "Replay-driven resource-count assertions against backend world-view counts.",
    id: "resource-counts",
  },
  {
    description: "Browser-visible factory PNG export flow with a real downloadable artifact.",
    id: "png-export",
  },
  {
    description: "Browser-visible factory PNG import preview and validation through the drop surface.",
    id: "png-import-preview",
  },
  {
    description: "Browser-visible factory PNG activation after previewing an imported PNG.",
    id: "png-import-activation",
  },
] as const;

export type ReplayCoverageSurfaceID = (typeof replayCoverageSurfaceCatalog)[number]["id"];

export interface BrowserIntegrationReplayMetadata {
  finalTick: number;
  headingName: string;
  historicalHiddenButtonName?: RegExp;
  inFlightSelectionTick?: number;
  name: string;
  requiresWorkItemSelection: boolean;
  selectedWorkText?: string;
  workstationName: RegExp;
}

export interface ReplayFixtureScenarioDefinition {
  browserIntegration?: BrowserIntegrationReplayMetadata;
  description: string;
  fileName: string;
  id: string;
  surfaces: readonly ReplayCoverageSurfaceID[];
  verificationLayers: readonly string[];
}

export interface SupplementalReplayCoverageScenarioDefinition {
  description: string;
  fileName: string;
  id: string;
  surfaces: readonly ReplayCoverageSurfaceID[];
  verificationLayers: readonly string[];
}

export const replayFixtureCatalog = {
  baseline: {
    browserIntegration: {
      finalTick: 2,
      headingName: "Agent Factory",
      historicalHiddenButtonName: /^work-1/i,
      name: "baseline replay",
      requiresWorkItemSelection: true,
      workstationName: /^Select plan workstation$/i,
    },
    description: "Baseline dashboard event-stream replay smoke fixture.",
    fileName: "event-stream-replay.jsonl",
    id: "baseline",
    surfaces: ["dashboard-shell", "selected-work", "trace-drilldown"],
    verificationLayers: ["app-smoke", "browser-integration"],
  },
  failureAnalysis: {
    description: "Failure-focused replay fixture with queued, active, and failed work rendering.",
    fileName: "failure-analysis-replay.jsonl",
    id: "failureAnalysis",
    surfaces: ["current-selection", "failure-rendering", "timeline-history"],
    verificationLayers: ["app-smoke"],
  },
  graphStateSmoke: {
    description: "Graph-state replay fixture covering state markers, selection, and tick changes.",
    fileName: "graph-state-smoke-replay.jsonl",
    id: "graphStateSmoke",
    surfaces: ["dashboard-shell", "graph-state", "selection-history"],
    verificationLayers: ["app-smoke"],
  },
  runtimeConfigInterfaceConsolidation: {
    browserIntegration: {
      finalTick: 8,
      headingName: "Agent Factory",
      historicalHiddenButtonName: /^work-1/i,
      inFlightSelectionTick: 7,
      name: "captured replay 2",
      requiresWorkItemSelection: false,
      workstationName: /^Select process workstation$/i,
    },
    description: "Captured runtime-config replay fixture with setup-workspace projections.",
    fileName: "event-stream-replay-2.jsonl",
    id: "runtimeConfigInterfaceConsolidation",
    surfaces: ["current-selection", "trace-drilldown", "workspace-setup"],
    verificationLayers: ["browser-integration", "projection-helper"],
  },
  runtimeDetails: {
    description: "Runtime-request replay fixture with pending, completed, and failed detail rendering.",
    fileName: "runtime-details-replay.jsonl",
    id: "runtimeDetails",
    surfaces: ["current-selection", "failure-rendering", "runtime-request-details"],
    verificationLayers: ["app-smoke"],
  },
  weirdNumberSummary: {
    description: "Focused replay fixture for one failed dispatch that produces three failed work-item summaries.",
    fileName: "weird-number-summary-replay.jsonl",
    id: "weirdNumberSummary",
    surfaces: ["failure-rendering", "resource-counts", "terminal-summary"],
    verificationLayers: ["app-smoke", "projection-helper"],
  },
} as const satisfies Record<
  string,
  ReplayFixtureScenarioDefinition
>;

export const supplementalReplayCoverageCatalog = {
  pngRoundTrip: {
    description: "Browser export/import PNG roundtrip smoke layered on top of existing jsdom and unit PNG coverage.",
    fileName: "graph-state-smoke-replay.jsonl",
    id: "pngRoundTrip",
    surfaces: ["png-export", "png-import-preview", "png-import-activation"],
    verificationLayers: ["browser-integration", "jsdom", "unit"],
  },
} as const satisfies Record<string, SupplementalReplayCoverageScenarioDefinition>;

export type ReplayFixtureCatalog = typeof replayFixtureCatalog;
export type ReplayFixtureID = keyof ReplayFixtureCatalog;
export type ReplayFixtureDefinition = ReplayFixtureCatalog[ReplayFixtureID];
export type ReplayCoverageScenarioID =
  | ReplayFixtureID
  | keyof typeof supplementalReplayCoverageCatalog;
export type ReplayCoverageScenarioDefinition =
  | ReplayFixtureDefinition
  | (typeof supplementalReplayCoverageCatalog)[keyof typeof supplementalReplayCoverageCatalog];

export type BrowserIntegrationReplayScenario = ReplayFixtureDefinition & {
  browserIntegration: BrowserIntegrationReplayMetadata;
};

export interface ReplayCoverageSurfaceReport {
  description: string;
  id: ReplayCoverageSurfaceID;
  scenarios: ReplayCoverageScenarioID[];
  status: "covered" | "gap";
}

export interface ReplayCoverageScenarioReport {
  description: string;
  fileName: string;
  id: ReplayCoverageScenarioID;
  surfaces: readonly ReplayCoverageSurfaceID[];
  verificationLayers: readonly string[];
}

export interface ReplayCoverageReport {
  coveredSurfaceCount: number;
  gapSurfaceCount: number;
  scenarioCount: number;
  scenarios: ReplayCoverageScenarioReport[];
  surfaces: ReplayCoverageSurfaceReport[];
  totalTrackedSurfaceCount: number;
}

function hasBrowserIntegration(
  scenario: ReplayFixtureDefinition,
): scenario is BrowserIntegrationReplayScenario {
  return "browserIntegration" in scenario;
}

export function listBrowserIntegrationReplayScenarios(): BrowserIntegrationReplayScenario[] {
  return Object.values(replayFixtureCatalog).filter(hasBrowserIntegration);
}

function listReplayCoverageScenarios(): ReplayCoverageScenarioDefinition[] {
  return [
    ...Object.values(replayFixtureCatalog),
    ...Object.values(supplementalReplayCoverageCatalog),
  ];
}

export function buildReplayCoverageReport(): ReplayCoverageReport {
  const scenarios: ReplayCoverageScenarioReport[] = listReplayCoverageScenarios().map(
    (scenario) => ({
      description: scenario.description,
      fileName: scenario.fileName,
      id: scenario.id,
      surfaces: [...scenario.surfaces],
      verificationLayers: [...scenario.verificationLayers],
    }),
  );

  const surfaces = replayCoverageSurfaceCatalog.map((surface) => {
    const coveringScenarios = scenarios
      .filter((scenario) => scenario.surfaces.includes(surface.id))
      .map((scenario) => scenario.id);

    return {
      description: surface.description,
      id: surface.id,
      scenarios: coveringScenarios,
      status: coveringScenarios.length > 0 ? "covered" : "gap",
    } satisfies ReplayCoverageSurfaceReport;
  });

  const coveredSurfaceCount = surfaces.filter((surface) => surface.status === "covered").length;

  return {
    coveredSurfaceCount,
    gapSurfaceCount: surfaces.length - coveredSurfaceCount,
    scenarioCount: scenarios.length,
    scenarios,
    surfaces,
    totalTrackedSurfaceCount: surfaces.length,
  };
}

export function formatReplayCoverageReportMarkdown(report: ReplayCoverageReport): string {
  const lines = [
    "# Agent Factory UI Replay Coverage",
    "",
    "This artifact is generated from `src/testing/replay-fixture-catalog.ts`.",
    "",
    "## Baseline",
    `- Tracked replay scenarios: ${report.scenarioCount}`,
    `- Covered tracked surfaces: ${report.coveredSurfaceCount} / ${report.totalTrackedSurfaceCount}`,
    `- Remaining tracked gaps: ${report.gapSurfaceCount}`,
    "",
    "## Scenarios",
    "| Scenario | Fixture | Verification layers | Covered surfaces |",
    "| --- | --- | --- | --- |",
    ...report.scenarios.map(
      (scenario) =>
        `| \`${scenario.id}\` | \`${scenario.fileName}\` | ${scenario.verificationLayers.join(", ")} | ${scenario.surfaces.join(", ")} |`,
    ),
    "",
    "## Surface Matrix",
    "| Surface | Status | Covered by | Notes |",
    "| --- | --- | --- | --- |",
    ...report.surfaces.map((surface) => {
      const coveredBy = surface.scenarios.length > 0 ? surface.scenarios.map((scenario) => `\`${scenario}\``).join(", ") : "none yet";
      return `| \`${surface.id}\` | ${surface.status} | ${coveredBy} | ${surface.description} |`;
    }),
    "",
  ];

  return `${lines.join("\n")}\n`;
}

export function validateReplayCoverageReport(report: ReplayCoverageReport): string[] {
  const issues: string[] = [];
  const scenarioIDs = new Set(report.scenarios.map((scenario) => scenario.id));
  const surfaceIDs = new Set(replayCoverageSurfaceCatalog.map((surface) => surface.id));

  for (const scenario of report.scenarios) {
    if (scenario.verificationLayers.length === 0) {
      issues.push(`Scenario '${scenario.id}' is missing verification layers.`);
    }
    if (scenario.surfaces.length === 0) {
      issues.push(`Scenario '${scenario.id}' is missing covered surfaces.`);
    }
    for (const surfaceID of scenario.surfaces) {
      if (!surfaceIDs.has(surfaceID)) {
        issues.push(`Scenario '${scenario.id}' references unknown surface '${surfaceID}'.`);
      }
    }
  }

  for (const surface of report.surfaces) {
    const expectedStatus = surface.scenarios.length > 0 ? "covered" : "gap";
    if (surface.status !== expectedStatus) {
      issues.push(`Surface '${surface.id}' has status '${surface.status}' but should be '${expectedStatus}'.`);
    }
    for (const scenarioID of surface.scenarios) {
      if (!scenarioIDs.has(scenarioID)) {
        issues.push(`Surface '${surface.id}' references unknown scenario '${scenarioID}'.`);
      }
    }
  }

  return issues;
}
