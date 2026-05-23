import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

import { createFactory } from "../../../api/named-factory";
import type { EditableWorkstationConfigurationState } from "../components/detail-card-types";
import { useSaveEditableWorkstationConfiguration } from "./use-save-editable-workstation-configuration";

vi.mock("../../../api/named-factory", async () => {
  const actual = await vi.importActual("../../../api/named-factory");

  return {
    ...actual,
    createFactory: vi.fn(),
  };
});

describe("useSaveEditableWorkstationConfiguration", () => {
  it("uses localized fallback copy for unknown save errors", async () => {
    vi.mocked(createFactory).mockRejectedValue("network unavailable");

    const { result } = renderHook(
      () =>
        useSaveEditableWorkstationConfiguration({
          editableConfigurationState: buildReadyEditableConfigurationState(),
          locale: "zh-CN",
          scopeKey: "review:transition:Review",
        }),
      { wrapper: createQueryClientWrapper() },
    );

    act(() => {
      result.current.beginSaveConfirmation();
    });

    await act(async () => {
      await result.current.confirmSave();
    });

    await waitFor(() => {
      expect(result.current.saveState).toEqual({
        errorMessage: "无法保存运行中的工厂。",
        status: "error",
      });
    });
  });

  it("allows empty-body pollers to stay saveable", () => {
    const { result } = renderHook(
      () =>
        useSaveEditableWorkstationConfiguration({
          editableConfigurationState: buildReadyEditableConfigurationState({
            behavior: "POLLER",
            prompt: "",
          }),
          scopeKey: "review:transition:Review",
        }),
      { wrapper: createQueryClientWrapper() },
    );

    expect(result.current.canSave).toBe(true);
    expect(result.current.saveState).toEqual({ status: "idle" });
  });
});

function createQueryClientWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  });

  return function QueryClientWrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}

function buildReadyEditableConfigurationState(overrides?: {
  behavior?: "STANDARD" | "REPEATER" | "POLLER";
  prompt?: string;
}): EditableWorkstationConfigurationState {
  return {
    draft: {
      behavior: overrides?.behavior ?? "STANDARD",
      prompt: overrides?.prompt ?? "Review the story.",
      runnerName: null,
      workerName: "reviewer",
    },
    hasValidationErrors: false,
    initialValues: {
      behavior: "STANDARD",
      behaviorOptions: ["STANDARD", "REPEATER", "POLLER"],
      effectiveRunnerName: "codex",
      factoryRunnerName: null,
      prompt: overrides?.prompt ?? "Review the story.",
      runnerName: null,
      runnerOptions: ["codex"],
      workerName: "reviewer",
      workerOptions: ["reviewer"],
      workerTypeByName: {
        reviewer: "MODEL_WORKER",
      },
      workstationName: "Review",
    },
    isDirty: true,
    markChangesSaved: vi.fn(),
    onBehaviorChange: vi.fn(),
    onPromptChange: vi.fn(),
    onRunnerChange: vi.fn(),
    onWorkerChange: vi.fn(),
    overwriteFieldNames: [],
    pendingFactoryDefinition: {
      name: "Current Factory",
      workers: [],
      workstations: [],
    },
    promptDiagnostics: [],
    promptHelpState: {
      contract: {
        availableVariables: [],
        inputCount: 0,
        unavailableAccessPatterns: [],
      },
      status: "ready",
    },
    promptValidationState: {
      diagnostics: [],
      result: {
        diagnostics: [],
        valid: true,
      },
      status: "ready",
    },
    status: "ready",
    validationErrors: {},
    workerOptionsState: {
      options: ["reviewer"],
      status: "ready",
    },
  };
}
