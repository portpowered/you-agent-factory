import { describe, expect, it } from "vitest";

import { semanticWorkflowDashboardSnapshot } from "../../../../components/dashboard/test-fixtures";
import { getWorkstationDetailMessages } from "../messages/workstation-detail";
import {
  resolveWorkstationSummaryKindValue,
  resolveWorkstationSummaryRequiresWorkerAssignment,
  resolveWorkstationSummaryRunnerValue,
  resolveWorkstationSummaryTypeValue,
} from "./workstation-summary-field-values";

const readyEditableConfigurationState = {
  draft: {
    behavior: "STANDARD" as const,
    guards: [],
    inputs: [],
    prompt: "Review",
    runnerName: null,
    workerName: "reviewer",
  },
  hasValidationErrors: false,
  initialValues: {
    behavior: "STANDARD" as const,
    behaviorOptions: ["STANDARD"],
    effectiveRunnerName: "codex",
    factoryRunnerName: null,
    prompt: "Review",
    runnerName: null,
    runnerOptions: ["codex", "gemini", "kiro", "cursor-cli", "opencode"],
    runnerSelectionSource: "default",
    resolvedRunnerSelection: {
      runnerId: "codex",
      source: "default",
    },
    sharedWorkerWorkstationNamesByWorkerName: {},
    workerModelProvider: null,
    sharedWorkerWorkstationNames: [],
    workerName: "reviewer",
    workerOptions: ["reviewer"],
    workerTypeByName: {},
    workstationName: "Review",
    workstationOptions: ["Review"],
    workstationType: "MODEL_WORKSTATION" as const,
    guards: [],
    inputs: [],
  },
  isDirty: false,
  markChangesSaved: () => {},
  baseVersion: { logical: "1", physical: "2026-01-01T00:00:00Z" },
  onBehaviorChange: () => {},
  onPromptChange: () => {},
  onResetToLatest: () => {},
  onGuardsChange: () => {},
  onInputsChange: () => {},
  onRunnerChange: () => {},
  onWorkerChange: () => {},
  workstationOptionsState: {
    options: ["Review"],
    status: "ready",
  },
  overwriteFieldNames: [],
  pendingFactoryDefinition: null,
  promptDiagnostics: [],
  promptHelpState: { status: "loading" as const },
  promptValidationState: { status: "idle" as const },
  status: "ready" as const,
  validationErrors: {},
  workerOptionsState: { options: ["reviewer"], status: "ready" as const },
};

const modelWorkstationNode =
  semanticWorkflowDashboardSnapshot.topology.workstation_nodes_by_id.review;

const logicalMoveWorkstationNode = {
  ...modelWorkstationNode,
  workstation_kind: "LOGICAL_MOVE",
};

const logicalMoveEditableConfigurationState = {
  ...readyEditableConfigurationState,
  initialValues: {
    ...readyEditableConfigurationState.initialValues,
    workstationType: "LOGICAL_MOVE" as const,
  },
};

describe("resolveWorkstationSummaryRequiresWorkerAssignment", () => {
  it("returns false for ready LOGICAL_MOVE editable configuration", () => {
    expect(
      resolveWorkstationSummaryRequiresWorkerAssignment(
        logicalMoveEditableConfigurationState,
        logicalMoveWorkstationNode,
      ),
    ).toBe(false);
  });

  it("returns true for ready model workstation editable configuration", () => {
    expect(
      resolveWorkstationSummaryRequiresWorkerAssignment(
        readyEditableConfigurationState,
        modelWorkstationNode,
      ),
    ).toBe(true);
  });

  it("returns false when topology kind is LOGICAL_MOVE before editable configuration is ready", () => {
    expect(
      resolveWorkstationSummaryRequiresWorkerAssignment(
        { status: "loading" },
        logicalMoveWorkstationNode,
      ),
    ).toBe(false);
  });
});

describe("resolveWorkstationSummaryTypeValue", () => {
  const messages = getWorkstationDetailMessages("en");

  it("localizes the authoritative workstation type when editable configuration is ready", () => {
    expect(
      resolveWorkstationSummaryTypeValue(
        readyEditableConfigurationState,
        messages,
      ),
    ).toBe("Model workstation");
  });

  it("returns loading and unavailable copy for non-ready editable configuration states", () => {
    expect(
      resolveWorkstationSummaryTypeValue({ status: "loading" }, messages),
    ).toBe("Loading workstation type...");
    expect(
      resolveWorkstationSummaryTypeValue(
        { errorMessage: "Factory unavailable.", status: "error" },
        messages,
      ),
    ).toBe("Workstation type unavailable");
    expect(resolveWorkstationSummaryTypeValue(undefined, messages)).toBe(
      "Loading workstation type...",
    );
  });

  it("localizes LOGICAL_MOVE workstation type when editable configuration is ready", () => {
    expect(
      resolveWorkstationSummaryTypeValue(
        logicalMoveEditableConfigurationState,
        messages,
      ),
    ).toBe("Logical move");
  });
});

