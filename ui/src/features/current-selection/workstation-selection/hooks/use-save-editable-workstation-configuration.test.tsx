// biome-ignore lint/nursery/noExcessiveLinesPerFile: save-hook coverage includes invalid-to-valid prompt validation regressions alongside confirmation wiring.
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

import { CurrentFactoryDefinitionError } from "../../../../api/current-factory-definition";
import { staleFactoryVersionTarget } from "../../../../testing/factory-validation-target-fixtures";
import * as factoryDocumentSaveHooks from "../../../current-factory-definition/hooks/useFactoryDocumentSave";
import { DashboardSessionProvider } from "../../../dashboard/session/dashboard-session-provider";
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
    const saveAsync = vi.fn().mockRejectedValue("network unavailable");
    vi.spyOn(
      factoryDocumentSaveHooks,
      "useFactoryDocumentSave",
    ).mockReturnValue({
      isPending: false,
      saveAsync,
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
    expect(saveAsync).toHaveBeenCalledWith({
      baseVersion: {
        logical: "7",
        physical: "2026-05-23T15:52:00Z",
      },
      factory: {
        name: "Current Factory",
        workers: [],
        workstations: [],
      },
    });
  });

  it("does not open save confirmation when canSave is false after prompt validation errors", () => {
    const { result } = renderHook(
      () =>
        useSaveEditableWorkstationConfiguration({
          editableConfigurationState: buildReadyEditableConfigurationState({
            hasValidationErrors: true,
            pendingFactoryDefinition: null,
            promptValidationState: {
              diagnostics: [
                {
                  kind: "SYNTAX_ERROR",
                  message: "line 1: unexpected EOF",
                },
              ],
              result: {
                diagnostics: [
                  {
                    kind: "SYNTAX_ERROR",
                    message: "line 1: unexpected EOF",
                  },
                ],
                valid: false,
              },
              status: "ready",
            },
            validationErrors: {
              prompt: "See prompt diagnostics below.",
            },
          }),
          scopeKey: "review:transition:Review",
        }),
      { wrapper: createQueryClientWrapper() },
    );

    expect(result.current.canSave).toBe(false);

    act(() => {
      result.current.beginSaveConfirmation();
    });

    expect(result.current.saveState).toEqual({ status: "idle" });
  });

  it("opens save confirmation after prompt validation recovers for a dirty draft", () => {
    const { result } = renderHook(
      () =>
        useSaveEditableWorkstationConfiguration({
          editableConfigurationState: buildReadyEditableConfigurationState({
            prompt: "Use {{ .WorkID }}.",
          }),
          scopeKey: "review:transition:Review",
        }),
      { wrapper: createQueryClientWrapper() },
    );

    expect(result.current.canSave).toBe(true);

    act(() => {
      result.current.beginSaveConfirmation();
    });

    expect(result.current.saveState).toEqual({ status: "confirming" });
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
            mode: "REPLACE_CURRENT",
            factory: {
              name: "Current Factory",
              workers: [],
              workstations: [],
              version: {
                logical: "8",
                physical: "2026-05-23T15:52:00.001Z",
              },
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
    const saveAsync = vi.fn().mockReturnValue(deferredSave.promise);
    vi.spyOn(
      factoryDocumentSaveHooks,
      "useFactoryDocumentSave",
    ).mockReturnValue({
      isPending: false,
      saveAsync,
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

    expect(saveAsync).toHaveBeenCalledTimes(1);
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
    const saveAsync = vi.fn().mockRejectedValue(
      new CurrentFactoryDefinitionError(
        "Current factory definition is stale. Refresh the dashboard before saving or importing again.",
        {
          code: "STALE_FACTORY_VERSION",
          status: 409,
          targets: [staleFactoryVersionTarget()],
        },
      ),
    );
    vi.spyOn(
      factoryDocumentSaveHooks,
      "useFactoryDocumentSave",
    ).mockReturnValue({
      isPending: false,
      saveAsync,
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
        message:
          "Current factory definition is stale. Refresh the dashboard before saving or importing again.",
        status: "warning",
      });
    });
    expect(result.current.canSave).toBe(true);
  });

  it("does not open save confirmation when cron validation errors block save", () => {
    const { result } = renderHook(
      () =>
        useSaveEditableWorkstationConfiguration({
          editableConfigurationState: buildReadyEditableConfigurationState({
            behavior: "CRON",
            cron: {
              expiryWindow: "bad",
              jitter: "bad",
              schedule: "",
            },
            hasValidationErrors: true,
            pendingFactoryDefinition: null,
            validationErrors: {
              cronSchedule: "cron workstation requires non-empty 'schedule'",
            },
          }),
          scopeKey: "review:transition:Review",
        }),
      { wrapper: createQueryClientWrapper() },
    );

    expect(result.current.canSave).toBe(false);

    act(() => {
      result.current.beginSaveConfirmation();
    });

    expect(result.current.saveState).toEqual({ status: "idle" });
  });

  it("saves modified cron.schedule through the scoped running-factory save payload", async () => {
    const saveAsync = vi.fn().mockResolvedValue({
      name: "Current Factory",
      version: {
        logical: "8",
        physical: "2026-05-23T15:52:00.001Z",
      },
      workers: [
        {
          model: "gpt-5.5",
          name: "reviewer",
          type: "MODEL_WORKER",
        },
      ],
      workstations: [
        {
          behavior: "CRON",
          body: "",
          cron: {
            schedule: "0 9 * * *",
            triggerAtStart: true,
          },
          id: "daily-refresh",
          name: "Daily Refresh",
          worker: "reviewer",
        },
      ],
    });
    vi.spyOn(
      factoryDocumentSaveHooks,
      "useFactoryDocumentSave",
    ).mockReturnValue({
      isPending: false,
      saveAsync,
    } as never);
    const markChangesSaved = vi.fn();

    const { result } = renderHook(
      () =>
        useSaveEditableWorkstationConfiguration({
          editableConfigurationState: buildReadyEditableConfigurationState({
            behavior: "CRON",
            cron: {
              schedule: "0 9 * * *",
              triggerAtStart: true,
            },
            markChangesSaved,
            pendingFactoryDefinition: {
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
                  behavior: "CRON",
                  body: "",
                  cron: {
                    schedule: "0 9 * * *",
                    triggerAtStart: true,
                  },
                  id: "daily-refresh",
                  name: "Daily Refresh",
                  worker: "reviewer",
                },
              ],
            },
            prompt: "",
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

    expect(saveAsync).toHaveBeenCalledWith({
      baseVersion: {
        logical: "7",
        physical: "2026-05-23T15:52:00Z",
      },
      factory: {
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
            behavior: "CRON",
            body: "",
            cron: {
              schedule: "0 9 * * *",
              triggerAtStart: true,
            },
            id: "daily-refresh",
            name: "Daily Refresh",
            worker: "reviewer",
          },
        ],
      },
    });
    expect(markChangesSaved).toHaveBeenCalledTimes(1);
  });

  it("maps targeted save validation failures onto workstation field errors", async () => {
    const saveAsync = vi.fn().mockRejectedValue(
      new CurrentFactoryDefinitionError(
        "Worker selection must reference a configured worker.",
        {
          code: "BAD_REQUEST",
          status: 400,
          targets: [
            {
              code: "factory.worker.danglingReference",
              message: "Worker selection must reference a configured worker.",
              severity: "error",
              subject: {
                id: "worker",
                location: "DEFINITION",
                type: "WORKSTATION",
              },
            },
          ],
        },
      ),
    );
    vi.spyOn(
      factoryDocumentSaveHooks,
      "useFactoryDocumentSave",
    ).mockReturnValue({
      isPending: false,
      saveAsync,
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
      <QueryClientProvider client={queryClient}>
        <DashboardSessionProvider>{children}</DashboardSessionProvider>
      </QueryClientProvider>
    );
  };
}

function buildReadyEditableConfigurationState(overrides?: {
  behavior?: "STANDARD" | "REPEATER" | "POLLER" | "CRON";
  cron?: {
    expiryWindow?: string;
    jitter?: string;
    schedule: string;
    triggerAtStart?: boolean;
  };
  hasValidationErrors?: boolean;
  markChangesSaved?: () => void;
  pendingFactoryDefinition?: EditableWorkstationConfigurationState extends {
    status: "ready";
  }
    ? NonNullable<
        Extract<
          EditableWorkstationConfigurationState,
          { status: "ready" }
        >["pendingFactoryDefinition"]
      >
    : never;
  prompt?: string;
  promptValidationState?: Extract<
    EditableWorkstationConfigurationState,
    { status: "ready" }
  >["promptValidationState"];
  validationErrors?: Extract<
    EditableWorkstationConfigurationState,
    { status: "ready" }
  >["validationErrors"];
}): EditableWorkstationConfigurationState {
  const behavior = overrides?.behavior ?? "STANDARD";
  const cron =
    behavior === "CRON"
      ? {
          expiryWindow: overrides?.cron?.expiryWindow ?? "",
          jitter: overrides?.cron?.jitter ?? "",
          schedule: overrides?.cron?.schedule ?? "0 0 * * *",
          triggerAtStart: overrides?.cron?.triggerAtStart ?? false,
        }
      : null;

  return {
    draft: {
      behavior,
      cron,
      guards: [],
      inputs: [],
      prompt: overrides?.prompt ?? "Review the story.",
      runnerName: null,
      workerName: "reviewer",
    },
    hasValidationErrors: overrides?.hasValidationErrors ?? false,
    initialValues: {
      behavior,
      behaviorOptions:
        behavior === "CRON"
          ? ["CRON"]
          : ["STANDARD", "REPEATER", "POLLER"],
      cron,
      effectiveRunnerName: "codex",
      factoryRunnerName: null,
      prompt: overrides?.prompt ?? "Review the story.",
      resolvedRunnerSelection: {
        runnerId: "codex",
        source: "default",
      },
      runnerName: null,
      runnerOptions: ["codex", "gemini", "kiro", "cursor-cli", "opencode"],
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
      workstationOptions: ["Review"],
      workstationType: "MODEL_WORKSTATION",
      guards: [],
      inputs: [],
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
    onGuardsChange: vi.fn(),
    onInputsChange: vi.fn(),
    onRunnerChange: vi.fn(),
    onWorkerChange: vi.fn(),
    workstationOptionsState: {
      options: ["Review"],
      status: "ready",
    },
    overwriteFieldNames: [],
    pendingFactoryDefinition:
      overrides?.pendingFactoryDefinition ??
      ({
        name: "Current Factory",
        workers: [],
        workstations: [],
      } as const),
    promptDiagnostics: [],
    promptHelpState: {
      contract: {
        availableVariables: [],
        inputCount: 0,
        unavailableAccessPatterns: [],
      },
      status: "ready",
    },
    promptValidationState: overrides?.promptValidationState ?? {
      diagnostics: [],
      result: {
        diagnostics: [],
        valid: true,
      },
      status: "ready",
    },
    status: "ready",
    validationErrors: overrides?.validationErrors ?? {},
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
