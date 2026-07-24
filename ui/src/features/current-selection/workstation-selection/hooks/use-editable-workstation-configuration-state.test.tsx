import { act, renderHook, waitFor } from "@testing-library/react";
import type { CurrentFactoryDocument } from "../../../../api/current-factory-definition";
import { semanticWorkflowDashboardSnapshot } from "../../../../components/dashboard/test-fixtures";
import { useCurrentFactoryDocument } from "../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import type { DashboardSelection } from "../../base/state/selection-types";
import {
  hasEditableWorkstationValidationErrors,
  validateEditableWorkstationDraft,
} from "../lib/validation/editable-workstation-configuration-validation";
import { useEditableWorkstationConfigurationState as useEditableWorkstationConfigurationStateImplementation } from "./use-editable-workstation-configuration-state";
import { useCurrentWorkstationPromptTemplateContract } from "./useCurrentWorkstationPromptTemplateContract";
import { useCurrentWorkstationPromptTemplateValidation } from "./useCurrentWorkstationPromptTemplateValidation";

vi.mock(
  "../../../current-factory-definition/hooks/useCurrentFactoryDefinition",
  async () => {
    const actual = await vi.importActual(
      "../../../current-factory-definition/hooks/useCurrentFactoryDefinition",
    );

    return {
      ...actual,
      useCurrentFactoryDocument: vi.fn(),
    };
  },
);

vi.mock("./useCurrentWorkstationPromptTemplateContract", () => ({
  useCurrentWorkstationPromptTemplateContract: vi.fn(),
}));

vi.mock("./useCurrentWorkstationPromptTemplateValidation", () => ({
  useCurrentWorkstationPromptTemplateValidation: vi.fn(),
}));

const selectedNode =
  semanticWorkflowDashboardSnapshot.topology.workstation_nodes_by_id.review;
const planNode =
  semanticWorkflowDashboardSnapshot.topology.workstation_nodes_by_id.plan;
const selection: DashboardSelection = {
  kind: "node",
  nodeId: selectedNode.node_id,
};
const planSelection: DashboardSelection = {
  kind: "node",
  nodeId: planNode.node_id,
};

function useEditableWorkstationConfigurationState(
  selection: DashboardSelection | null,
  selectedNode:
    | typeof semanticWorkflowDashboardSnapshot.topology.workstation_nodes_by_id.review
    | null,
  locale?: string | null,
) {
  const currentFactoryDocument = useCurrentFactoryDocument(false) as {
    data?: CurrentFactoryDocument;
  };

  return useEditableWorkstationConfigurationStateImplementation(
    selection,
    selectedNode,
    locale,
    currentFactoryDocument.data,
  );
}

