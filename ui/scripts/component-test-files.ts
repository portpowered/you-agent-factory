import { readFileSync } from "node:fs";

export const VITEST_COMPONENT_MARKER = "@component-test-runner vitest";

const COMPONENT_TEST_PATTERNS = [
  "src/**/*.test.tsx",
  "src/**/*.component.test.ts",
];
const EXPLICIT_BUN_COMPONENT_SUFFIX = ".bun.component.test.tsx";
const PERFORMANCE_TEST_PATTERN = /(?:^|\/)performance\//;
const BUN_INCOMPATIBLE_WORKSPACE_GRAPH_FILES = new Set([
  "src/features/bento/components/dashboard-widget-header-seam.test.tsx",
  "src/features/current-selection/components/widget-tests/selection/current-selection-widget.work-contents.test.tsx",
  "src/features/current-selection/components/widget-tests/selection/work-time-presentation.locale.test.tsx",
  "src/features/current-selection/components/widget-tests/selection/current-selection-widget.isolated.bun.component.test.tsx",
  "src/features/current-selection/components/widget-tests/save/current-selection-widget.graph-draft-conflict-notifications.isolated.bun.component.test.tsx",
  "src/features/current-selection/components/widget-tests/save/current-selection-widget.save-notifications.isolated.bun.component.test.tsx",
  "src/features/current-selection/components/widget-tests/save/current-selection-widget.save.isolated.bun.component.test.tsx",
  "src/features/current-selection/work-selection/components/work-item/work-item-card.bun.component.test.tsx",
  "src/features/current-selection/work-selection/components/work-item/work-item-card.lineage-payload.test.tsx",
  "src/features/current-selection/work-selection/components/work-item/work-item-card.operation-history.test.tsx",
  "src/features/current-selection/work-selection/components/work-item/work-item-card.work-contents.test.tsx",
  "src/features/current-selection/workstation-selection/components/detail-card/workstation-detail-card.editable-configuration.bun.component.test.tsx",
  "src/features/current-selection/workstation-selection/components/detail-card/workstation-detail-card.history.test.tsx",
  "src/features/current-selection/workstation-selection/components/detail-card/workstation-detail-card.lifecycle.test.tsx",
  "src/features/current-selection/workstation-selection/components/detail-card/workstation-detail-card.localization.test.tsx",
  "src/features/current-selection/workstation-selection/components/detail-card/workstation-detail-card.test.tsx",
  "src/features/current-selection/workstation-selection/components/editable/workstation-editable-configuration-section.bun.component.test.tsx",
  "src/features/current-selection/workstation-selection/components/editable/workstation-editable-configuration-section.cron.test.tsx",
  "src/features/current-selection/workstation-selection/components/editable/workstation-editable-configuration-section.overwrite.test.tsx",
  "src/features/current-selection/workstation-selection/components/workstation-model-invoke-fields.test.tsx",
  "src/features/current-selection/worker-selection/components/worker-detail-card.isolated.bun.component.test.tsx",
  "src/features/factory-emulator/components/customer-factory-emulator-demos-lifecycle.test.tsx",
  "src/features/factory-emulator/components/customer-factory-emulator-demos-submission.test.tsx",
  "src/features/factory-emulator/components/customer-factory-emulator-demos.test.tsx",
  "src/features/factory-graph-editor/components/chrome/factory-graph-editor-work-state-phase-legend.test.tsx",
  "src/features/factory-graph-editor/components/controls/factory-graph-editor-controls-visual-groups.test.tsx",
  "src/features/factory-graph-editor/components/controls/factory-graph-editor-controls.bun.component.test.tsx",
  "src/features/factory-graph-editor/components/controls/factory-graph-editor-toolbar-sizing.test.tsx",
  "src/features/factory-graph-editor/components/factory-graph-editor-toolbar-icons.test.tsx",
  "src/features/factory-graph-editor/components/flow/factory-graph-editor-flow.progress-outcome-routes.test.tsx",
  "src/features/factory-graph-editor/hooks/use-editable-factory-graph.document-plane.test.tsx",
  "src/features/factory-graph-editor/hooks/use-editable-factory-graph.snapshot-overlay.test.tsx",
  "src/features/factory-graph-editor/hooks/use-editable-factory-graph.worker-assignment.test.tsx",
  "src/features/factory-graph-editor/hooks/validation/use-factory-validation.bun.component.test.tsx",
  "src/features/flowchart/components/current-activity-doc-node.test.tsx",
  "src/features/flowchart/components/current-activity-node-chrome.test.tsx",
  "src/features/flowchart/components/current-activity-work-type-node.test.tsx",
  "src/features/flowchart/components/graph-semantic-icon.test.tsx",
  "src/features/graphs/components/work-relation-node.test.tsx",
  "src/features/submit-work/components/submit-work-widget.isolated.bun.component.test.tsx",
  "src/features/terminal-work/components/terminal-work-card.test.tsx",
  "src/features/terminal-work/components/terminal-work-widget.test.tsx",
  "src/features/trace-drilldown/components/trace-factory-graph-nodes.test.tsx",
  "src/features/trace-drilldown/components/trace-grid-card/trace-grid-card-scroll.bun.component.test.tsx",
  "src/features/trace-drilldown/components/trace-grid-card/trace-grid-card.bun.component.test.tsx",
  "src/features/trace-drilldown/components/trace-relation-factory-graph-nodes.test.tsx",
  "src/features/work-outcome/components/d3-information-card.bun.component.test.tsx",
  "src/features/work-outcome/components/work-chart/contracts/work-chart-interactions.bun.component.test.tsx",
  "src/features/work-outcome/components/work-chart/contracts/work-chart-legend.bun.component.test.tsx",
  "src/features/work-outcome/components/work-chart/contracts/work-chart-series-and-states.bun.component.test.tsx",
  "src/features/workflow-activity/components/activity-card/react-flow-current-activity-card-editor-chrome.test.tsx",
  "src/features/workflow-activity/components/current-activity-card/react-flow-current-activity-card-graph-semantics.test.tsx",
  "src/features/workflow-activity/components/dashboard-flow-axis-legend.test.tsx",
  "src/features/workflow-activity/hooks/react-flow-current-activity-card-graph-view-model.component.test.ts",
]);

