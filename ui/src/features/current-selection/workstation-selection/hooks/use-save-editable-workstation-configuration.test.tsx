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

  it("saves a promptless script definition without adding a workstation body", async () => {
    const scriptFactory = {
      name: "Script Factory",
      version: {
        logical: "7",
        physical: "2026-05-23T15:52:00Z",
      },
      workers: [
        {
          args: ["--mode", "script"],
          command: "./run-script.sh",
          name: "script-runner",
          type: "SCRIPT_WORKER" as const,
        },
      ],
      workstations: [
        {
          id: "run-script",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Run Script",
          outputs: [{ state: "done", workType: "story" }],
          type: "SCRIPT_RUN" as const,
          worker: "script-runner",
        },
      ],
    };
    const saveAsync = vi.fn().mockResolvedValue(scriptFactory);
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
          editableConfigurationState: buildReadyEditableConfigurationState({
            pendingFactoryDefinition: scriptFactory,
            prompt: "",
            workstationType: "SCRIPT_RUN",
            draft: {
              prompt: "",
              workerName: "script-runner",
              workstationType: "SCRIPT_RUN",
            },
          }),
          scopeKey: "run-script:transition:Run Script",
        }),
      { wrapper: createQueryClientWrapper() },
    );

    expect(result.current.canSave).toBe(true);

    await act(async () => {
      await result.current.save();
    });

    expect(saveAsync).toHaveBeenCalledWith({
      baseVersion: {
        logical: "7",
        physical: "2026-05-23T15:52:00Z",
      },
      factory: scriptFactory,
    });
    expect(scriptFactory.workstations[0]).not.toHaveProperty("body");
  });

  it("saves a poller definition with POLLER_RUN and no prompt or runner fields", async () => {
    const pollerFactory = {
      name: "Poller Factory",
      version: {
        logical: "7",
        physical: "2026-05-23T15:52:00Z",
      },
      workers: [
        {
          auth: { secretRef: "secrets/linear" },
          name: "linear-poller",
          provider: "LINEAR" as const,
          type: "HOSTED_WORKER" as const,
        },
      ],
      workstations: [
        {
          behavior: "POLLER" as const,
          id: "linear-ingress",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Linear Ingress",
          outputs: [{ state: "received", workType: "story" }],
          type: "POLLER_RUN" as const,
          worker: "linear-poller",
        },
      ],
    };
    const saveAsync = vi.fn().mockResolvedValue(pollerFactory);
    const markChangesSaved = vi.fn();
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
          editableConfigurationState: buildReadyEditableConfigurationState({
            draft: {
              behavior: "POLLER",
              prompt: "",
              runnerName: null,
              workerName: "linear-poller",
              workstationType: "POLLER_RUN",
            },
            markChangesSaved,
            pendingFactoryDefinition: pollerFactory,
            prompt: "",
            workstationType: "POLLER_RUN",
          }),
          scopeKey: "linear-ingress:transition:Linear Ingress",
        }),
      { wrapper: createQueryClientWrapper() },
    );

    expect(result.current.canSave).toBe(true);

    await act(async () => {
      await result.current.save();
    });

    expect(saveAsync).toHaveBeenCalledWith({
      baseVersion: {
        logical: "7",
        physical: "2026-05-23T15:52:00Z",
      },
      factory: pollerFactory,
    });
    expect(pollerFactory.workstations[0]).toMatchObject({
      behavior: "POLLER",
      type: "POLLER_RUN",
      worker: "linear-poller",
    });
    expect(pollerFactory.workstations[0]).not.toHaveProperty("body");
    expect(pollerFactory.workstations[0]).not.toHaveProperty("runner");
    expect(markChangesSaved).toHaveBeenCalledTimes(1);
  });

  it("saves legacy model workstations without changing their type or worker alias", async () => {
    const legacyModelFactory = {
      name: "Legacy Model Factory",
      version: {
        logical: "7",
        physical: "2026-05-23T15:52:00Z",
      },
      workers: [
        {
          args: ["--json"],
          command: "model-runner",
          model: "gpt-5.4",
          name: "legacy-model",
          type: "MODEL_WORKER" as const,
        },
      ],
      workstations: [
        {
          behavior: "STANDARD" as const,
          body: "Review the updated story.",
          guards: [{ maxVisits: 2, type: "VISIT_COUNT" as const }],
          id: "legacy-review",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Legacy Review Updated",
          outputs: [{ state: "approved", workType: "story" }],
          promptFile: "prompts/legacy-review.md",
          runner: "claude" as const,
          type: "MODEL_WORKSTATION" as const,
          worker: "legacy-model",
        },
      ],
    };
    const saveAsync = vi.fn().mockResolvedValue(legacyModelFactory);
    const markChangesSaved = vi.fn();
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
          editableConfigurationState: buildReadyEditableConfigurationState({
            markChangesSaved,
            pendingFactoryDefinition: legacyModelFactory,
            prompt: "Review the updated story.",
            workstationType: "MODEL_WORKSTATION",
            draft: {
              name: "Legacy Review Updated",
              prompt: "Review the updated story.",
              runnerName: "claude",
              workerName: "legacy-model",
              workstationType: "MODEL_WORKSTATION",
            },
          }),
          scopeKey: "legacy-review:transition:Legacy Review",
        }),
      { wrapper: createQueryClientWrapper() },
    );

    expect(result.current.canSave).toBe(true);

    await act(async () => {
      await result.current.save();
    });

    expect(saveAsync).toHaveBeenCalledWith({
      baseVersion: {
        logical: "7",
        physical: "2026-05-23T15:52:00Z",
      },
      factory: legacyModelFactory,
    });
    expect(markChangesSaved).toHaveBeenCalledTimes(1);
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
  workstationType?:
    | "MODEL_WORKSTATION"
    | "MODEL_INVOKE"
    | "LOGICAL_MOVE"
    | "POLLER_RUN"
    | "SCRIPT_RUN";
}): EditableWorkstationConfigurationState {
  const workstationType = overrides?.workstationType ?? "MODEL_WORKSTATION";
  const isPollerRun = workstationType === "POLLER_RUN";
  const behavior = isPollerRun ? "POLLER" : (overrides?.behavior ?? "STANDARD");
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
      prompt: overrides?.prompt ?? (isPollerRun ? "" : "Review the story."),
      runnerName: isPollerRun ? null : null,
      workerName:
        overrides?.draft?.workerName ??
        (isPollerRun ? "linear-poller" : "reviewer"),
      ...overrides?.draft,
    },
    hasValidationErrors: overrides?.hasValidationErrors ?? false,
    initialValues: buildReadyEditableConfigurationInitialValues({
      behavior,
      operation: overrides?.draft?.operation ?? "",
      operationBindings: overrides?.draft?.operationBindings ?? [],
      prompt: overrides?.prompt ?? (isPollerRun ? null : "Review the story."),
      workstationType,
    }),
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
      options: isPollerRun ? ["linear-poller", "script-poller"] : ["reviewer"],
      status: "ready",
    },
  };
}