describe("useEditableWorkstationConfigurationState", () => {
  beforeEach(() => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildEditableDefinitionResult(buildEditableFactoryDefinition()),
    );
    vi.mocked(useCurrentWorkstationPromptTemplateContract).mockReturnValue(
      buildPromptTemplateContractResult(),
    );
    vi.mocked(useCurrentWorkstationPromptTemplateValidation).mockReturnValue(
      buildPromptTemplateValidationResult(),
    );
  });

  it("tracks local draft changes and validates edited fields before save", async () => {
    const { result } = renderHook(() =>
      useEditableWorkstationConfigurationState(selection, selectedNode),
    );

    await waitFor(() => {
      expect(result.current?.status).toBe("ready");
    });

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("expected editable configuration to be ready");
      }
      result.current.onPromptChange("");
      result.current.onWorkerChange("");
    });

    expect(result.current).toMatchObject({
      draft: {
        behavior: "STANDARD",
        prompt: "",
        runnerName: "gemini",
        workerName: "",
      },
      hasValidationErrors: true,
      isDirty: true,
      status: "ready",
      validationErrors: {
        prompt: "Enter a prompt before saving this workstation.",
        workerName: "Select a worker before saving this workstation.",
      },
    });
    expect(
      result.current?.status === "ready"
        ? result.current.pendingFactoryDefinition
        : undefined,
    ).toBeNull();
  });

  it("blocks save when the workstation name duplicates another workstation", async () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildEditableDefinitionResult(
        buildMultiWorkstationEditableFactoryDefinition(),
      ),
    );

    const { result } = renderHook(() =>
      useEditableWorkstationConfigurationState(selection, selectedNode),
    );

    await waitFor(() => {
      expect(result.current?.status).toBe("ready");
    });

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("expected editable configuration to be ready");
      }
      result.current.onNameChange("Plan");
    });

    expect(result.current).toMatchObject({
      status: "ready",
      hasValidationErrors: true,
      isDirty: true,
      validationErrors: {
        name: expect.stringContaining("Plan"),
      },
    });
    expect(
      result.current?.status === "ready"
        ? result.current.pendingFactoryDefinition
        : undefined,
    ).toBeNull();
  });

  it("returns editable empty and validation messages for the selected locale", async () => {
    const { rerender, result } = renderHook(
      ({ locale }: { locale: string }) =>
        useEditableWorkstationConfigurationState(
          selection,
          selectedNode,
          locale,
        ),
      { initialProps: { locale: "zh-CN" } },
    );

    await waitFor(() => {
      expect(result.current?.status).toBe("ready");
    });

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("expected editable configuration to be ready");
      }
      result.current.onPromptChange("");
      result.current.onWorkerChange("");
    });

    expect(result.current).toMatchObject({
      hasValidationErrors: true,
      status: "ready",
      validationErrors: {
        prompt: "保存此工作站前请输入提示词。",
        workerName: "保存此工作站前请选择工作器。",
      },
    });

    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildEditableDefinitionResult({
        ...buildEditableFactoryDefinition(),
        workstations: [],
      }),
    );
    rerender({ locale: "zh-CN" });

    await waitFor(() => {
      expect(result.current).toMatchObject({
        message:
          "运行中的工厂定义没有为所选工作站公开可编辑的 worker 和 prompt 值。",
        status: "empty",
      });
    });
  });

  it("reports loading while the event-backed factory document is unavailable", async () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: undefined,
      error: { message: "Current factory definition failed to load." },
      isError: true,
      isPending: false,
      isSuccess: false,
      status: "error",
    } as never);

    const { result } = renderHook(() =>
      useEditableWorkstationConfigurationState(selection, selectedNode),
    );

    await waitFor(() => {
      expect(result.current).toEqual({ status: "loading" });
    });
  });

  it("rehydrates clean sessions from newer editable factory data", async () => {
    const { rerender, result } = renderHook(() =>
      useEditableWorkstationConfigurationState(selection, selectedNode),
    );

    await waitFor(() => {
      expect(result.current?.status).toBe("ready");
    });

    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildEditableDefinitionResult(
        buildEditableFactoryDefinition({
          prompt: "Server refreshed prompt before local edits.",
        }),
      ),
    );

    rerender();

    await waitFor(() => {
      expect(result.current).toMatchObject({
        draft: {
          behavior: "STANDARD",
          prompt: "Server refreshed prompt before local edits.",
          workerName: "reviewer",
        },
        isDirty: false,
        status: "ready",
      });
    });
  });

  it("keeps dirty drafts local while tracking newer server-backed field changes", async () => {
    const { rerender, result } = renderHook(() =>
      useEditableWorkstationConfigurationState(selection, selectedNode),
    );

    await waitFor(() => {
      expect(result.current?.status).toBe("ready");
    });

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("expected editable configuration to be ready");
      }
      result.current.onPromptChange("Keep this local prompt draft.");
    });

    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildEditableDefinitionResult(
        buildEditableFactoryDefinition({
          prompt: "Server refreshed prompt before local save.",
          workerName: "planner",
          workerOptions: [
            { name: "planner", type: "MODEL_WORKER" },
            { name: "reviewer", type: "MODEL_WORKER" },
          ],
        }),
      ),
    );

    rerender();

    await waitFor(() => {
      expect(result.current).toMatchObject({
        draft: {
          prompt: "Keep this local prompt draft.",
          workerName: "reviewer",
        },
        isDirty: true,
        overwriteFieldNames: ["prompt", "worker"],
        status: "ready",
      });
    });
  });

  it("resets dirty drafts to the latest server-backed values", async () => {
    const { rerender, result } = renderHook(() =>
      useEditableWorkstationConfigurationState(selection, selectedNode),
    );

    await waitFor(() => {
      expect(result.current?.status).toBe("ready");
    });

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("expected editable configuration to be ready");
      }
      result.current.onPromptChange("Keep this local prompt draft.");
    });

    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildEditableDefinitionResult(
        buildEditableFactoryDefinition({
          prompt: "Server refreshed prompt before local save.",
          workerName: "planner",
          workerOptions: [
            { name: "planner", type: "MODEL_WORKER" },
            { name: "reviewer", type: "MODEL_WORKER" },
          ],
        }),
      ),
    );

    rerender();

    await waitFor(() => {
      expect(result.current).toMatchObject({
        isDirty: true,
        overwriteFieldNames: ["prompt", "worker"],
        status: "ready",
      });
    });

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("expected editable configuration to be ready");
      }
      result.current.onResetToLatest();
    });

    expect(result.current).toMatchObject({
      draft: {
        prompt: "Server refreshed prompt before local save.",
        workerName: "planner",
      },
      isDirty: false,
      overwriteFieldNames: [],
      status: "ready",
    });
  });

  it("resets the editable draft when the selected workstation changes", async () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildEditableDefinitionResult(
        buildMultiWorkstationEditableFactoryDefinition(),
      ),
    );

    const { rerender, result } = renderHook(
      ({
        currentSelection,
        currentNode,
      }: {
        currentNode: typeof selectedNode;
        currentSelection: DashboardSelection;
      }) =>
        useEditableWorkstationConfigurationState(currentSelection, currentNode),
      {
        initialProps: {
          currentNode: selectedNode,
          currentSelection: selection,
        },
      },
    );

    await waitFor(() => {
      expect(result.current?.status).toBe("ready");
    });

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("expected editable configuration to be ready");
      }
      result.current.onPromptChange("Keep this local review draft.");
      result.current.onWorkerChange("planner");
    });

    expect(result.current).toMatchObject({
      draft: {
        behavior: "STANDARD",
        prompt: "Keep this local review draft.",
        workerName: "planner",
      },
      isDirty: true,
      status: "ready",
    });

    rerender({
      currentNode: planNode,
      currentSelection: planSelection,
    });

    await waitFor(() => {
      expect(result.current).toMatchObject({
        draft: {
          behavior: "STANDARD",
          prompt: "Plan the implementation.",
          workerName: "planner",
        },
        initialValues: {
          behavior: "STANDARD",
          prompt: "Plan the implementation.",
          workerName: "planner",
          workstationName: "Plan",
        },
        isDirty: false,
        overwriteFieldNames: [],
        status: "ready",
      });
    });
  });

  it("blocks saving a dirty draft when the selected workstation disappears from the latest factory definition", async () => {
    const { rerender, result } = renderHook(() =>
      useEditableWorkstationConfigurationState(selection, selectedNode),
    );

    await waitFor(() => {
      expect(result.current?.status).toBe("ready");
    });

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("expected editable configuration to be ready");
      }
      result.current.onPromptChange("Keep this local prompt draft.");
    });

    expect(result.current).toMatchObject({
      draft: {
        prompt: "Keep this local prompt draft.",
      },
      isDirty: true,
      status: "ready",
    });

    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildEditableDefinitionResult(buildFactoryDefinitionWithoutReview()),
    );

    rerender();

    await waitFor(() => {
      expect(result.current).toMatchObject({
        message:
          "This running factory definition does not expose editable worker and prompt values for the selected workstation.",
        status: "empty",
      });
    });
  });

  it("surfaces an unavailable worker selection when the selected worker falls out of the current worker list", async () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildEditableDefinitionResult(
        buildEditableFactoryDefinition({
          workerName: "missing-worker",
          workerOptions: [{ name: "reviewer", type: "MODEL_WORKER" }],
        }),
      ),
    );

    const { result } = renderHook(() =>
      useEditableWorkstationConfigurationState(selection, selectedNode),
    );

    await waitFor(() => {
      expect(result.current?.status).toBe("ready");
    });

    expect(result.current).toMatchObject({
      hasValidationErrors: true,
      promptHelpState: {
        status: "ready",
      },
      status: "ready",
      validationErrors: {
        workerName:
          "The selected worker is no longer available. Choose another worker before saving this workstation.",
      },
      workerOptionsState: {
        message:
          "The selected workstation references a worker that is no longer available in the current factory definition. Reload current selection and choose another worker.",
        status: "error",
      },
    });
  });

  it("surfaces an empty worker-options state when the workstation currently exposes no workers", async () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildEditableDefinitionResult(
        buildEditableFactoryDefinition({
          workerName: "reviewer",
          workerOptions: [],
        }),
      ),
    );

    const { result } = renderHook(() =>
      useEditableWorkstationConfigurationState(selection, selectedNode),
    );

    await waitFor(() => {
      expect(result.current?.status).toBe("ready");
    });

    expect(result.current).toMatchObject({
      hasValidationErrors: true,
      status: "ready",
      workerOptionsState: {
        message:
          "No current workers are available for this workstation. Add a worker to the factory before editing this field.",
        status: "empty",
      },
    });
  });

  it("maps prompt variable help query states into the editable workstation form state", async () => {
    vi.mocked(useCurrentWorkstationPromptTemplateContract).mockReturnValue({
      data: undefined,
      error: {
        message: "Current named factory workstation not found.",
      },
      isError: true,
      isPending: false,
      status: "error",
    } as never);

    const { result } = renderHook(() =>
      useEditableWorkstationConfigurationState(selection, selectedNode),
    );

    await waitFor(() => {
      expect(result.current?.status).toBe("ready");
    });

    expect(result.current).toMatchObject({
      promptHelpState: {
        errorMessage: "Current named factory workstation not found.",
        status: "error",
      },
      status: "ready",
    });
  });

  it("uses the resolved authored workstation name for prompt help and validation", async () => {
    const aliasedSelectedNode = {
      ...selectedNode,
      transition_id: "review-transition",
      workstation_name: "Runtime Review Alias",
    };
    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildEditableDefinitionResult({
        ...buildEditableFactoryDefinition(),
        workstations: [
          {
            behavior: "STANDARD",
            body: "Review the latest story changes before approval.",
            id: "review-transition",
            inputs: [{ state: "queued", workType: "story" }],
            name: "Canonical Review",
            outputs: [{ state: "approved", workType: "story" }],
            promptFile: "prompts/review.md",
            runner: "gemini",
            worker: "reviewer",
          },
        ],
      }),
    );

    renderHook(() =>
      useEditableWorkstationConfigurationState(selection, aliasedSelectedNode),
    );

    await waitFor(() => {
      expect(
        useCurrentWorkstationPromptTemplateContract,
      ).toHaveBeenLastCalledWith("Canonical Review", true);
      expect(
        useCurrentWorkstationPromptTemplateValidation,
      ).toHaveBeenLastCalledWith(
        "Canonical Review",
        "Review the latest story changes before approval.",
        true,
      );
    });
  });

  it("surfaces authoritative prompt diagnostics and blocks saving until the draft is fixed", async () => {
    vi.mocked(useCurrentWorkstationPromptTemplateValidation).mockReturnValue({
      data: {
        diagnostics: [
          {
            endOffset: 30,
            kind: "UNAVAILABLE_VARIABLE",
            message: "Only input 0 is available.",
            path: ".Inputs[1]",
            sourceText: "(index .Inputs 1)",
            startOffset: 14,
          },
        ],
        valid: false,
      },
      error: null,
      isError: false,
      isPending: false,
      isSuccess: true,
      status: "success",
    } as never);

    const { result } = renderHook(() =>
      useEditableWorkstationConfigurationState(selection, selectedNode),
    );

    await waitFor(() => {
      expect(result.current?.status).toBe("ready");
    });

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("expected editable configuration to be ready");
      }
      result.current.onPromptChange("Use {{ (index .Inputs 1).Payload }}");
    });

    await waitFor(() => {
      expect(result.current).toMatchObject({
        hasValidationErrors: true,
        promptDiagnostics: [
          {
            kind: "UNAVAILABLE_VARIABLE",
            path: ".Inputs[1]",
          },
        ],
        promptValidationState: {
          status: "ready",
        },
        status: "ready",
        validationErrors: {
          prompt: "See prompt diagnostics below.",
        },
      });
    });
    expect(
      result.current?.status === "ready"
        ? result.current.pendingFactoryDefinition
        : undefined,
    ).toBeNull();
  });

  it("keeps loading prompt validation out of observable prompt errors", () => {
    const selectedEditableValues = {
      behavior: "STANDARD",
      behaviorOptions: ["STANDARD", "REPEATER", "POLLER"],
      effectiveRunnerName: "codex",
      factoryRunnerName: null,
      prompt: "Review the story.",
      resolvedRunnerSelection: {
        runnerId: "codex",
        source: "default",
      },
      runnerName: null,
      runnerOptions: [
        "codex",
        "gemini",
        "kiro",
        "cursor-cli",
        "opencode",
        "pi",
      ],
      runnerSelectionSource: "default",
      sharedWorkerWorkstationNamesByWorkerName: {},
      sharedWorkerWorkstationNames: [],
      workerModelProvider: null,
      workerName: "reviewer",
      workerOptions: ["reviewer"],
      workerTypeByName: {
        reviewer: "MODEL_WORKER",
      },
      workstationName: "Review",
      guards: [],
      inputs: [],
    };

    expect(
      validateEditableWorkstationDraft(
        {
          behavior: "STANDARD",
          cron: null,
          guards: [],
          inputs: [],
          name: "Review",
          prompt: "Review the story.",
          runnerName: null,
          workerName: "reviewer",
        },
        selectedEditableValues,
        { status: "loading" },
      ),
    ).toEqual({});

    expect(
      validateEditableWorkstationDraft(
        {
          behavior: "STANDARD",
          cron: null,
          guards: [],
          inputs: [],
          name: "Review",
          prompt: "Review the story.",
          runnerName: null,
          workerName: "reviewer",
        },
        selectedEditableValues,
        {
          errorMessage: "Prompt validation API unavailable.",
          status: "error",
        },
      ),
    ).toEqual({
      prompt:
        "Prompt validation unavailable. Prompt validation API unavailable.",
    });
  });
});

