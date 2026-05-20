import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

import { createFactory } from "../../api/named-factory";
import type { EditableWorkstationConfigurationState } from "./detail-card-types";
import { useSaveEditableWorkstationConfiguration } from "./use-save-editable-workstation-configuration";

vi.mock("../../api/named-factory", async () => {
  const actual = await vi.importActual("../../api/named-factory");

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

function buildReadyEditableConfigurationState(): EditableWorkstationConfigurationState {
  return {
    draft: {
      model: "gpt-5.5",
      prompt: "Review the story.",
      promptFile: "prompts/review.md",
    },
    hasValidationErrors: false,
    initialValues: {
      isModelEditable: true,
      model: "gpt-5.5",
      modelEditBlockedReason: null,
      prompt: "Review the story.",
      promptFile: "prompts/review.md",
      workerName: "reviewer",
      workstationName: "Review",
    },
    isDirty: true,
    isModelEditable: true,
    markChangesSaved: vi.fn(),
    onModelChange: vi.fn(),
    onPromptChange: vi.fn(),
    onPromptFileChange: vi.fn(),
    overwriteFieldNames: [],
    pendingFactoryDefinition: {
      name: "Current Factory",
      workers: [],
      workstations: [],
    },
    status: "ready",
    validationErrors: {},
  };
}