type ReadyEditableConfigurationState = Extract<
  EditableWorkstationConfigurationState,
  { status: "ready" }
>;

function buildReadyEditableConfigurationInitialValues({
  behavior,
  operation,
  operationBindings,
  prompt,
  workstationType,
}: {
  behavior: ReadyEditableConfigurationState["draft"]["behavior"];
  operation: ReadyEditableConfigurationState["draft"]["operation"];
  operationBindings: ReadyEditableConfigurationState["draft"]["operationBindings"];
  prompt: ReadyEditableConfigurationState["initialValues"]["prompt"];
  workstationType: ReadyEditableConfigurationState["draft"]["workstationType"];
}): ReadyEditableConfigurationState["initialValues"] {
  const isPollerRun = workstationType === "POLLER_RUN";

  return {
    behavior,
    behaviorOptions: isPollerRun
      ? ["POLLER"]
      : behavior === "CRON"
        ? ["CRON"]
        : ["STANDARD", "REPEATER", "POLLER"],
    cron: null,
    effectiveRunnerName: "codex",
    factoryRunnerName: null,
    prompt,
    resolvedRunnerSelection: { runnerId: "codex", source: "default" },
    runnerName: null,
    runnerOptions: ["codex", "gemini", "kiro", "codex", "opencode", "pi"],
    runnerSelectionSource: "default",
    sharedWorkerWorkstationNamesByWorkerName: {},
    sharedWorkerWorkstationNames: [],
    workerModelProvider: null,
    workerName: isPollerRun ? "linear-poller" : "reviewer",
    workerOptions: isPollerRun
      ? ["linear-poller", "script-poller"]
      : ["reviewer"],
    workerTypeByName: {
      "linear-poller": "HOSTED_WORKER",
      reviewer: "MODEL_WORKER",
      "script-poller": "SCRIPT_WORKER",
    },
    workstationName: "Review",
    workstationOptions: ["Review"],
    workstationType,
    guards: [],
    inputs: [],
    modelInvokeWorkerOptions: [],
    modelOperationsByWorkerName: {},
    operation,
    operationBindings,
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