describe("useEditableWorkstationConfigurationState guards and cron", () => {
  beforeEach(() => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildEditableDefinitionResult(buildEditableFactoryDefinition()),
    );
    vi.mocked(useCurrentWorkstationPromptTemplateContract).mockReturnValue(
      buildPromptTemplateContractResult(),
    );
    vi.mocked(useCurrentWorkstationPromptTemplateValidation).mockReturnValue(
      buildPromptTemplateValidationResult(),
    );
  });

  it("blocks poller behavior when the selected worker is not poller-capable", async () => {
    const { result } = renderHook(() =>
      useEditableWorkstationConfigurationState(selection, selectedNode),
    );

    await waitFor(() => {
      expect(result.current?.status).toBe("ready");
    });

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("expected editable configuration to be ready");
      }
      result.current.onBehaviorChange("POLLER");
    });

    expect(result.current).toMatchObject({
      draft: {
        behavior: "POLLER",
      },
      hasValidationErrors: true,
      status: "ready",
      validationErrors: {
        behavior:
          "Poller workstations must use a script or hosted worker before saving this workstation.",
      },
    });
  });

  it("marks the draft dirty when workstation guards change", async () => {
    const { result } = renderHook(() =>
      useEditableWorkstationConfigurationState(selection, selectedNode),
    );

    await waitFor(() => {
      expect(result.current?.status).toBe("ready");
    });

    expect(
      result.current?.status === "ready" ? result.current.isDirty : true,
    ).toBe(false);

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("expected editable configuration to be ready");
      }
      result.current.onGuardsChange([
        {
          maxVisits: 2,
          type: "VISIT_COUNT",
          workstation: "Review",
        },
      ]);
    });

    expect(result.current).toMatchObject({
      draft: {
        guards: [
          {
            maxVisits: 2,
            type: "VISIT_COUNT",
            workstation: "Review",
          },
        ],
      },
      isDirty: true,
      status: "ready",
    });
  });

  it("blocks save when guard validation errors exist", async () => {
    const { result } = renderHook(() =>
      useEditableWorkstationConfigurationState(selection, selectedNode),
    );

    await waitFor(() => {
      expect(result.current?.status).toBe("ready");
    });

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("expected editable configuration to be ready");
      }
      result.current.onGuardsChange([
        {
          maxVisits: 0,
          type: "VISIT_COUNT",
          workstation: "",
        },
      ]);
    });

    expect(result.current).toMatchObject({
      hasValidationErrors: true,
      status: "ready",
      validationErrors: {
        "guards[0].maxVisits": "Max visits must be a positive whole number.",
        "guards[0].workstation":
          "Select the workstation whose visits are counted.",
      },
    });
    expect(
      result.current?.status === "ready"
        ? result.current.pendingFactoryDefinition
        : undefined,
    ).toBeNull();
  });

  it("marks the draft dirty when per-input guards change", async () => {
    const { result } = renderHook(() =>
      useEditableWorkstationConfigurationState(selection, selectedNode),
    );

    await waitFor(() => {
      expect(result.current?.status).toBe("ready");
    });

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("expected editable configuration to be ready");
      }
      result.current.onInputsChange([
        {
          guards: [{ matchInput: "planItem", type: "SAME_NAME" }],
          state: "queued",
          workType: "story",
        },
      ]);
    });

    expect(result.current).toMatchObject({
      draft: {
        inputs: [
          {
            guards: [{ matchInput: "planItem", type: "SAME_NAME" }],
            state: "queued",
            workType: "story",
          },
        ],
      },
      isDirty: true,
      status: "ready",
    });
  });

  it("skips worker and prompt validation for LOGICAL_MOVE drafts", () => {
    const logicalMoveValues = {
      behavior: "STANDARD",
      behaviorOptions: ["STANDARD", "REPEATER", "POLLER"],
      cron: null,
      effectiveRunnerName: "codex",
      factoryRunnerName: null,
      prompt: null,
      resolvedRunnerSelection: {
        runnerId: "codex",
        source: "default",
      },
      runnerName: null,
      runnerOptions: [
        "codex",
        "gemini",
        "kiro",
        "cursor-cli",
        "opencode",
        "pi",
      ],
      runnerSelectionSource: "default",
      sharedWorkerWorkstationNamesByWorkerName: {},
      sharedWorkerWorkstationNames: [],
      workerModelProvider: null,
      workerName: "legacy-missing-worker",
      workerOptions: ["reviewer"],
      workerTypeByName: {
        reviewer: "MODEL_WORKER",
      },
      workstationName: "Route",
      workstationOptions: ["Route"],
      workstationType: "LOGICAL_MOVE" as const,
      guards: [],
      inputs: [],
    };

    expect(
      validateEditableWorkstationDraft(
        {
          behavior: "STANDARD",
          cron: null,
          guards: [],
          inputs: [],
          name: "Route",
          operation: "",
          operationBindings: [],
          prompt: "",
          runnerName: null,
          workerName: "",
          workstationType: "LOGICAL_MOVE",
        },
        logicalMoveValues,
        { status: "loading" },
      ),
    ).toEqual({});
    expect(
      validateEditableWorkstationDraft(
        {
          behavior: "POLLER",
          cron: null,
          guards: [],
          inputs: [],
          name: "Route",
          operation: "",
          operationBindings: [],
          prompt: "",
          runnerName: null,
          workerName: "legacy-missing-worker",
          workstationType: "LOGICAL_MOVE",
        },
        logicalMoveValues,
        {
          errorMessage: "Prompt validation API unavailable.",
          status: "error",
        },
      ),
    ).toEqual({});
  });

  it("does not surface worker option errors for LOGICAL_MOVE with a legacy missing worker", async () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildEditableDefinitionResult(
        buildEditableFactoryDefinition({
          workerName: "legacy-missing-worker",
          workerOptions: [{ name: "reviewer", type: "MODEL_WORKER" }],
          workstationType: "LOGICAL_MOVE",
        }),
      ),
    );

    const { result } = renderHook(() =>
      useEditableWorkstationConfigurationState(selection, selectedNode),
    );

    await waitFor(() => {
      expect(result.current?.status).toBe("ready");
    });

    expect(result.current).toMatchObject({
      hasValidationErrors: false,
      status: "ready",
      validationErrors: {},
      workerOptionsState: {
        options: ["reviewer"],
        status: "ready",
      },
    });
  });

  it("keeps empty-body pollers saveable in the current-selection editor", async () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildEditableDefinitionResult(
        buildEditableFactoryDefinition({
          behavior: "POLLER",
          prompt: "",
          workerName: "linear-poller",
          workerOptions: [{ name: "linear-poller", type: "HOSTED_WORKER" }],
        }),
      ),
    );

    const { result } = renderHook(() =>
      useEditableWorkstationConfigurationState(selection, selectedNode),
    );

    await waitFor(() => {
      expect(result.current?.status).toBe("ready");
    });

    expect(result.current).toMatchObject({
      draft: {
        behavior: "POLLER",
        prompt: "",
        workerName: "linear-poller",
      },
      hasValidationErrors: false,
      status: "ready",
      validationErrors: {},
    });
    expect(
      result.current?.status === "ready"
        ? result.current.pendingFactoryDefinition
        : undefined,
    ).toMatchObject({
      workstations: [
        {
          behavior: "POLLER",
          body: "",
          worker: "linear-poller",
        },
      ],
    });
  });

  it("validates cron fields for CRON workstations and skips them for other behaviors", () => {
    const cronDraft = {
      behavior: "CRON" as const,
      cron: {
        schedule: "",
        triggerAtStart: false,
        jitter: "-1s",
        expiryWindow: "0s",
      },
      guards: [],
      inputs: [],
      name: "Cron Tick",
      prompt: "",
      runnerName: null,
      workerName: "reviewer",
    };

    const cronErrors = validateEditableWorkstationDraft(cronDraft);
    expect(cronErrors).toMatchObject({
      cronSchedule: "cron workstation requires non-empty 'schedule'",
      cronJitter: 'jitter must be a non-negative duration, got "-1s"',
      cronExpiryWindow: 'expiry_window must be a positive duration, got "0s"',
    });
    expect(hasEditableWorkstationValidationErrors(cronErrors)).toBe(true);

    const standardErrors = validateEditableWorkstationDraft({
      behavior: "STANDARD",
      cron: null,
      guards: [],
      inputs: [],
      name: "Review",
      prompt: "Review the story.",
      runnerName: null,
      workerName: "reviewer",
    });
    expect(standardErrors.cronSchedule).toBeUndefined();
    expect(standardErrors.cronJitter).toBeUndefined();
    expect(standardErrors.cronExpiryWindow).toBeUndefined();
  });

  it("builds pendingFactoryDefinition with updated cron when schedule changes", async () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildEditableDefinitionResult(
        buildEditableFactoryDefinition({
          behavior: "CRON",
          cron: {
            schedule: "0 0 * * *",
          },
          prompt: "Run daily refresh.",
        }),
      ),
    );

    const { result } = renderHook(() =>
      useEditableWorkstationConfigurationState(selection, selectedNode),
    );

    await waitFor(() => {
      expect(result.current?.status).toBe("ready");
    });

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("expected editable configuration to be ready");
      }
      result.current.onCronScheduleChange("0 9 * * *");
    });

    expect(result.current).toMatchObject({
      draft: {
        behavior: "CRON",
        cron: {
          schedule: "0 9 * * *",
        },
      },
      hasValidationErrors: false,
      isDirty: true,
      status: "ready",
    });
    expect(
      result.current?.status === "ready"
        ? result.current.pendingFactoryDefinition?.workstations?.[0]?.cron
        : undefined,
    ).toEqual({
      schedule: "0 9 * * *",
      triggerAtStart: false,
    });
  });

  it("markChangesSaved clears dirty cron edits after a successful save", async () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildEditableDefinitionResult(
        buildEditableFactoryDefinition({
          behavior: "CRON",
          cron: {
            schedule: "0 0 * * *",
          },
          prompt: "Run daily refresh.",
        }),
      ),
    );

    const { result } = renderHook(() =>
      useEditableWorkstationConfigurationState(selection, selectedNode),
    );

    await waitFor(() => {
      expect(result.current?.status).toBe("ready");
    });

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("expected editable configuration to be ready");
      }
      result.current.onCronScheduleChange("0 9 * * *");
      result.current.markChangesSaved();
    });

    expect(result.current).toMatchObject({
      draft: {
        behavior: "CRON",
        cron: {
          schedule: "0 9 * * *",
        },
      },
      isDirty: false,
      overwriteFieldNames: [],
      status: "ready",
    });
  });

  it("tracks cron overwrite fields when the running factory cron changes during an edit session", async () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildEditableDefinitionResult(
        buildEditableFactoryDefinition({
          behavior: "CRON",
          cron: {
            schedule: "0 0 * * *",
          },
          prompt: "Run daily refresh.",
        }),
      ),
    );

    const { rerender, result } = renderHook(() =>
      useEditableWorkstationConfigurationState(selection, selectedNode),
    );

    await waitFor(() => {
      expect(result.current?.status).toBe("ready");
    });

    act(() => {
      if (result.current?.status !== "ready") {
        throw new Error("expected editable configuration to be ready");
      }
      result.current.onCronScheduleChange("0 9 * * *");
    });

    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildEditableDefinitionResult(
        buildEditableFactoryDefinition({
          behavior: "CRON",
          cron: {
            jitter: "5m",
            schedule: "*/15 * * * *",
          },
          prompt: "Run daily refresh.",
        }),
      ),
    );

    rerender();

    await waitFor(() => {
      expect(result.current).toMatchObject({
        draft: {
          cron: {
            schedule: "0 9 * * *",
          },
        },
        isDirty: true,
        overwriteFieldNames: ["cronSchedule", "cronJitter"],
        status: "ready",
      });
    });
  });

  it("blocks save for CRON workstations with invalid cron settings", async () => {
    vi.mocked(useCurrentFactoryDocument).mockReturnValue(
      buildEditableDefinitionResult(
        buildEditableFactoryDefinition({
          behavior: "CRON",
          cron: {
            expiryWindow: "bad",
            jitter: "bad",
            schedule: "not a cron",
          },
          prompt: "",
        }),
      ),
    );

    const { result } = renderHook(() =>
      useEditableWorkstationConfigurationState(selection, selectedNode),
    );

    await waitFor(() => {
      expect(result.current?.status).toBe("ready");
    });

    expect(result.current).toMatchObject({
      hasValidationErrors: true,
      status: "ready",
      validationErrors: {
        cronJitter: 'jitter must be a non-negative duration, got "bad"',
        cronExpiryWindow:
          'expiry_window must be a positive duration, got "bad"',
      },
    });
    expect(
      result.current?.status === "ready"
        ? result.current.validationErrors.cronSchedule
        : undefined,
    ).toContain('invalid cron schedule "not a cron"');
    expect(
      result.current?.status === "ready"
        ? result.current.pendingFactoryDefinition
        : undefined,
    ).toBeNull();
  });
});

