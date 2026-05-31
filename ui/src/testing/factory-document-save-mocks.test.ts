import { describe, expect, it } from "vitest";

import { CurrentFactoryDefinitionError } from "../api/current-factory-definition";
import {
  factoryDocumentSaveError,
  mockFactoryDocumentSave,
  mockPendingFactoryDocumentSave,
} from "./factory-document-save-mocks";

describe("factory-document-save-mocks", () => {
  it("mockFactoryDocumentSave exposes the useFactoryDocumentSave hook contract", () => {
    const idle = mockFactoryDocumentSave({ mode: "idle" });
    expect(idle).toMatchObject({
      error: null,
      isPending: false,
    });
    expect(idle.saveAsync).toBeTypeOf("function");
    expect(idle.save).toBeTypeOf("function");
    expect(idle.reset).toBeTypeOf("function");
  });

  it("mockFactoryDocumentSave exposes idle success and error mutation seams", async () => {
    const success = mockFactoryDocumentSave({ mode: "success" });
    expect(success.isPending).toBe(false);
    await expect(success.saveAsync({} as never)).resolves.toEqual({
      name: "Current Factory",
      version: {
        logical: "7",
        physical: "2026-05-23T15:52:00Z",
      },
      workers: [],
      workstations: [],
      workTypes: [],
    });

    const error = mockFactoryDocumentSave({
      mode: "error",
      errorMode: "factory_not_idle",
    });
    await expect(error.saveAsync({} as never)).rejects.toBeInstanceOf(
      CurrentFactoryDefinitionError,
    );
  });

  it("factoryDocumentSaveError returns stale-version and generic failures", () => {
    expect(factoryDocumentSaveError("factory_not_idle")).toBeInstanceOf(
      CurrentFactoryDefinitionError,
    );
    expect(factoryDocumentSaveError("stale_version")).toBeInstanceOf(
      CurrentFactoryDefinitionError,
    );
    expect(factoryDocumentSaveError("generic")).toBeInstanceOf(Error);
    expect(factoryDocumentSaveError("generic", { message: "Save failed." })).toEqual(
      new Error("Save failed."),
    );
  });

  it("mockFactoryDocumentSave resolves custom documents for success mode", async () => {
    const resolvedDocument = {
      name: "Custom Factory",
      version: { logical: "11", physical: "2026-05-30T00:00:00Z" },
      workers: [],
      workstations: [],
      workTypes: [],
    };
    const success = mockFactoryDocumentSave({
      mode: "success",
      resolvedDocument,
    });

    await expect(success.saveAsync({} as never)).resolves.toEqual(
      resolvedDocument,
    );
  });

  it("mockPendingFactoryDocumentSave keeps saveAsync pending until resolved", async () => {
    const pending = mockPendingFactoryDocumentSave();
    expect(pending.saveMutation.isPending).toBe(true);

    const savePromise = pending.saveAsync({} as never);
    pending.deferred.resolve({
      name: "Saved",
      version: { logical: "8", physical: "2026-05-23T15:52:00.001Z" },
      workers: [],
      workstations: [],
      workTypes: [],
    });

    await expect(savePromise).resolves.toEqual({
      name: "Saved",
      version: { logical: "8", physical: "2026-05-23T15:52:00.001Z" },
      workers: [],
      workstations: [],
      workTypes: [],
    });
  });
});