const VITEST_COMPATIBILITY_PATTERNS: Array<{
  pattern: RegExp;
  reason: string;
}> = [
  {
    pattern: /@vitest-environment/,
    reason: "declares a Vitest environment",
  },
];
const BUN_NATIVE_VI_APIS = new Set(["fn", "mocked"]);
const BUN_SHIMMED_VI_APIS = new Set([
  "clearAllMocks",
  "restoreAllMocks",
  "spyOn",
  "stubGlobal",
  "unstubAllGlobals",
]);

function findUnsupportedViApis(source: string): string[] {
  const usesBunViShim = /import\s*\{\s*bunVi\s+as\s+vi\s*\}/.test(source);
  return [
    ...new Set(
      [...source.matchAll(/\bvi\.([A-Za-z_]+)/g)]
        .map((match) => match[1])
        .filter(
          (api) =>
            !BUN_NATIVE_VI_APIS.has(api) &&
            !(usesBunViShim && BUN_SHIMMED_VI_APIS.has(api)),
        ),
    ),
  ].sort();
}

export interface ClassifiedComponentTestFile {
  path: string;
  reason: string;
  runner: "bun" | "vitest";
}

export function classifyComponentTestSource(
  path: string,
  source: string,
): ClassifiedComponentTestFile {
  const normalizedPath = path.replaceAll("\\", "/");
  if (BUN_INCOMPATIBLE_WORKSPACE_GRAPH_FILES.has(normalizedPath)) {
    return {
      path,
      reason:
        "imports workspace graph packages that Bun resolves through declaration files",
      runner: "vitest",
    };
  }

  if (path.endsWith(EXPLICIT_BUN_COMPONENT_SUFFIX)) {
    return {
      path,
      reason: "explicit native Bun component test",
      runner: "bun",
    };
  }

  if (source.includes(VITEST_COMPONENT_MARKER)) {
    return {
      path,
      reason: "explicitly documented Vitest compatibility exception",
      runner: "vitest",
    };
  }

  for (const compatibility of VITEST_COMPATIBILITY_PATTERNS) {
    if (compatibility.pattern.test(source)) {
      return {
        path,
        reason: compatibility.reason,
        runner: "vitest",
      };
    }
  }

  const unsupportedViApis = findUnsupportedViApis(source);
  if (unsupportedViApis.length > 0) {
    return {
      path,
      reason: `uses unsupported Vitest APIs: ${unsupportedViApis.join(", ")}`,
      runner: "vitest",
    };
  }

  return {
    path,
    reason: "browserless component test without Vitest-only APIs",
    runner: "bun",
  };
}

export function listComponentTestFiles(
  cwd = process.cwd(),
): ClassifiedComponentTestFile[] {
  const paths = new Set<string>();
  for (const pattern of COMPONENT_TEST_PATTERNS) {
    const glob = new Bun.Glob(pattern);
    for (const path of glob.scanSync({ cwd })) {
      if (!PERFORMANCE_TEST_PATTERN.test(path)) {
        paths.add(path);
      }
    }
  }

  return [...paths]
    .sort()
    .map((path) =>
      classifyComponentTestSource(path, readFileSync(`${cwd}/${path}`, "utf8")),
    );
}
