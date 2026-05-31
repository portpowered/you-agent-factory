import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

import { CurrentFactoryDefinitionError } from "../../../../api/current-factory-definition";
import {
  mockFactoryDocumentSave,
  mockPendingFactoryDocumentSave,
} from "../../../../testing/factory-document-save-mocks";
import * as factoryDocumentSaveHooks from "../../../current-factory-definition/hooks/useFactoryDocumentSave";
import { DashboardSessionProvider } from "../../../dashboard/session/dashboard-session-provider";
import { useDashboardSessionStore } from "../../../dashboard/state/dashboardSessionStore";
import type { EditableWorkTypeConfigurationState } from "../lib/detail-card-types";
import { useSaveEditableWorkTypeConfiguration } from "./use-save-editable-work-type-configuration";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: focused save-hook regressions share one mocked mutation seam.
describe("useSaveEditableWorkTypeConfiguration", () => {
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

    const { result } = renderHook(
      () =>
        useSaveEditableWorkTypeConfiguration({
          editableConfigurationState: buildReadyEditableConfigurationState(),
          locale: "zh-CN",
          scopeKey: "story",
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

  it("saves work type edits through the selected session current-factory route", async () => {
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
        workers: [],
        workstations: [],
        workTypes: [
          {
            name: "feature",
            states: [{ name: "queued", type: "INITIAL" }],
          },
        ],
      },
    });
    vi.spyOn(
      factoryDocumentSaveHooks,
      "useFactoryDocumentSave",
    ).mockReturnValue(saveMutation as never);

    const { result } = renderHook(
      () =>
        useSaveEditableWorkTypeConfiguration({
          editableConfigurationState: buildReadyEditableConfigurationState({
            markChangesSaved,
          }),
          scopeKey: "story",
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
      expect(result.current.saveState).toEqual({ status: "success" });
    });
    expect(markChangesSaved).toHaveBeenCalledTimes(1);
    expect(saveMutation.saveAsync).toHaveBeenCalledWith({
      baseVersion: {
        logical: "7",
        physical: "2026-05-23T15:52:00Z",
      },
      factory: {
        name: "Current Factory",
        workTypes: [
          {
            handlingBehavior: ["DEFAULT"],
            name: "feature",
            states: [{ name: "queued", type: "INITIAL" }],
          },
        ],
        workstations: [],
        workers: [],
      },
    });
  });

  it("updates work type selection after a successful rename save", async () => {
    const markChangesSaved = vi.fn();
    const onWorkTypeRenamed = vi.fn();
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
        name: "feature",
      };
      renamedState.pendingFactoryDefinition = {
        name: "Current Factory",
        workTypes: [
          {
            name: "feature",
            states: [{ name: "queued", type: "INITIAL" }],
          },
        ],
        workstations: [
          {
            id: "review",
            inputs: [{ state: "queued", workType: "feature" }],
            name: "Review",
            worker: "reviewer",
          },
        ],
        workers: [],
      };
    }

    const { result } = renderHook(
      () =>
        useSaveEditableWorkTypeConfiguration({
          editableConfigurationState: renamedState,
          onWorkTypeRenamed,
          scopeKey: "story",
        }),
      { wrapper: createQueryClientWrapper() },
    );

    act(() => {
      result.current.beginSaveConfirmation();
    });

    await act(async () => {
      await result.current.confirmSave();
    });

    expect(markChangesSaved).toHaveBeenCalledTimes(1);
    expect(onWorkTypeRenamed).toHaveBeenCalledWith("feature");
  });

  it("blocks save when validation errors are present", () => {
    const { result } = renderHook(
      () =>
        useSaveEditableWorkTypeConfiguration({
          editableConfigurationState: buildReadyEditableConfigurationState({
            canSave: false,
            hasValidationErrors: true,
            validationErrors: {
              name: "Enter a work type name before saving this work type.",
            },
          }),
          scopeKey: "story",
        }),
      { wrapper: createQueryClientWrapper() },
    );

    expect(result.current.canSave).toBe(false);
  });

  it("maps work type save errors onto editable fields", async () => {
    const message = "handlingBehavior must include DEFAULT exactly once.";
    const saveAsync = vi.fn().mockRejectedValue(
      new CurrentFactoryDefinitionError(message, {
        code: "BAD_REQUEST",
        status: 400,
        targets: [
          {
            code: "factory.workType.handlingBehavior",
            message,
            severity: "error",
            subject: {
              id: "handlingBehavior",
              location: "DEFINITION",
              type: "WORK_TYPE",
            },
          },
        ],
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
        useSaveEditableWorkTypeConfiguration({
          editableConfigurationState: buildReadyEditableConfigurationState(),
          scopeKey: "story",
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
        fieldErrors: {
          handlingBehavior:
            "handlingBehavior must include DEFAULT exactly once.",
        },
        errorMessage: "handlingBehavior must include DEFAULT exactly once.",
        status: "error",
      });
    });
  });

  it("ignores repeated save requests while the current save is still in flight", async () => {
    const pendingSave = mockPendingFactoryDocumentSave();
    vi.spyOn(
      factoryDocumentSaveHooks,
      "useFactoryDocumentSave",
    ).mockReturnValue(pendingSave.saveMutation as never);

    const { result } = renderHook(
      () =>
        useSaveEditableWorkTypeConfiguration({
          editableConfigurationState: buildReadyEditableConfigurationState(),
          scopeKey: "story",
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

    expect(pendingSave.saveAsync).toHaveBeenCalledTimes(1);
    expect(result.current.saveState).toEqual({ status: "submitting" });

    pendingSave.deferred.resolve({
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
});

function createQueryClientWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
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
  canSave?: boolean;
  hasValidationErrors?: boolean;
  markChangesSaved?: () => void;
  validationErrors?: EditableWorkTypeConfigurationState extends {
    status: "ready";
  }
    ? Extract<EditableWorkTypeConfigurationState, { status: "ready" }>["validationErrors"]
    : never;
}): EditableWorkTypeConfigurationState {
  return {
    baseVersion: {
      logical: "7",
      physical: "2026-05-23T15:52:00Z",
    },
    canSave: overrides?.canSave ?? true,
    draft: {
      handlingBehavior: ["DEFAULT"],
      name: "feature",
    },
    hasValidationErrors: overrides?.hasValidationErrors ?? false,
    initialValues: {
      handlingBehavior: undefined,
      states: [{ name: "queued", type: "INITIAL" }],
      workTypeName: "story",
    },
    isDirty: true,
    markChangesSaved: overrides?.markChangesSaved ?? vi.fn(),
    onHandlingBehaviorChange: vi.fn(),
    onNameChange: vi.fn(),
    onResetToLatest: vi.fn(),
    pendingFactoryDefinition: {
      name: "Current Factory",
      workTypes: [
        {
          handlingBehavior: ["DEFAULT"],
          name: "feature",
          states: [{ name: "queued", type: "INITIAL" }],
        },
      ],
      workstations: [],
      workers: [],
    },
    status: "ready",
    validationErrors: overrides?.validationErrors ?? {},
  };
}