function buildEditableDefinitionResult(
  data: CurrentFactoryDocument | undefined,
) {
  return {
    data,
    error: null,
    isError: false,
    isPending: false,
    isSuccess: true,
    status: "success",
  } as never;
}

function buildEditableFactoryDefinition(overrides?: {
  behavior?: "STANDARD" | "REPEATER" | "POLLER" | "CRON";
  cron?: {
    expiryWindow?: string;
    jitter?: string;
    schedule: string;
    triggerAtStart?: boolean;
  };
  prompt?: string;
  runnerName?: "codex" | "gemini" | "kiro" | "cursor-cli" | "opencode" | null;
  workerName?: string;
  workerOptions?: Array<{
    name: string;
    type: "HOSTED_WORKER" | "MODEL_WORKER" | "SCRIPT_WORKER";
  }>;
  workstationType?: "MODEL_WORKSTATION" | "LOGICAL_MOVE";
}): CurrentFactoryDocument {
  return {
    name: "Current Factory",
    runner: "codex",
    version: {
      logical: "7",
      physical: "2026-05-23T15:52:00Z",
    },
    workers: (
      overrides?.workerOptions ?? [
        { name: "reviewer", type: "MODEL_WORKER" as const },
      ]
    ).map((worker, index) => ({
      model: worker.type === "MODEL_WORKER" ? `gpt-5.${index + 5}` : undefined,
      name: worker.name,
      ...(worker.type === "SCRIPT_WORKER" ? { command: "./poller.sh" } : {}),
      type: worker.type,
    })),
    workstations: [
      {
        behavior: overrides?.behavior,
        body:
          overrides?.prompt ??
          "Review the latest story changes before approval.",
        ...(overrides?.behavior === "CRON" && overrides.cron
          ? {
              cron: {
                expiryWindow: overrides.cron.expiryWindow,
                jitter: overrides.cron.jitter,
                schedule: overrides.cron.schedule,
                triggerAtStart: overrides.cron.triggerAtStart ?? false,
              },
            }
          : {}),
        id: "review",
        inputs: [{ state: "queued", workType: "story" }],
        name: "Review",
        outputs: [{ state: "approved", workType: "story" }],
        promptFile: "prompts/review.md",
        runner: overrides?.runnerName ?? "gemini",
        type: overrides?.workstationType,
        worker: overrides?.workerName ?? "reviewer",
      },
    ],
    workTypes: [],
  };
}

