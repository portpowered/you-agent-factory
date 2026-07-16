// biome-ignore-all lint/style/noExcessiveLinesPerFile lint/complexity/noExcessiveLinesPerFunction: focused save-hook regressions share one mocked mutation seam.
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

import { CurrentFactoryDefinitionError } from "../../../../api/current-factory-definition";
import {
  mockFactoryDocumentSave,
  mockPendingFactoryDocumentSave,
} from "../../../../testing/factory-document-save-mocks";
import { resourceFieldValidationTarget } from "../../../../testing/factory-validation-target-fixtures";
import * as factoryDocumentSaveHooks from "../../../current-factory-definition/hooks/useFactoryDocumentSave";
import { useDashboardSessionStore } from "../../../dashboard/state/dashboardSessionStore";
import type { EditableResourceConfigurationState } from "../lib/detail-card-types";
import { useSaveEditableResourceConfiguration } from "./use-save-editable-resource-configuration";

describe("useSaveEditableResourceConfiguration", () => {
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
        useSaveEditableResourceConfiguration({
          editableConfigurationState: buildReadyEditableConfigurationState(),
          locale: "zh-CN",
          scopeKey: "agent-slot",
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
        resources: [{ capacity: 3, name: "agent-slot" }],
        workTypes: [],
      },
    });
  });

  it("saves resource edits through the selected session current-factory route", async () => {
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
        resources: [{ capacity: 3, name: "agent-slot" }],
        workers: [],
        workstations: [],
        workTypes: [],
      },
    });
    vi.spyOn(
      factoryDocumentSaveHooks,
      "useFactoryDocumentSave",
    ).mockReturnValue(saveMutation as never);
    const saveAsync = saveMutation.saveAsync;

    const { result } = renderHook(
      () =>
        useSaveEditableResourceConfiguration({
          editableConfigurationState: buildReadyEditableConfigurationState({
            isDirty: false,
            markChangesSaved,
          }),
          scopeKey: "agent-slot",
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
        resources: [{ capacity: 3, name: "agent-slot" }],
        workTypes: [],
      },
    });
  });

  it("updates resource selection after a successful rename save", async () => {
    const markChangesSaved = vi.fn();
    const onResourceRenamed = vi.fn();
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
        name: "expanded-slot",
      };
      renamedState.pendingFactoryDefinition = {
        name: "Current Factory",
        resources: [{ capacity: 3, name: "expanded-slot" }],
        workers: [
          {
            model: "gpt-5.5",
            modelProvider: "CODEX",
            name: "reviewer",
            resources: [{ capacity: 1, name: "expanded-slot" }],
            type: "MODEL_WORKER",
          },
        ],
        workstations: [
          {
            id: "review",
            name: "Review",
            resources: [{ capacity: 1, name: "expanded-slot" }],
            worker: "reviewer",
          },
        ],
      };
    }

    const { result } = renderHook(
      () =>
        useSaveEditableResourceConfiguration({
          editableConfigurationState: renamedState,
          onResourceRenamed,
          scopeKey: "agent-slot",
        }),
      { wrapper: createQueryClientWrapper() },
    );

    await act(async () => {
      await result.current.save();
    });

    expect(markChangesSaved).toHaveBeenCalledTimes(1);
    expect(onResourceRenamed).toHaveBeenCalledWith("expanded-slot");
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
        useSaveEditableResourceConfiguration({
          editableConfigurationState: buildReadyEditableConfigurationState(),
          scopeKey: "agent-slot",
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
      resources: [{ capacity: 3, name: "agent-slot" }],
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
    vi.spyOn(
      factoryDocumentSaveHooks,
      "useFactoryDocumentSave",
    ).mockReturnValue(saveMutation as never);

    const { result } = renderHook(
      () =>
        useSaveEditableResourceConfiguration({
          editableConfigurationState: buildReadyEditableConfigurationState(),
          scopeKey: "agent-slot",
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

  it("clears save feedback when the selected resource scope changes", async () => {
    const saveAsync = vi.fn().mockResolvedValue({
      name: "Current Factory",
      version: {
        logical: "8",
        physical: "2026-05-23T15:52:00.001Z",
      },
      resources: [{ capacity: 3, name: "agent-slot" }],
      workers: [],
      workstations: [],
      workTypes: [],
    });
    vi.spyOn(
      factoryDocumentSaveHooks,
      "useFactoryDocumentSave",
    ).mockReturnValue({
      isPending: false,
      saveAsync,
    } as never);

    const { rerender, result } = renderHook(
      ({ scopeKey }) =>
        useSaveEditableResourceConfiguration({
          editableConfigurationState: buildReadyEditableConfigurationState({
            isDirty: false,
          }),
          scopeKey,
        }),
      {
        initialProps: { scopeKey: "agent-slot" },
        wrapper: createQueryClientWrapper(),
      },
    );

    await act(async () => {
      await result.current.save();
    });

    await waitFor(() => {
      expect(result.current.saveState).toEqual({ status: "success" });
    });

    rerender({ scopeKey: "voice-model" });

    expect(result.current.saveState).toEqual({ status: "idle" });
  });

  it("maps targeted save validation failures onto resource field errors", async () => {
    const message = "Resource capacity must be positive.";
    const saveAsync = vi.fn().mockRejectedValue(
      new CurrentFactoryDefinitionError(message, {
        code: "BAD_REQUEST",
        status: 400,
        targets: [resourceFieldValidationTarget("capacity", message)],
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
        useSaveEditableResourceConfiguration({
          editableConfigurationState: buildReadyEditableConfigurationState(),
          scopeKey: "agent-slot",
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
          capacity: message,
        },
        status: "error",
      });
    });
  });

  it.each([
    ["factory.resources[0].backend", "backend"],
    ["factory.resources[0].loadPolicy", "loadPolicy"],
    ["factory.resources[0].model", "model"],
    ["factory.resources[0].provider", "provider"],
    ["factory.resources[0].type", "type"],
    ["factory.resources[0].name", "name"],
  ] as const)("maps save validation target %s onto %s", async (_field, expectedField) => {
    const message = `Invalid ${expectedField}.`;
    const saveAsync = vi.fn().mockRejectedValue(
      new CurrentFactoryDefinitionError(message, {
        code: "BAD_REQUEST",
        status: 400,
        targets: [resourceFieldValidationTarget(expectedField, message)],
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
        useSaveEditableResourceConfiguration({
          editableConfigurationState: buildReadyEditableConfigurationState(),
          scopeKey: "agent-slot",
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
  isDirty?: boolean;
  markChangesSaved?: () => void;
}): EditableResourceConfigurationState {
  return {
    baseVersion: {
      logical: "7",
      physical: "2026-05-23T15:52:00Z",
    },
    canSave: true,
    draft: {
      backend: "",
      capacityText: "3",
      loadPolicy: "",
      model: "",
      name: "agent-slot",
      provider: "",
      type: "INVOCATION_SLOT",
    },
    hasValidationErrors: false,
    initialValues: {
      backend: "",
      capacity: 2,
      capacityText: "2",
      loadPolicy: "",
      model: "",
      name: "agent-slot",
      provider: "",
      resourceName: "agent-slot",
      type: "INVOCATION_SLOT",
      workerNames: ["reviewer"],
      workstationNames: ["Review"],
    },
    isDirty: overrides?.isDirty ?? true,
    markChangesSaved: overrides?.markChangesSaved ?? vi.fn(),
    onBackendChange: vi.fn(),
    onCapacityChange: vi.fn(),
    onLoadPolicyChange: vi.fn(),
    onModelChange: vi.fn(),
    onNameChange: vi.fn(),
    onProviderChange: vi.fn(),
    onResetToLatest: vi.fn(),
    onTypeChange: vi.fn(),
    overwriteFieldNames: [],
    pendingFactoryDefinition: {
      name: "Current Factory",
      resources: [{ capacity: 3, name: "agent-slot" }],
      workTypes: [],
    },
    savedFactoryDefinition: {
      name: "Current Factory",
      resources: [{ capacity: 2, name: "agent-slot" }],
      workTypes: [],
    },
    status: "ready",
    validationErrors: {},
  };
}
