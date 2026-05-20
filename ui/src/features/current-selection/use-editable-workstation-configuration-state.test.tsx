import { act, renderHook, waitFor } from "@testing-library/react";

import { semanticWorkflowDashboardSnapshot } from "../../components/dashboard/test-fixtures";
import type { CanonicalFactoryDefinition } from "../current-factory-definition";
import { useCurrentEditableFactoryDefinition } from "../current-factory-definition";
import type { DashboardSelection } from "./types";
import { useEditableWorkstationConfigurationState } from "./use-editable-workstation-configuration-state";
import { useCurrentWorkstationPromptTemplateContract } from "./useCurrentWorkstationPromptTemplateContract";

vi.mock("../current-factory-definition", async () => {
  const actual = await vi.importActual("../current-factory-definition");

  return {
    ...actual,
    useCurrentEditableFactoryDefinition: vi.fn(),
  };
});

vi.mock("./useCurrentWorkstationPromptTemplateContract", () => ({
  useCurrentWorkstationPromptTemplateContract: vi.fn(),
}));

const selectedNode =
  semanticWorkflowDashboardSnapshot.topology.workstation_nodes_by_id.review;
const selection: DashboardSelection = {
  kind: "node",
  nodeId: selectedNode.node_id,
};

describe("useEditableWorkstationConfigurationState", () => {
  beforeEach(() => {
    vi.mocked(useCurrentEditableFactoryDefinition).mockReturnValue(
      buildEditableDefinitionResult(buildEditableFactoryDefinition()),
    );
    vi.mocked(useCurrentWorkstationPromptTemplateContract).mockReturnValue(
      buildPromptTemplateContractResult(),
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
        prompt: "",
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

    vi.mocked(useCurrentEditableFactoryDefinition).mockReturnValue(
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

  it("rehydrates clean sessions from newer editable factory data", async () => {
    const { rerender, result } = renderHook(() =>
      useEditableWorkstationConfigurationState(selection, selectedNode),
    );

    await waitFor(() => {
      expect(result.current?.status).toBe("ready");
    });

    vi.mocked(useCurrentEditableFactoryDefinition).mockReturnValue(
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
          prompt: "Server refreshed prompt before local edits.",
          workerName: "reviewer",
        },
        isDirty: false,
        status: "ready",
      });
    });
  });

  it("surfaces an unavailable worker selection when the selected worker falls out of the current worker list", async () => {
    vi.mocked(useCurrentEditableFactoryDefinition).mockReturnValue(
      buildEditableDefinitionResult(
        buildEditableFactoryDefinition({
          workerName: "missing-worker",
          workerOptions: ["reviewer"],
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
});

function buildEditableDefinitionResult(
  data: CanonicalFactoryDefinition | undefined,
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
  prompt?: string;
  workerName?: string;
  workerOptions?: string[];
}): CanonicalFactoryDefinition {
  return {
    name: "Current Factory",
    workers: (overrides?.workerOptions ?? ["reviewer"]).map((name, index) => ({
      model: `gpt-5.${index + 5}`,
      name,
      type: "MODEL_WORKER",
    })),
    workstations: [
      {
        body:
          overrides?.prompt ??
          "Review the latest story changes before approval.",
        id: "review",
        inputs: [{ state: "queued", workType: "story" }],
        name: "Review",
        outputs: [{ state: "approved", workType: "story" }],
        promptFile: "prompts/review.md",
        worker: overrides?.workerName ?? "reviewer",
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
          description: "The current work item identifier.",
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
