import { act, renderHook, waitFor } from "@testing-library/react";

import { CurrentFactoryDefinitionError } from "../../../../api/current-factory-definition";
import {
  mockFactoryDocumentSave,
  mockPendingFactoryDocumentSave,
} from "../../../../testing/factory-document-save-mocks";
import {
  createScopedFactoryDocumentSaveQueryClientWrapper,
  defaultScopedFactoryDocumentSaveRequest,
  seedScopedFactoryDocumentSaveTestSession,
} from "../../../../testing/scoped-factory-document-save-test-helpers";
import * as factoryDocumentSaveHooks from "../../../current-factory-definition/hooks/useFactoryDocumentSave";
import type { ScopedFactoryDocumentSaveRequest } from "./useScopedFactoryDocumentSave";
import { useScopedFactoryDocumentSave } from "./useScopedFactoryDocumentSave";

beforeEach(() => {
  seedScopedFactoryDocumentSaveTestSession();
  vi.restoreAllMocks();
});

describe("useScopedFactoryDocumentSave scope isolation when scopeKey changes", () => {
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
        wrapper: createScopedFactoryDocumentSaveQueryClientWrapper(),
      },
    );

    act(() => {
      result.current.beginConfirmation();
    });
    expect(result.current.saveState).toEqual({ status: "confirming" });

    await act(async () => {
      await result.current.confirmSave(defaultScopedFactoryDocumentSaveRequest);
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
      ...defaultScopedFactoryDocumentSaveRequest,
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
        wrapper: createScopedFactoryDocumentSaveQueryClientWrapper(),
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
});

describe("useScopedFactoryDocumentSave scope isolation for dirty drafts", () => {
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
        wrapper: createScopedFactoryDocumentSaveQueryClientWrapper(),
      },
    );

    await act(async () => {
      await result.current.saveNow(defaultScopedFactoryDocumentSaveRequest);
    });

    await waitFor(() => {
      expect(result.current.saveState).toEqual({ status: "success" });
    });

    rerender({ isDirty: true });

    expect(result.current.saveState).toEqual({ status: "idle" });
  });
});

describe("useScopedFactoryDocumentSave scope isolation during in-flight save", () => {
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
        wrapper: createScopedFactoryDocumentSaveQueryClientWrapper(),
      },
    );

    let savePromise: Promise<void> | undefined;
    await act(async () => {
      savePromise = result.current.saveNow({
        ...defaultScopedFactoryDocumentSaveRequest,
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
        wrapper: createScopedFactoryDocumentSaveQueryClientWrapper(),
      },
    );

    let savePromise: Promise<void> | undefined;
    await act(async () => {
      savePromise = result.current.saveNow(
        defaultScopedFactoryDocumentSaveRequest,
      );
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