describe("resolveWorkstationSummaryKindValue", () => {
  const messages = getWorkstationDetailMessages("en");
  const selectedNode =
    semanticWorkflowDashboardSnapshot.topology.workstation_nodes_by_id.review;

  it("localizes draft scheduling kind when editable configuration is ready", () => {
    expect(
      resolveWorkstationSummaryKindValue(
        {
          ...readyEditableConfigurationState,
          draft: {
            ...readyEditableConfigurationState.draft,
            behavior: "REPEATER",
          },
        },
        selectedNode,
        messages,
      ),
    ).toBe("Repeater");
  });

  it("ignores stale topology kind when editable configuration is ready", () => {
    expect(
      resolveWorkstationSummaryKindValue(
        readyEditableConfigurationState,
        {
          ...selectedNode,
          workstation_kind: "future-kind",
        },
        messages,
      ),
    ).toBe("Standard");
  });

  it("returns loading and unavailable copy for non-ready editable configuration states", () => {
    expect(
      resolveWorkstationSummaryKindValue(
        { status: "loading" },
        selectedNode,
        messages,
      ),
    ).toBe("Loading workstation kind...");
    expect(
      resolveWorkstationSummaryKindValue(
        { errorMessage: "Factory unavailable.", status: "error" },
        selectedNode,
        messages,
      ),
    ).toBe("Workstation kind unavailable");
  });

  it("localizes uppercase topology kinds when only topology is available", () => {
    expect(
      resolveWorkstationSummaryKindValue(
        undefined,
        {
          ...selectedNode,
          workstation_kind: "REPEATER",
        },
        messages,
      ),
    ).toBe("Repeater");
  });

  it("normalizes legacy lowercase topology kinds when only topology is available", () => {
    expect(
      resolveWorkstationSummaryKindValue(
        undefined,
        {
          ...selectedNode,
          workstation_kind: "repeater",
        },
        messages,
      ),
    ).toBe("Repeater");
  });

  it("localizes unknown topology kinds with explicit fallback text", () => {
    expect(
      resolveWorkstationSummaryKindValue(
        undefined,
        {
          ...selectedNode,
          workstation_kind: "future-kind",
        },
        getWorkstationDetailMessages("zh-CN"),
      ),
    ).toBe("未知种类：FUTURE-KIND");
  });

  it("returns null for LOGICAL_MOVE workstations", () => {
    expect(
      resolveWorkstationSummaryKindValue(
        logicalMoveEditableConfigurationState,
        logicalMoveWorkstationNode,
        getWorkstationDetailMessages("en"),
      ),
    ).toBeNull();
  });
});

describe("resolveWorkstationSummaryRunnerValue", () => {
  it("returns null for LOGICAL_MOVE workstations", () => {
    expect(
      resolveWorkstationSummaryRunnerValue(
        logicalMoveEditableConfigurationState,
        getWorkstationDetailMessages("en"),
        logicalMoveWorkstationNode,
      ),
    ).toBeNull();
  });

  it("shows localized runner display metadata with RunnerSelectionSource labels", () => {
    expect(
      resolveWorkstationSummaryRunnerValue(
        {
          draft: {
            behavior: "STANDARD",
            prompt: "Review",
            runnerName: "gemini",
            workerName: "reviewer",
          },
          initialValues: {
            ...readyEditableConfigurationState.initialValues,
            factoryRunnerName: "codex",
            runnerName: "gemini",
            resolvedRunnerSelection: {
              runnerId: "gemini",
              source: "workstation",
            },
            runnerSelectionSource: "workstation",
          },
          status: "ready",
        } as never,
        getWorkstationDetailMessages("en"),
        modelWorkstationNode,
      ),
    ).toBe("Gemini (Workstation)");
  });

  it("uses legacy_provider when inheriting from worker modelProvider", () => {
    expect(
      resolveWorkstationSummaryRunnerValue(
        {
          draft: {
            behavior: "STANDARD",
            prompt: "Review",
            runnerName: null,
            workerName: "reviewer",
          },
          initialValues: {
            ...readyEditableConfigurationState.initialValues,
            factoryRunnerName: null,
            runnerName: null,
            resolvedRunnerSelection: {
              runnerId: "codex",
              source: "legacy_provider",
            },
            runnerSelectionSource: "legacy_provider",
            workerModelProvider: "codex",
          },
          status: "ready",
        } as never,
        getWorkstationDetailMessages("en"),
        modelWorkstationNode,
      ),
    ).toBe("Codex (Legacy provider)");
  });

  it("returns unavailable copy for invalid runner ids", () => {
    const messages = getWorkstationDetailMessages("en");

    expect(
      resolveWorkstationSummaryRunnerValue(
        {
          draft: {
            behavior: "STANDARD",
            prompt: "Review",
            runnerName: "claude",
            workerName: "reviewer",
          },
          initialValues: readyEditableConfigurationState.initialValues,
          status: "ready",
        } as never,
        messages,
        modelWorkstationNode,
      ),
    ).toBe(messages.unavailableRunnerValue);
  });
});