function buildMultiWorkstationEditableFactoryDefinition(): CurrentFactoryDocument {
  return {
    name: "Current Factory",
    runner: "codex",
    version: {
      logical: "7",
      physical: "2026-05-23T15:52:00Z",
    },
    workers: [
      {
        model: "gpt-5.5",
        name: "reviewer",
        type: "MODEL_WORKER",
      },
      {
        model: "gpt-5.6",
        name: "planner",
        type: "MODEL_WORKER",
      },
    ],
    workstations: [
      {
        body: "Review the latest story changes before approval.",
        id: "review",
        inputs: [{ state: "queued", workType: "story" }],
        name: "Review",
        outputs: [{ state: "approved", workType: "story" }],
        promptFile: "prompts/review.md",
        runner: "gemini",
        worker: "reviewer",
      },
      {
        body: "Plan the implementation.",
        id: "plan",
        inputs: [{ state: "queued", workType: "story" }],
        name: "Plan",
        outputs: [{ state: "planned", workType: "story" }],
        promptFile: "prompts/plan.md",
        runner: "codex",
        worker: "planner",
      },
    ],
    workTypes: [],
  };
}

function buildFactoryDefinitionWithoutReview(): CurrentFactoryDocument {
  return {
    name: "Current Factory",
    runner: "codex",
    version: {
      logical: "8",
      physical: "2026-05-23T15:53:00Z",
    },
    workers: [
      {
        model: "gpt-5.6",
        name: "planner",
        type: "MODEL_WORKER",
      },
    ],
    workstations: [
      {
        body: "Plan the implementation.",
        id: "plan",
        inputs: [{ state: "queued", workType: "story" }],
        name: "Plan",
        outputs: [{ state: "planned", workType: "story" }],
        promptFile: "prompts/plan.md",
        runner: "codex",
        worker: "planner",
      },
    ],
    workTypes: [],
  };
}

function buildPromptTemplateContractResult() {
  return {
    data: {
      availableVariables: [
        {
          category: "ROOT",
          description: "Current work identifier.",
          example: "{{ .WorkID }}",
          path: ".WorkID",
        },
      ],
      inputCount: 1,
      unavailableAccessPatterns: [],
    },
    error: null,
    isError: false,
    isPending: false,
    isSuccess: true,
    status: "success",
  } as never;
}

function buildPromptTemplateValidationResult() {
  return {
    data: {
      diagnostics: [],
      valid: true,
    },
    error: null,
    isError: false,
    isPending: false,
    isSuccess: true,
    status: "success",
  } as never;
}
