import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

import { CurrentFactoryDefinitionError } from "../../../../api/current-factory-definition";
import {
  mockFactoryDocumentSave,
  mockPendingFactoryDocumentSave,
} from "../../../../testing/factory-document-save-mocks";
import { workerFieldValidationTarget } from "../../../../testing/factory-validation-target-fixtures";
import * as currentFactoryFeature from "../../../current-factory-definition/public";
import { useDashboardSessionStore } from "../../../dashboard/state/dashboardSessionStore";
import type { EditableWorkerConfigurationState } from "../lib/detail-card-types";
import { useSaveEditableWorkerConfiguration } from "./use-save-editable-worker-configuration";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: focused save-hook regressions share one mocked mutation seam.
describe("useSaveEditableWorkerConfiguration", () => {
  beforeEach(() => {
    useDashboardSessionStore.setState({ selectedSessionID: "~default" });
    vi.restoreAllMocks();
  });

  it("uses localized fallback copy for unknown save errors", async () => {
    const saveMutation = mockFactoryDocumentSave({
      mode: "error",
      rejectedError: "network unavailable",
    });
    vi.spyOn(currentFactoryFeature, "useSaveCurrentFactory").mockReturnValue(
      saveMutation as never,
    );
    const mutateAsync = saveMutation.mutateAsync;

    const { result } = renderHook(
      () =>
        useSaveEditableWorkerConfiguration({
          editableConfigurationState: buildReadyEditableConfigurationState(),
          locale: "zh-CN",
          scopeKey: "reviewer",
        }),
      { wrapper: createQueryClientWrapper() },
    );

    await act(async () => {
      await result.current.save();
    });

    await waitFor(() => {
      expect(result.current.saveState).toEqual({
        errorMessage: "无法保存运行中的工厂。",
        status: "error",
      });
    });
    expect(mutateAsync).toHaveBeenCalledWith({
      baseVersion: {
        logical: "7",
        physical: "2026-05-23T15:52:00Z",
      },
      factoryDefinition: {
        name: "Current Factory",
        workers: [
          {
            model: "gpt-5.5",
            modelProvider: "CODEX",
            name: "reviewer",
            type: "MODEL_WORKER",
          },
        ],
        workstations: [],
      },
    });
  });

  it("saves worker edits through the selected session current-factory route", async () => {
    useDashboardSessionStore.setState({ selectedSessionID: "session-beta" });
    const markChangesSaved = vi.fn();
    const saveMutation = mockFactoryDocumentSave({
      mode: "success",
      resolvedDocument: {
        name: "Current Factory",
        version: {
          logical: "8",
          physical: "2026-05-23T15:52:00.001Z",
        },
        workers: [
          {
            model: "gpt-5.5",
            modelProvider: "CODEX",
            name: "reviewer",
            type: "MODEL_WORKER",
          },
        ],
        workstations: [],
        workTypes: [],
      },
    });
    vi.spyOn(currentFactoryFeature, "useSaveCurrentFactory").mockReturnValue(
      saveMutation as never,
    );
    const mutateAsync = saveMutation.mutateAsync;

    const { result } = renderHook(
      () =>
        useSaveEditableWorkerConfiguration({
          editableConfigurationState: buildReadyEditableConfigurationState({
            isDirty: false,
            markChangesSaved,
          }),
          scopeKey: "reviewer",
        }),
      { wrapper: createQueryClientWrapper() },
    );

    await act(async () => {
      await result.current.save();
    });

    await waitFor(() => {
      expect(result.current.saveState).toEqual({ status: "success" });
    });
    expect(markChangesSaved).toHaveBeenCalledTimes(1);
    expect(mutateAsync).toHaveBeenCalledWith({
      baseVersion: {
        logical: "7",
        physical: "2026-05-23T15:52:00Z",
      },
      factoryDefinition: {
        name: "Current Factory",
        workers: [
          {
            model: "gpt-5.5",
            modelProvider: "CODEX",
            name: "reviewer",
            type: "MODEL_WORKER",
          },
        ],
        workstations: [],
      },
    });
  });

  it("ignores repeated save requests while the current save is still in flight", async () => {
    const pendingSave = mockPendingFactoryDocumentSave();
    vi.spyOn(currentFactoryFeature, "useSaveCurrentFactory").mockReturnValue(
      pendingSave.saveMutation as never,
    );
    const mutateAsync = pendingSave.mutateAsync;
    const deferredSave = pendingSave.deferred;

    const { result } = renderHook(
      () =>
        useSaveEditableWorkerConfiguration({
          editableConfigurationState: buildReadyEditableConfigurationState(),
          scopeKey: "reviewer",
        }),
      { wrapper: createQueryClientWrapper() },
    );

    let firstSave: Promise<void> | undefined;
    await act(async () => {
      firstSave = result.current.save();
      await Promise.resolve();
      await result.current.save();
    });

    expect(mutateAsync).toHaveBeenCalledTimes(1);
    expect(result.current.saveState).toEqual({ status: "submitting" });

    deferredSave.resolve({
      name: "Current Factory",
      version: {
        logical: "7",
        physical: "2026-05-23T15:52:00Z",
      },
      workers: [],
      workstations: [],
      workTypes: [],
    });

    await act(async () => {
      await firstSave;
    });

    await waitFor(() => {
      expect(result.current.saveState).not.toEqual({ status: "submitting" });
    });
  });

  it("keeps stale-version save failures recoverable as warnings", async () => {
    const saveMutation = mockFactoryDocumentSave({
      mode: "error",
      errorMode: "stale_version",
    });
    vi.spyOn(currentFactoryFeature, "useSaveCurrentFactory").mockReturnValue(
      saveMutation as never,
    );

    const { result } = renderHook(
      () =>
        useSaveEditableWorkerConfiguration({
          editableConfigurationState: buildReadyEditableConfigurationState(),
          scopeKey: "reviewer",
        }),
      { wrapper: createQueryClientWrapper() },
    );

    await act(async () => {
      await result.current.save();
    });

    await waitFor(() => {
      expect(result.current.saveState).toEqual({
        message:
          "Current factory definition is stale. Refresh the graph before saving.",
        status: "warning",
      });
    });
    expect(result.current.canSave).toBe(true);
  });

  it("clears save feedback when the selected worker scope changes", async () => {
    const mutateAsync = vi.fn().mockResolvedValue({
      name: "Current Factory",
      version: {
        logical: "8",
        physical: "2026-05-23T15:52:00.001Z",
      },
      workers: [
        {
          model: "gpt-5.5",
          modelProvider: "CODEX",
          name: "reviewer",
          type: "MODEL_WORKER",
        },
      ],
      workstations: [],
    });
    vi.spyOn(currentFactoryFeature, "useSaveCurrentFactory").mockReturnValue({
      isPending: false,
      mutateAsync,
    } as never);

    const { rerender, result } = renderHook(
      ({ scopeKey }) =>
        useSaveEditableWorkerConfiguration({
          editableConfigurationState: buildReadyEditableConfigurationState({
            isDirty: false,
          }),
          scopeKey,
        }),
      {
        initialProps: { scopeKey: "reviewer" },
        wrapper: createQueryClientWrapper(),
      },
    );

    await act(async () => {
      await result.current.save();
    });

    await waitFor(() => {
      expect(result.current.saveState).toEqual({ status: "success" });
    });

    rerender({ scopeKey: "planner" });

    expect(result.current.saveState).toEqual({ status: "idle" });
  });

  it("maps targeted save validation failures onto worker field errors", async () => {
    const mutateAsync = vi.fn().mockRejectedValue(
      new CurrentFactoryDefinitionError("Model provider is not supported.", {
        code: "BAD_REQUEST",
        status: 400,
        targets: [
          workerFieldValidationTarget(
            "modelProvider",
            "Model provider is not supported.",
          ),
        ],
      }),
    );
    vi.spyOn(currentFactoryFeature, "useSaveCurrentFactory").mockReturnValue({
      isPending: false,
      mutateAsync,
    } as never);

    const { result } = renderHook(
      () =>
        useSaveEditableWorkerConfiguration({
          editableConfigurationState: buildReadyEditableConfigurationState(),
          scopeKey: "reviewer",
        }),
      { wrapper: createQueryClientWrapper() },
    );

    await act(async () => {
      await result.current.save();
    });

    await waitFor(() => {
      expect(result.current.saveState).toEqual({
        errorMessage: "Model provider is not supported.",
        fieldErrors: {
          modelProvider: "Model provider is not supported.",
        },
        status: "error",
      });
    });
  });

  it.each([
    ["factory.workers[0].modelLocality", "modelLocality"],
    ["factory.workers[0].executorProvider", "executorProvider"],
    ["factory.workers[0].command", "command"],
    ["factory.workers[0].args", "args"],
    ["factory.workers[0].body", "body"],
    ["factory.workers[0].provider", "provider"],
    ["factory.workers[0].type", "type"],
    ["factory.workers[0].model", "model"],
  ] as const)(
    "maps save validation target %s onto %s",
    async (_field, expectedField) => {
      const message = `Invalid ${expectedField}.`;
      const mutateAsync = vi.fn().mockRejectedValue(
        new CurrentFactoryDefinitionError(message, {
          code: "BAD_REQUEST",
          status: 400,
          targets: [workerFieldValidationTarget(expectedField, message)],
        }),
      );
      vi.spyOn(currentFactoryFeature, "useSaveCurrentFactory").mockReturnValue({
        isPending: false,
        mutateAsync,
      } as never);

      const { result } = renderHook(
        () =>
          useSaveEditableWorkerConfiguration({
            editableConfigurationState: buildReadyEditableConfigurationState(),
            scopeKey: "reviewer",
          }),
        { wrapper: createQueryClientWrapper() },
      );

      await act(async () => {
        await result.current.save();
      });

      await waitFor(() => {
        expect(result.current.saveState).toEqual({
          errorMessage: message,
          fieldErrors: {
            [expectedField]: message,
          },
          status: "error",
        });
      });
    },
  );
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
  isDirty?: boolean;
  markChangesSaved?: () => void;
}): EditableWorkerConfigurationState {
  return {
    baseVersion: {
      logical: "7",
      physical: "2026-05-23T15:52:00Z",
    },
    canSave: true,
    draft: {
      argsText: "",
      body: "",
      command: "",
      executorProvider: null,
      model: "gpt-5.5",
      modelLocality: null,
      modelProvider: "CODEX",
      provider: null,
      type: "MODEL_WORKER",
    },
    hasValidationErrors: false,
    initialValues: {
      argsText: "",
      body: "",
      command: "",
      executorProvider: null,
      model: "gpt-5.5",
      modelLocality: null,
      modelProvider: "CURSOR",
      provider: null,
      type: "MODEL_WORKER",
      workerName: "reviewer",
      workstationNames: ["Review"],
    },
    isDirty: overrides?.isDirty ?? true,
    markChangesSaved: overrides?.markChangesSaved ?? vi.fn(),
    onArgsTextChange: vi.fn(),
    onBodyChange: vi.fn(),
    onCommandChange: vi.fn(),
    onExecutorProviderChange: vi.fn(),
    onModelChange: vi.fn(),
    onModelLocalityChange: vi.fn(),
    onModelProviderChange: vi.fn(),
    onProviderChange: vi.fn(),
    onResetToLatest: vi.fn(),
    onTypeChange: vi.fn(),
    overwriteFieldNames: [],
    pendingFactoryDefinition: {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.5",
          modelProvider: "CODEX",
          name: "reviewer",
          type: "MODEL_WORKER",
        },
      ],
      workstations: [],
    },
    status: "ready",
    validationErrors: {},
  };
}
