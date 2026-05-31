import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

import { CurrentFactoryDefinitionError } from "../../../../api/current-factory-definition";
import {
  mockFactoryDocumentSave,
  mockPendingFactoryDocumentSave,
} from "../../../../testing/factory-document-save-mocks";
import { staleFactoryVersionTarget } from "../../../../testing/factory-validation-target-fixtures";
import * as factoryDocumentSaveHooks from "../../../current-factory-definition/hooks/useFactoryDocumentSave";
import { DashboardSessionProvider } from "../../../dashboard/session/dashboard-session-provider";
import { useDashboardSessionStore } from "../../../dashboard/state/dashboardSessionStore";
import type { ScopedFactoryDocumentSaveRequest } from "./useScopedFactoryDocumentSave";
import { useScopedFactoryDocumentSave } from "./useScopedFactoryDocumentSave";

const defaultSaveRequest: ScopedFactoryDocumentSaveRequest = {
  baseVersion: {
    logical: "7",
    physical: "2026-05-23T15:52:00Z",
  },
  factory: {
    name: "Current Factory",
    workers: [],
    workstations: [],
  },
  scopeKey: "review:transition:Review",
};

beforeEach(() => {
  useDashboardSessionStore.setState({ selectedSessionID: "~default" });
  vi.restoreAllMocks();
});

describe("useScopedFactoryDocumentSave scope isolation", () => {
  it("clears confirmation, success, warning, and error state when scopeKey changes", async () => {
    const saveMutation = mockFactoryDocumentSave({
      mode: "error",
      rejectedError: new CurrentFactoryDefinitionError("Save failed.", {
        code: "BAD_REQUEST",
      }),
    });
    vi.spyOn(
      factoryDocumentSaveHooks,
      "useFactoryDocumentSave",
    ).mockReturnValue(saveMutation as never);

    const { rerender, result } = renderHook(
      ({ scopeKey }) =>
        useScopedFactoryDocumentSave({
          fallbackErrorMessage: "Unable to save the active factory.",
          scopeKey,
        }),
      {
        initialProps: { scopeKey: "review:transition:Review" },
        wrapper: createQueryClientWrapper(),
      },
    );

    act(() => {
      result.current.beginConfirmation();
    });
    expect(result.current.saveState).toEqual({ status: "confirming" });

    await act(async () => {
      await result.current.confirmSave(defaultSaveRequest);
    });

    await waitFor(() => {
      expect(result.current.saveState).toEqual({
        errorMessage: "Save failed.",
        status: "error",
      });
    });

    rerender({ scopeKey: "worker:reviewer" });

    expect(result.current.saveState).toEqual({ status: "idle" });
  });

  it("surfaces success on a new scope after rerendering from a previous scope", async () => {
    const saveMutation = mockFactoryDocumentSave({ mode: "success" });
    vi.spyOn(
      factoryDocumentSaveHooks,
      "useFactoryDocumentSave",
    ).mockReturnValue(saveMutation as never);

    const scopeBSaveRequest: ScopedFactoryDocumentSaveRequest = {
      ...defaultSaveRequest,
      scopeKey: "worker:reviewer",
    };

    const { rerender, result } = renderHook(
      ({ scopeKey }) =>
        useScopedFactoryDocumentSave({
          fallbackErrorMessage: "Unable to save the active factory.",
          scopeKey,
        }),
      {
        initialProps: { scopeKey: "review:transition:Review" },
        wrapper: createQueryClientWrapper(),
      },
    );

    rerender({ scopeKey: "worker:reviewer" });

    await act(async () => {
      await result.current.saveNow(scopeBSaveRequest);
    });

    await waitFor(() => {
      expect(result.current.saveState).toEqual({ status: "success" });
    });
  });

  it("clears success when the draft becomes dirty again for the current scope", async () => {
    const saveMutation = mockFactoryDocumentSave({ mode: "success" });
    vi.spyOn(
      factoryDocumentSaveHooks,
      "useFactoryDocumentSave",
    ).mockReturnValue(saveMutation as never);

    const { rerender, result } = renderHook(
      ({ isDirty }) =>
        useScopedFactoryDocumentSave({
          fallbackErrorMessage: "Unable to save the active factory.",
          isDirty,
          scopeKey: "review:transition:Review",
        }),
      {
        initialProps: { isDirty: false },
        wrapper: createQueryClientWrapper(),
      },
    );

    await act(async () => {
      await result.current.saveNow(defaultSaveRequest);
    });

    await waitFor(() => {
      expect(result.current.saveState).toEqual({ status: "success" });
    });

    rerender({ isDirty: true });

    expect(result.current.saveState).toEqual({ status: "idle" });
  });

  it("does not apply success state or onSaved when scopeKey changes during an in-flight save", async () => {
    const pendingSave = mockPendingFactoryDocumentSave();
    vi.spyOn(
      factoryDocumentSaveHooks,
      "useFactoryDocumentSave",
    ).mockReturnValue(pendingSave.saveMutation as never);
    const onSaved = vi.fn();

    const { rerender, result } = renderHook(
      ({ scopeKey }) =>
        useScopedFactoryDocumentSave({
          fallbackErrorMessage: "Unable to save the active factory.",
          scopeKey,
        }),
      {
        initialProps: { scopeKey: "review:transition:Review" },
        wrapper: createQueryClientWrapper(),
      },
    );

    let savePromise: Promise<void> | undefined;
    await act(async () => {
      savePromise = result.current.saveNow({
        ...defaultSaveRequest,
        onSaved,
      });
      await Promise.resolve();
    });

    expect(result.current.saveState).toEqual({ status: "submitting" });

    rerender({ scopeKey: "worker:reviewer" });
    expect(result.current.saveState).toEqual({ status: "idle" });

    pendingSave.deferred.resolve({
      name: "Current Factory",
      version: {
        logical: "8",
        physical: "2026-05-23T15:52:00.001Z",
      },
      workers: [],
      workstations: [],
    });

    await act(async () => {
      await savePromise;
    });

    expect(onSaved).not.toHaveBeenCalled();
    expect(result.current.saveState).toEqual({ status: "idle" });
  });

  it("does not apply error state when scopeKey changes during an in-flight save", async () => {
    const pendingSave = mockPendingFactoryDocumentSave();
    vi.spyOn(
      factoryDocumentSaveHooks,
      "useFactoryDocumentSave",
    ).mockReturnValue(pendingSave.saveMutation as never);

    const { rerender, result } = renderHook(
      ({ scopeKey }) =>
        useScopedFactoryDocumentSave({
          fallbackErrorMessage: "Unable to save the active factory.",
          scopeKey,
        }),
      {
        initialProps: { scopeKey: "review:transition:Review" },
        wrapper: createQueryClientWrapper(),
      },
    );

    let savePromise: Promise<void> | undefined;
    await act(async () => {
      savePromise = result.current.saveNow(defaultSaveRequest);
      await Promise.resolve();
    });

    rerender({ scopeKey: "worker:reviewer" });

    pendingSave.deferred.reject(
      new CurrentFactoryDefinitionError("Save failed.", {
        code: "BAD_REQUEST",
      }),
    );

    await act(async () => {
      await savePromise;
    });

    expect(result.current.saveState).toEqual({ status: "idle" });
  });
});

