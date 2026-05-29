import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

import { CurrentFactoryDefinitionError } from "../../../../api/current-factory-definition";
import * as currentFactoryFeature from "../../../current-factory-definition/public";
import { useDashboardSessionStore } from "../../../dashboard/state/dashboardSessionStore";
import type { EditableWorkstationConfigurationState } from "../lib/detail-card-types";
import { useSaveEditableWorkstationConfiguration } from "./use-save-editable-workstation-configuration";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: focused save-hook regressions share one mocked mutation seam to keep re-entrant action behavior readable.
describe("useSaveEditableWorkstationConfiguration", () => {
  beforeEach(() => {
    useDashboardSessionStore.setState({ selectedSessionID: "~default" });
    vi.restoreAllMocks();
  });

  it("uses localized fallback copy for unknown save errors", async () => {
    const mutateAsync = vi.fn().mockRejectedValue("network unavailable");
    vi.spyOn(currentFactoryFeature, "useSaveCurrentFactory").mockReturnValue({
      isPending: false,
      mutateAsync,
    } as never);

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
    expect(mutateAsync).toHaveBeenCalledWith({
      baseVersion: {
        logical: "7",
        physical: "2026-05-23T15:52:00Z",
      },
      factoryDefinition: {
        name: "Current Factory",
        workers: [],
        workstations: [],
      },
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

  it("saves workstation edits through the selected session current-factory route", async () => {
    useDashboardSessionStore.setState({ selectedSessionID: "session-beta" });
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(
        JSON.stringify({
          name: "Current Factory",
          version: {
            logical: "7",
            physical: "2026-05-23T15:52:00Z",
          },
          workers: [],
          workstations: [],
        }),
        {
          headers: {
            "content-type": "application/json",
          },
          status: 200,
        },
      ),
    );
    const markChangesSaved = vi.fn();

    const { result } = renderHook(
      () =>
        useSaveEditableWorkstationConfiguration({
          editableConfigurationState: buildReadyEditableConfigurationState({
            markChangesSaved,
            prompt: "Save into the selected session.",
          }),
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
      expect(fetchMock).toHaveBeenCalledWith(
        "/factory-sessions/session-beta/factory",
        expect.objectContaining({
          body: JSON.stringify({
            name: "Current Factory",
            workers: [],
            workstations: [],
            version: {
              logical: "8",
              physical: "2026-05-23T15:52:00.001Z",
            },
          }),
          headers: {
            "content-type": "application/json",
          },
          method: "PUT",
        }),
      );
    });
    expect(markChangesSaved).toHaveBeenCalledTimes(1);
  });

  it("ignores repeated save confirmations while the current save is still in flight", async () => {
    const deferredSave = createDeferredPromise<unknown>();
    const mutateAsync = vi.fn().mockReturnValue(deferredSave.promise);
    vi.spyOn(currentFactoryFeature, "useSaveCurrentFactory").mockReturnValue({
      isPending: false,
      mutateAsync,
    } as never);

    const { result } = renderHook(
      () =>
        useSaveEditableWorkstationConfiguration({
          editableConfigurationState: buildReadyEditableConfigurationState(),
          scopeKey: "review:transition:Review",
        }),
      { wrapper: createQueryClientWrapper() },
    );

    act(() => {
      result.current.beginSaveConfirmation();
    });

    let firstSave: Promise<void> | undefined;
    await act(async () => {
      firstSave = result.current.confirmSave();
      await Promise.resolve();
      await result.current.confirmSave();
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
    });

    await act(async () => {
      await firstSave;
    });

    await waitFor(() => {
      expect(result.current.saveState).not.toEqual({ status: "submitting" });
    });
  });

  it("keeps stale-version save failures recoverable as warnings", async () => {
    const mutateAsync = vi.fn().mockRejectedValue(
      new CurrentFactoryDefinitionError("Current factory definition is stale. Refresh the graph before saving.", {
        code: "STALE_FACTORY_VERSION",
        status: 409,
        targets: [{ id: "stale-version", kind: "save" }],
      }),
    );
    vi.spyOn(currentFactoryFeature, "useSaveCurrentFactory").mockReturnValue({
      isPending: false,
      mutateAsync,
    } as never);

    const { result } = renderHook(
      () =>
        useSaveEditableWorkstationConfiguration({
          editableConfigurationState: buildReadyEditableConfigurationState(),
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
        message: "Current factory definition is stale. Refresh the graph before saving.",
        status: "warning",
      });
    });
    expect(result.current.canSave).toBe(true);
  });

  it("maps targeted save validation failures onto workstation field errors", async () => {
    const mutateAsync = vi.fn().mockRejectedValue(
      new CurrentFactoryDefinitionError("Worker selection must reference a configured worker.", {
        code: "BAD_REQUEST",
        status: 400,
        targets: [
          {
            field: "factory.workstations[0].worker",
            kind: "field",
          },
        ],
      }),
    );
    vi.spyOn(currentFactoryFeature, "useSaveCurrentFactory").mockReturnValue({
      isPending: false,
      mutateAsync,
    } as never);

    const { result } = renderHook(
      () =>
        useSaveEditableWorkstationConfiguration({
          editableConfigurationState: buildReadyEditableConfigurationState(),
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
        errorMessage: "Worker selection must reference a configured worker.",
        fieldErrors: {
          workerName: "Worker selection must reference a configured worker.",
        },
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

function buildReadyEditableConfigurationState(overrides?: {
  behavior?: "STANDARD" | "REPEATER" | "POLLER";
  markChangesSaved?: () => void;
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
      sharedWorkerWorkstationNamesByWorkerName: {},
      sharedWorkerWorkstationNames: [],
      workerName: "reviewer",
      workerOptions: ["reviewer"],
      workerTypeByName: {
        reviewer: "MODEL_WORKER",
      },
      workstationName: "Review",
    },
    isDirty: true,
    markChangesSaved: overrides?.markChangesSaved ?? vi.fn(),
    baseVersion: {
      logical: "7",
      physical: "2026-05-23T15:52:00Z",
    },
    onBehaviorChange: vi.fn(),
    onPromptChange: vi.fn(),
    onResetToLatest: vi.fn(),
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

function createDeferredPromise<T>() {
  let reject!: (reason?: unknown) => void;
  let resolve!: (value: T | PromiseLike<T>) => void;
  const promise = new Promise<T>((innerResolve, innerReject) => {
    reject = innerReject;
    resolve = innerResolve;
  });

  return { promise, reject, resolve };
}
