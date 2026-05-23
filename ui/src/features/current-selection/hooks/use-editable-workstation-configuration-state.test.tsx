import { act, renderHook, waitFor } from "@testing-library/react";

import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import type { CurrentFactoryDocument } from "../../../api/current-factory-definition";
import { useCurrentFactoryDocument } from "../../current-factory-definition";
import type { DashboardSelection } from "../types";
import {
  useEditableWorkstationConfigurationState,
  validateEditableWorkstationDraft,
} from "./use-editable-workstation-configuration-state";
import { useCurrentWorkstationPromptTemplateContract } from "./useCurrentWorkstationPromptTemplateContract";
import { useCurrentWorkstationPromptTemplateValidation } from "./useCurrentWorkstationPromptTemplateValidation";

vi.mock("../../current-factory-definition", async () => {
  const actual = await vi.importActual("../../current-factory-definition");

  return {
    ...actual,
    useCurrentFactoryDocument: vi.fn(),
  };
});

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
      buildEditableDefinitionResult(undefined),
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

  it("surfaces editable-definition load failures directly in the form state", async () => {
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
      expect(result.current).toMatchObject({
        errorMessage: "Current factory definition failed to load.",
        status: "error",
      });
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
          prompt:
            "Resolve the highlighted prompt diagnostics before saving this workstation.",
        },
      });
    });
    expect(
      result.current?.status === "ready"
        ? result.current.pendingFactoryDefinition
        : undefined,
    ).toBeNull();
  });

  it("formats loading and error prompt validation states into observable prompt errors", () => {
    const selectedEditableValues = {
      behavior: "STANDARD",
      behaviorOptions: ["STANDARD", "REPEATER", "POLLER"],
      effectiveRunnerName: "codex",
      factoryRunnerName: null,
      prompt: "Review the story.",
      runnerName: null,
      runnerOptions: ["codex"],
      workerName: "reviewer",
      workerOptions: ["reviewer"],
      workerTypeByName: {
        reviewer: "MODEL_WORKER",
      },
      workstationName: "Review",
    };

    expect(
      validateEditableWorkstationDraft(
        {
          behavior: "STANDARD",
          prompt: "Review the story.",
          runnerName: null,
          workerName: "reviewer",
        },
        selectedEditableValues,
        { status: "loading" },
      ),
    ).toEqual({
      prompt: "Validating prompt variables for the current draft.",
    });

    expect(
      validateEditableWorkstationDraft(
        {
          behavior: "STANDARD",
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
  prompt?: string;
  runnerName?: "codex" | "gemini" | "kiro" | "cursor-cli" | "opencode" | null;
  workerName?: string;
  workerOptions?: Array<{
    name: string;
    type: "HOSTED_WORKER" | "MODEL_WORKER" | "SCRIPT_WORKER";
  }>;
}): CurrentFactoryDocument {
  return {
    name: "Current Factory",
    runner: "codex",
    version: {
      logical: 7,
      physical: "2026-05-23T15:52:00Z",
    },
    workers: (overrides?.workerOptions ?? [
      { name: "reviewer", type: "MODEL_WORKER" as const },
    ]).map((worker, index) => ({
      model:
        worker.type === "MODEL_WORKER" ? `gpt-5.${index + 5}` : undefined,
      name: worker.name,
      ...(worker.type === "SCRIPT_WORKER"
        ? { command: "./poller.sh" }
        : {}),
      type: worker.type,
    })),
    workstations: [
      {
        behavior: overrides?.behavior,
        body:
          overrides?.prompt ??
          "Review the latest story changes before approval.",
        id: "review",
        inputs: [{ state: "queued", workType: "story" }],
        name: "Review",
        outputs: [{ state: "approved", workType: "story" }],
        promptFile: "prompts/review.md",
        runner: overrides?.runnerName ?? "gemini",
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
      logical: 7,
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
