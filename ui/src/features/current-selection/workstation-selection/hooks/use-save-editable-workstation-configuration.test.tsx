// biome-ignore lint/style/noExcessiveLinesPerFile: save-hook coverage includes invalid-to-valid prompt validation regressions alongside scoped save wiring.
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

import { CurrentFactoryDefinitionError } from "../../../../api/current-factory-definition";
import { DashboardSessionStoreTestProvider } from "../../../../testing/dashboard-session-test-provider";
import { mockFactoryDocumentSave } from "../../../../testing/factory-document-save-mocks";
import { staleFactoryVersionTarget } from "../../../../testing/factory-validation-target-fixtures";
import * as factoryDocumentSaveHooks from "../../../current-factory-definition/hooks/useFactoryDocumentSave";
import { useDashboardSessionStore } from "../../../dashboard/state/dashboardSessionStore";
import type { EditableWorkstationConfigurationState } from "../lib/keys/detail-card-types";
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

    await act(async () => {
      await result.current.save();
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

  it("does not save when canSave is false after prompt validation errors", async () => {
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

    await act(async () => {
      await result.current.save();
    });

    expect(result.current.saveState).toEqual({ status: "idle" });
  });

  it("allows save after prompt validation recovers for a dirty draft", () => {
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

    expect(result.current.saveState).toEqual({ status: "idle" });
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

    await act(async () => {
      await result.current.save();
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

  it("updates workstation selection after a successful rename save", async () => {
    const markChangesSaved = vi.fn();
    const onWorkstationRenamed = vi.fn();
    const saveMutation = mockFactoryDocumentSave({ mode: "success" });
    vi.spyOn(
      factoryDocumentSaveHooks,
      "useFactoryDocumentSave",
    ).mockReturnValue(saveMutation as never);

    const renamedState = buildReadyEditableConfigurationState({
      markChangesSaved,
    });
    if (renamedState.status === "ready") {
      renamedState.draft = {
        ...renamedState.draft,
        name: "Senior Review",
      };
      renamedState.pendingFactoryDefinition = {
        name: "Current Factory",
        workers: [],
        workstations: [
          {
            id: "review",
            name: "Senior Review",
            worker: "reviewer",
          },
        ],
      };
    }

    const { result } = renderHook(
      () =>
        useSaveEditableWorkstationConfiguration({
          editableConfigurationState: renamedState,
          onWorkstationRenamed,
          scopeKey: "review:transition:Review",
        }),
      { wrapper: createQueryClientWrapper() },
    );

    await act(async () => {
      await result.current.save();
    });

    expect(markChangesSaved).toHaveBeenCalledTimes(1);
    expect(onWorkstationRenamed).toHaveBeenCalledWith("review");
  });

  it("ignores repeated saves while the current save is still in flight", async () => {
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

    let firstSave: Promise<void> | undefined;
    await act(async () => {
      firstSave = result.current.save();
      await Promise.resolve();
      await result.current.save();
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

    await act(async () => {
      await result.current.save();
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

  it("does not save when cron validation errors block save", async () => {
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

    await act(async () => {
      await result.current.save();
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

    await act(async () => {
      await result.current.save();
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

  it("saves model invoke workstation bindings through the scoped running-factory save payload", async () => {
    const {
      editableConfigurationState,
      expectedSavePayload,
      markChangesSaved,
      saveAsync,
    } = buildModelInvokeSaveScenario();
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
          editableConfigurationState,
          scopeKey: "review:transition:speak-story",
        }),
      { wrapper: createQueryClientWrapper() },
    );

    await act(async () => {
      await result.current.save();
    });

    expect(saveAsync).toHaveBeenCalledWith(expectedSavePayload);
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

    await act(async () => {
      await result.current.save();
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
        <DashboardSessionStoreTestProvider>
          {children}
        </DashboardSessionStoreTestProvider>
      </QueryClientProvider>
    );
  };
}

function buildModelInvokeSaveScenario() {
  const modelInvokeFactory = {
    name: "tts-factory",
    workers: [
      {
        name: "tts-worker",
        type: "MODEL_WORKER",
        operations: [
          {
            name: "TTS",
            inputs: [{ name: "text", contentTypes: ["TEXT"], required: true }],
            outputs: [{ name: "audio", contentTypes: ["AUDIO"] }],
          },
        ],
      },
    ],
    workstations: [
      {
        name: "speak-story",
        type: "MODEL_INVOKE",
        worker: "tts-worker",
        operation: "TTS",
        operationBindings: [
          {
            slot: "text",
            selector: { label: "utterance", type: "TEXT" },
            config: [{ text: "static narration", type: "text" }],
          },
        ],
        inputs: [{ state: "init", workType: "story" }],
        outputs: [{ state: "complete", workType: "story" }],
      },
    ],
  };
  const markChangesSaved = vi.fn();
  const saveAsync = vi.fn().mockResolvedValue({
    ...modelInvokeFactory,
    version: {
      logical: "8",
      physical: "2026-05-23T15:52:00.001Z",
    },
  });

  return {
    editableConfigurationState: buildReadyEditableConfigurationState({
      markChangesSaved,
      pendingFactoryDefinition: modelInvokeFactory,
      prompt: "",
      workstationType: "MODEL_INVOKE",
      draft: {
        operation: "TTS",
        operationBindings: [
          {
            slot: "text",
            configText: "static narration",
            defaultContentText: "",
            selector: {
              label: "utterance",
              role: "",
              slot: "",
              type: "TEXT",
            },
          },
        ],
        workerName: "tts-worker",
      },
    }),
    expectedSavePayload: {
      baseVersion: {
        logical: "7",
        physical: "2026-05-23T15:52:00Z",
      },
      factory: modelInvokeFactory,
    },
    markChangesSaved,
    saveAsync,
  };
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: the fixture builder keeps the complete ready-state defaults visible for save-hook cases.
function buildReadyEditableConfigurationState(overrides?: {
  behavior?: "STANDARD" | "REPEATER" | "POLLER" | "CRON";
  cron?: {
    expiryWindow?: string;
    jitter?: string;
    schedule: string;
    triggerAtStart?: boolean;
  };
  draft?: Partial<
    Extract<EditableWorkstationConfigurationState, { status: "ready" }>["draft"]
  >;
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
  workstationType?: "MODEL_WORKSTATION" | "MODEL_INVOKE" | "LOGICAL_MOVE";
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
      name: "Review",
      operation: overrides?.draft?.operation ?? "",
      operationBindings: overrides?.draft?.operationBindings ?? [],
      prompt: overrides?.prompt ?? "Review the story.",
      runnerName: null,
      workerName: overrides?.draft?.workerName ?? "reviewer",
      ...overrides?.draft,
    },
    hasValidationErrors: overrides?.hasValidationErrors ?? false,
    initialValues: {
      behavior,
      behaviorOptions:
        behavior === "CRON" ? ["CRON"] : ["STANDARD", "REPEATER", "POLLER"],
      cron,
      effectiveRunnerName: "codex",
      factoryRunnerName: null,
      prompt: overrides?.prompt ?? "Review the story.",
      resolvedRunnerSelection: {
        runnerId: "codex",
        source: "default",
      },
      runnerName: null,
      runnerOptions: [
        "codex",
        "gemini",
        "kiro",
        "codex",
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
      workstationOptions: ["Review"],
      workstationType: overrides?.workstationType ?? "MODEL_WORKSTATION",
      guards: [],
      inputs: [],
      modelInvokeWorkerOptions: [],
      modelOperationsByWorkerName: {},
      operation: overrides?.draft?.operation ?? "",
      operationBindings: overrides?.draft?.operationBindings ?? [],
    },
    isDirty: true,
    markChangesSaved: overrides?.markChangesSaved ?? vi.fn(),
    baseVersion: {
      logical: "7",
      physical: "2026-05-23T15:52:00Z",
    },
    onBehaviorChange: vi.fn(),
    onNameChange: vi.fn(),
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
    savedFactoryDefinition:
      overrides?.savedFactoryDefinition ??
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
