// biome-ignore-all lint/style/noExcessiveLinesPerFile lint/complexity/noExcessiveLinesPerFunction: focused save-hook regressions share one mocked mutation seam.
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

import { CurrentFactoryDefinitionError } from "../../../../api/current-factory-definition";
import {
  mockFactoryDocumentSave,
  mockPendingFactoryDocumentSave,
} from "../../../../testing/factory-document-save-mocks";
import { workStateFieldValidationTarget } from "../../../../testing/factory-validation-target-fixtures";
import * as factoryDocumentSaveHooks from "../../../current-factory-definition/hooks/useFactoryDocumentSave";
import { useDashboardSessionStore } from "../../../dashboard/state/dashboardSessionStore";
import type { EditableWorkStateConfigurationState } from "../lib/detail-card-types";
import { useSaveEditableWorkStateConfiguration } from "./use-save-editable-work-state-configuration";

describe("useSaveEditableWorkStateConfiguration", () => {
  beforeEach(() => {
    useDashboardSessionStore.setState({ selectedSessionID: "~default" });
    vi.restoreAllMocks();
  });

  it("uses localized fallback copy for unknown save errors", async () => {
    const saveMutation = mockFactoryDocumentSave({
      mode: "error",
      rejectedError: "network unavailable",
    });
    vi.spyOn(
      factoryDocumentSaveHooks,
      "useFactoryDocumentSave",
    ).mockReturnValue(saveMutation as never);
    const saveAsync = saveMutation.saveAsync;

    const { result } = renderHook(
      () =>
        useSaveEditableWorkStateConfiguration({
          editableConfigurationState: buildReadyEditableConfigurationState(),
          locale: "zh-CN",
          scopeKey: "story:queued",
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
        workers: [
          {
            modelProvider: "CURSOR",
            name: "reviewer",
            type: "MODEL_WORKER",
          },
        ],
        workTypes: [
          {
            name: "story",
            states: [
              { name: "queued", type: "INITIAL" },
              { name: "done", type: "TERMINAL" },
            ],
          },
        ],
        workstations: [],
      },
    });
  });

  it("saves work state edits through the scoped factory document route", async () => {
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
            modelProvider: "CURSOR",
            name: "reviewer",
            type: "MODEL_WORKER",
          },
        ],
        workstations: [],
        workTypes: [
          {
            name: "story",
            states: [
              { name: "queued", type: "INITIAL" },
              { name: "done", type: "TERMINAL" },
            ],
          },
        ],
      },
    });
    vi.spyOn(
      factoryDocumentSaveHooks,
      "useFactoryDocumentSave",
    ).mockReturnValue(saveMutation as never);
    const saveAsync = saveMutation.saveAsync;

    const { result } = renderHook(
      () =>
        useSaveEditableWorkStateConfiguration({
          editableConfigurationState: buildReadyEditableConfigurationState({
            isDirty: false,
            markChangesSaved,
          }),
          scopeKey: "story:queued",
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
    expect(saveAsync).toHaveBeenCalledWith({
      baseVersion: {
        logical: "7",
        physical: "2026-05-23T15:52:00Z",
      },
      factory: {
        name: "Current Factory",
        workers: [
          {
            modelProvider: "CURSOR",
            name: "reviewer",
            type: "MODEL_WORKER",
          },
        ],
        workTypes: [
          {
            name: "story",
            states: [
              { name: "queued", type: "INITIAL" },
              { name: "done", type: "TERMINAL" },
            ],
          },
        ],
        workstations: [],
      },
    });
  });

  it("updates work state selection after a successful rename save", async () => {
    const markChangesSaved = vi.fn();
    const onWorkStateRenamed = vi.fn();
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
        name: "ready",
        type: "INITIAL",
      };
      renamedState.pendingFactoryDefinition = {
        name: "Current Factory",
        workers: [
          {
            modelProvider: "CURSOR",
            name: "reviewer",
            type: "MODEL_WORKER",
          },
        ],
        workTypes: [
          {
            name: "story",
            states: [
              { name: "ready", type: "INITIAL" },
              { name: "done", type: "TERMINAL" },
            ],
          },
        ],
        workstations: [],
      };
    }

    const { result } = renderHook(
      () =>
        useSaveEditableWorkStateConfiguration({
          editableConfigurationState: renamedState,
          onWorkStateRenamed,
          scopeKey: "story:queued",
        }),
      { wrapper: createQueryClientWrapper() },
    );

    await act(async () => {
      await result.current.save();
    });

    expect(markChangesSaved).toHaveBeenCalledTimes(1);
    expect(onWorkStateRenamed).toHaveBeenCalledWith("story:ready");
  });

  it("does not call save when client validation blocks canSave", async () => {
    const saveMutation = mockFactoryDocumentSave({ mode: "success" });
    vi.spyOn(
      factoryDocumentSaveHooks,
      "useFactoryDocumentSave",
    ).mockReturnValue(saveMutation as never);
    const saveAsync = saveMutation.saveAsync;

    const { result } = renderHook(
      () =>
        useSaveEditableWorkStateConfiguration({
          editableConfigurationState: buildReadyEditableConfigurationState({
            canSave: false,
            validationErrors: {
              name: 'A work state named "done" already exists for this work type.',
            },
          }),
          scopeKey: "story:queued",
        }),
      { wrapper: createQueryClientWrapper() },
    );

    await act(async () => {
      await result.current.save();
    });

    expect(saveAsync).not.toHaveBeenCalled();
    expect(result.current.canSave).toBe(false);
  });

  it("ignores repeated save requests while the current save is still in flight", async () => {
    const pendingSave = mockPendingFactoryDocumentSave();
    vi.spyOn(
      factoryDocumentSaveHooks,
      "useFactoryDocumentSave",
    ).mockReturnValue(pendingSave.saveMutation as never);
    const saveAsync = pendingSave.saveAsync;
    const deferredSave = pendingSave.deferred;

    const { result } = renderHook(
      () =>
        useSaveEditableWorkStateConfiguration({
          editableConfigurationState: buildReadyEditableConfigurationState(),
          scopeKey: "story:queued",
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
      workTypes: [],
    });

    await act(async () => {
      await firstSave;
    });

    await waitFor(() => {
      expect(result.current.saveState).not.toEqual({ status: "submitting" });
    });
  });

  it("maps targeted save validation failures onto the work state name field", async () => {
    const message = "Work state name is invalid.";
    const saveAsync = vi.fn().mockRejectedValue(
      new CurrentFactoryDefinitionError(message, {
        code: "BAD_REQUEST",
        status: 400,
        targets: [workStateFieldValidationTarget("name", message)],
      }),
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
        useSaveEditableWorkStateConfiguration({
          editableConfigurationState: buildReadyEditableConfigurationState(),
          scopeKey: "story:queued",
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
          name: message,
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
  canSave?: boolean;
  isDirty?: boolean;
  markChangesSaved?: () => void;
  validationErrors?: EditableWorkStateConfigurationState extends {
    status: "ready";
  }
    ? EditableWorkStateConfigurationState["validationErrors"]
    : never;
}): EditableWorkStateConfigurationState {
  return {
    baseVersion: {
      logical: "7",
      physical: "2026-05-23T15:52:00Z",
    },
    canSave: overrides?.canSave ?? true,
    draft: {
      name: "queued",
      type: "INITIAL",
    },
    hasValidationErrors: overrides?.canSave === false,
    initialValues: {
      stateName: "queued",
      stateNamesInWorkType: ["queued", "done"],
      stateType: "INITIAL",
      workTypeName: "story",
    },
    isDirty: overrides?.isDirty ?? true,
    markChangesSaved: overrides?.markChangesSaved ?? vi.fn(),
    onNameChange: vi.fn(),
    onResetToLatest: vi.fn(),
    originalStateName: "queued",
    pendingFactoryDefinition: {
      name: "Current Factory",
      workers: [
        {
          modelProvider: "CURSOR",
          name: "reviewer",
          type: "MODEL_WORKER",
        },
      ],
      workTypes: [
        {
          name: "story",
          states: [
            { name: "queued", type: "INITIAL" },
            { name: "done", type: "TERMINAL" },
          ],
        },
      ],
      workstations: [],
    },
    savedFactoryDefinition: {
      name: "Current Factory",
      workers: [
        {
          modelProvider: "CURSOR",
          name: "reviewer",
          type: "MODEL_WORKER",
        },
      ],
      workTypes: [
        {
          name: "story",
          states: [
            { name: "queued", type: "INITIAL" },
            { name: "done", type: "TERMINAL" },
          ],
        },
      ],
      workstations: [],
    },
    status: "ready",
    validationErrors: overrides?.validationErrors ?? {},
    workTypeName: "story",
  };
}