describe("useScopedFactoryDocumentSave in-flight deduplication", () => {
  it("ignores repeated confirm and immediate save calls while a save is in flight", async () => {
    const pendingSave = mockPendingFactoryDocumentSave();
    vi.spyOn(
      factoryDocumentSaveHooks,
      "useFactoryDocumentSave",
    ).mockReturnValue(pendingSave.saveMutation as never);
    const saveAsync = pendingSave.saveAsync;

    const { result } = renderHook(
      () =>
        useScopedFactoryDocumentSave({
          fallbackErrorMessage: "Unable to save the active factory.",
          scopeKey: "review:transition:Review",
        }),
      { wrapper: createQueryClientWrapper() },
    );

    act(() => {
      result.current.beginConfirmation();
    });

    let firstSave: Promise<void> | undefined;
    await act(async () => {
      firstSave = result.current.confirmSave(defaultSaveRequest);
      await Promise.resolve();
      await result.current.confirmSave(defaultSaveRequest);
      await result.current.saveNow(defaultSaveRequest);
    });

    expect(saveAsync).toHaveBeenCalledTimes(1);
    expect(result.current.saveState).toEqual({ status: "submitting" });

    pendingSave.deferred.resolve({
      name: "Current Factory",
      version: {
        logical: "8",
        physical: "2026-05-23T15:52:00.001Z",
      },
      workers: [],
      workstations: [],
    });

    await act(async () => {
      await firstSave;
    });

    await waitFor(() => {
      expect(result.current.saveState).toEqual({ status: "success" });
    });
  });
});

