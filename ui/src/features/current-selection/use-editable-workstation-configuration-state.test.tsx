import { act, renderHook, waitFor } from "@testing-library/react";

import { semanticWorkflowDashboardSnapshot } from "../../components/dashboard/test-fixtures";
import type { CanonicalFactoryDefinition } from "../current-factory-definition";
import { useCurrentEditableFactoryDefinition } from "../current-factory-definition";
import type { DashboardSelection } from "./types";
import { useEditableWorkstationConfigurationState } from "./use-editable-workstation-configuration-state";

vi.mock("../current-factory-definition", async () => {
  const actual = await vi.importActual("../current-factory-definition");

  return {
    ...actual,
    useCurrentEditableFactoryDefinition: vi.fn(),
  };
});

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
      result.current.onModelChange("gpt-5.6");
      result.current.onPromptChange("");
      result.current.onPromptFileChange("   ");
    });

    expect(result.current).toMatchObject({
      draft: {
        model: "gpt-5.6",
        prompt: "",
        promptFile: "   ",
      },
      hasValidationErrors: true,
      isDirty: true,
      status: "ready",
      validationErrors: {
        prompt: "Enter a prompt before saving this workstation.",
        promptFile:
          "Template paths cannot be only whitespace. Clear the field to remove the template.",
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
      result.current.onModelChange("");
      result.current.onPromptChange("");
      result.current.onPromptFileChange("   ");
    });

    expect(result.current).toMatchObject({
      hasValidationErrors: true,
      status: "ready",
      validationErrors: {
        model: "保存此工作站前请输入模型。",
        prompt: "保存此工作站前请输入提示词。",
        promptFile: "模板路径不能只包含空白。请清空该字段以移除模板。",
      },
    });

    vi.mocked(useCurrentEditableFactoryDefinition).mockReturnValue(
      buildEditableDefinitionResult(undefined),
    );
    rerender({ locale: "zh-CN" });

    await waitFor(() => {
      expect(result.current).toMatchObject({
        message:
          "运行中的工厂定义没有为所选工作站公开可编辑的 prompt、model 和 template 值。",
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
          promptFile: "prompts/refreshed.md",
        }),
      ),
    );

    rerender();

    await waitFor(() => {
      expect(result.current).toMatchObject({
        draft: {
          prompt: "Server refreshed prompt before local edits.",
          promptFile: "prompts/refreshed.md",
        },
        isDirty: false,
        status: "ready",
      });
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
  promptFile?: string;
}): CanonicalFactoryDefinition {
  return {
    name: "Current Factory",
    workers: [
      {
        model: "gpt-5.5",
        name: "reviewer",
        type: "MODEL_WORKER",
      },
    ],
    workstations: [
      {
        body:
          overrides?.prompt ??
          "Review the latest story changes before approval.",
        id: "review",
        inputs: [{ state: "queued", workType: "story" }],
        name: "Review",
        outputs: [{ state: "approved", workType: "story" }],
        promptFile: overrides?.promptFile ?? "prompts/review.md",
        worker: "reviewer",
      },
    ],
    workTypes: [],
  };
}
