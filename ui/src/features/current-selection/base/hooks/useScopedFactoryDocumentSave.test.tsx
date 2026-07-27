import { act, renderHook, waitFor } from "@testing-library/react";

import { CurrentFactoryDefinitionError } from "../../../../api/current-factory-definition";
import {
  mockFactoryDocumentSave,
  mockPendingFactoryDocumentSave,
} from "../../../../testing/factory-document-save-mocks";
import { staleFactoryVersionTarget } from "../../../../testing/factory-validation-target-fixtures";
import {
  createScopedFactoryDocumentSaveQueryClientWrapper,
  defaultScopedFactoryDocumentSaveRequest,
  seedScopedFactoryDocumentSaveTestSession,
} from "../../../../testing/scoped-factory-document-save-test-helpers";
import * as factoryDocumentSaveHooks from "../../../current-factory-definition/hooks/useFactoryDocumentSave";
import { useScopedFactoryDocumentSave } from "./useScopedFactoryDocumentSave";

beforeEach(() => {
  seedScopedFactoryDocumentSaveTestSession();
  vi.restoreAllMocks();
});

describe("useScopedFactoryDocumentSave saveAttemptRevision", () => {
  it("starts at zero", () => {
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
      { wrapper: createScopedFactoryDocumentSaveQueryClientWrapper() },
    );

    expect(result.current.saveAttemptRevision).toBe(0);
  });

  it("increments on each saveNow or confirmSave invocation that starts a save", async () => {
    const saveMutation = mockFactoryDocumentSave({ mode: "success" });
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
      { wrapper: createScopedFactoryDocumentSaveQueryClientWrapper() },
    );

    await act(async () => {
      await result.current.saveNow(defaultScopedFactoryDocumentSaveRequest);
    });
    expect(result.current.saveAttemptRevision).toBe(1);

    await act(async () => {
      await result.current.saveNow(defaultScopedFactoryDocumentSaveRequest);
    });
    expect(result.current.saveAttemptRevision).toBe(2);

    act(() => {
      result.current.beginConfirmation();
    });

    await act(async () => {
      await result.current.confirmSave(defaultScopedFactoryDocumentSaveRequest);
    });
    expect(result.current.saveAttemptRevision).toBe(3);
  });

  it("does not increment on confirmation-only state transitions", () => {
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
      { wrapper: createScopedFactoryDocumentSaveQueryClientWrapper() },
    );

    act(() => {
      result.current.beginConfirmation();
    });
    expect(result.current.saveAttemptRevision).toBe(0);

    act(() => {
      result.current.cancelConfirmation();
    });
    expect(result.current.saveAttemptRevision).toBe(0);
  });

  it("does not increment when duplicate save calls are deduplicated while in flight", async () => {
    const pendingSave = mockPendingFactoryDocumentSave();
    vi.spyOn(
      factoryDocumentSaveHooks,
      "useFactoryDocumentSave",
    ).mockReturnValue(pendingSave.saveMutation as never);

    const { result } = renderHook(
      () =>
        useScopedFactoryDocumentSave({
          fallbackErrorMessage: "Unable to save the active factory.",
          scopeKey: "review:transition:Review",
        }),
      { wrapper: createScopedFactoryDocumentSaveQueryClientWrapper() },
    );

    await act(async () => {
      void result.current.saveNow(defaultScopedFactoryDocumentSaveRequest);
      await Promise.resolve();
      await result.current.saveNow(defaultScopedFactoryDocumentSaveRequest);
    });

    expect(result.current.saveAttemptRevision).toBe(1);

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
      await Promise.resolve();
    });
  });

  it("resets when scopeKey changes", async () => {
    const saveMutation = mockFactoryDocumentSave({ mode: "success" });
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
        wrapper: createScopedFactoryDocumentSaveQueryClientWrapper(),
      },
    );

    await act(async () => {
      await result.current.saveNow(defaultScopedFactoryDocumentSaveRequest);
    });
    expect(result.current.saveAttemptRevision).toBe(1);

    rerender({ scopeKey: "worker:reviewer" });
    expect(result.current.saveAttemptRevision).toBe(0);
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
      { wrapper: createScopedFactoryDocumentSaveQueryClientWrapper() },
    );

    act(() => {
      result.current.beginConfirmation();
    });

    let firstSave: Promise<void> | undefined;
    await act(async () => {
      firstSave = result.current.confirmSave(
        defaultScopedFactoryDocumentSaveRequest,
      );
      await Promise.resolve();
      await result.current.confirmSave(defaultScopedFactoryDocumentSaveRequest);
      await result.current.saveNow(defaultScopedFactoryDocumentSaveRequest);
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
      { wrapper: createScopedFactoryDocumentSaveQueryClientWrapper() },
    );

    await act(async () => {
      await result.current.saveNow(defaultScopedFactoryDocumentSaveRequest);
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
            return Object.keys(fieldErrors).length > 0
              ? fieldErrors
              : undefined;
          },
          scopeKey: "review:transition:Review",
        }),
      { wrapper: createScopedFactoryDocumentSaveQueryClientWrapper() },
    );

    await act(async () => {
      await result.current.saveNow(defaultScopedFactoryDocumentSaveRequest);
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
      { wrapper: createScopedFactoryDocumentSaveQueryClientWrapper() },
    );

    await act(async () => {
      await result.current.saveNow(defaultScopedFactoryDocumentSaveRequest);
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
      { wrapper: createScopedFactoryDocumentSaveQueryClientWrapper() },
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
      { wrapper: createScopedFactoryDocumentSaveQueryClientWrapper() },
    );

    act(() => {
      result.current.beginConfirmation();
    });

    await act(async () => {
      await result.current.confirmSave({
        ...defaultScopedFactoryDocumentSaveRequest,
        onSaved,
      });
    });

    await waitFor(() => {
      expect(result.current.saveState).toEqual({ status: "success" });
    });
    expect(onSaved).toHaveBeenCalledTimes(1);
    expect(saveAsync).toHaveBeenCalledWith({
      baseVersion: defaultScopedFactoryDocumentSaveRequest.baseVersion,
      factory: defaultScopedFactoryDocumentSaveRequest.factory,
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
      { wrapper: createScopedFactoryDocumentSaveQueryClientWrapper() },
    );

    act(() => {
      result.current.beginConfirmation();
    });

    await act(async () => {
      await result.current.confirmSave(defaultScopedFactoryDocumentSaveRequest);
    });

    await waitFor(() => {
      expect(result.current.saveState.status).toBe("warning");
    });
    expect(result.current.isPending).toBe(false);
  });
});