describe("useScopedFactoryDocumentSave error normalization", () => {
  it("maps STALE_FACTORY_VERSION to warning status", async () => {
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
        useScopedFactoryDocumentSave({
          fallbackErrorMessage: "Unable to save the active factory.",
          scopeKey: "review:transition:Review",
        }),
      { wrapper: createQueryClientWrapper() },
    );

    await act(async () => {
      await result.current.saveNow(defaultSaveRequest);
    });

    await waitFor(() => {
      expect(result.current.saveState).toEqual({
        message:
          "Current factory definition is stale. Refresh the dashboard before saving or importing again.",
        status: "warning",
      });
    });
  });

  it("maps factory validation errors through the caller field-error mapper", async () => {
    const saveMutation = mockFactoryDocumentSave({
      mode: "error",
      rejectedError: new CurrentFactoryDefinitionError(
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
    });
    vi.spyOn(
      factoryDocumentSaveHooks,
      "useFactoryDocumentSave",
    ).mockReturnValue(saveMutation as never);

    const { result } = renderHook(
      () =>
        useScopedFactoryDocumentSave({
          fallbackErrorMessage: "Unable to save the active factory.",
          mapSaveErrorToFieldErrors: (error) => {
            const fieldErrors: Record<string, string> = {};
            for (const target of error.targets ?? []) {
              if (
                target.code === "factory.worker.danglingReference" &&
                target.subject.type === "WORKSTATION"
              ) {
                fieldErrors.workerName = error.message;
              }
            }
            return Object.keys(fieldErrors).length > 0 ? fieldErrors : undefined;
          },
          scopeKey: "review:transition:Review",
        }),
      { wrapper: createQueryClientWrapper() },
    );

    await act(async () => {
      await result.current.saveNow(defaultSaveRequest);
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

  it("uses fallback copy for unknown save errors", async () => {
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
        useScopedFactoryDocumentSave({
          fallbackErrorMessage: "Unable to save the active factory.",
          scopeKey: "review:transition:Review",
        }),
      { wrapper: createQueryClientWrapper() },
    );

    await act(async () => {
      await result.current.saveNow(defaultSaveRequest);
    });

    await waitFor(() => {
      expect(result.current.saveState).toEqual({
        errorMessage: "Unable to save the active factory.",
        status: "error",
      });
    });
  });
});

describe("useScopedFactoryDocumentSave confirm mode", () => {
  it("closes confirmation on cancel when no save is pending", () => {
    const saveMutation = mockFactoryDocumentSave({ mode: "idle" });
    vi.spyOn(
      factoryDocumentSaveHooks,
      "useFactoryDocumentSave",
    ).mockReturnValue(saveMutation as never);

    const { result } = renderHook(
      () =>
        useScopedFactoryDocumentSave({
          fallbackErrorMessage: "Unable to save the active factory.",
          scopeKey: "review:transition:Review",
        }),
      { wrapper: createQueryClientWrapper() },
    );

    act(() => {
      result.current.beginConfirmation();
    });
    expect(result.current.saveState).toEqual({ status: "confirming" });

    act(() => {
      result.current.cancelConfirmation();
    });
    expect(result.current.saveState).toEqual({ status: "idle" });
  });

  it("persists through useFactoryDocumentSave and invokes onSaved after confirm", async () => {
    const onSaved = vi.fn();
    const saveMutation = mockFactoryDocumentSave({ mode: "success" });
    vi.spyOn(
      factoryDocumentSaveHooks,
      "useFactoryDocumentSave",
    ).mockReturnValue(saveMutation as never);
    const saveAsync = saveMutation.saveAsync;

    const { result } = renderHook(
      () =>
        useScopedFactoryDocumentSave({
          fallbackErrorMessage: "Unable to save the active factory.",
          scopeKey: "review:transition:Review",
        }),
      { wrapper: createQueryClientWrapper() },
    );

    act(() => {
      result.current.beginConfirmation();
    });

    await act(async () => {
      await result.current.confirmSave({
        ...defaultSaveRequest,
        onSaved,
      });
    });

    await waitFor(() => {
      expect(result.current.saveState).toEqual({ status: "success" });
    });
    expect(onSaved).toHaveBeenCalledTimes(1);
    expect(saveAsync).toHaveBeenCalledWith({
      baseVersion: defaultSaveRequest.baseVersion,
      factory: defaultSaveRequest.factory,
    });
  });

  it("surfaces stale-version warnings after confirm without blocking future saves", async () => {
    const saveMutation = mockFactoryDocumentSave({
      mode: "error",
      rejectedError: new CurrentFactoryDefinitionError(
        "Current factory definition is stale. Refresh the dashboard before saving or importing again.",
        {
          code: "STALE_FACTORY_VERSION",
          status: 409,
          targets: [staleFactoryVersionTarget()],
        },
      ),
    });
    vi.spyOn(
      factoryDocumentSaveHooks,
      "useFactoryDocumentSave",
    ).mockReturnValue(saveMutation as never);

    const { result } = renderHook(
      () =>
        useScopedFactoryDocumentSave({
          fallbackErrorMessage: "Unable to save the active factory.",
          scopeKey: "review:transition:Review",
        }),
      { wrapper: createQueryClientWrapper() },
    );

    act(() => {
      result.current.beginConfirmation();
    });

    await act(async () => {
      await result.current.confirmSave(defaultSaveRequest);
    });

    await waitFor(() => {
      expect(result.current.saveState.status).toBe("warning");
    });
    expect(result.current.isPending).toBe(false);
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
