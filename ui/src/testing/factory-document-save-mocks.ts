import { type Mock, vi } from "vitest";

import {
  CurrentFactoryDefinitionError,
  type CurrentFactoryDocument,
  type SaveCurrentFactoryInput,
} from "../api/current-factory-definition";
import { createDeferredPromise } from "./app-shell-export-test-utils";
import { staleFactoryVersionTarget } from "./factory-validation-target-fixtures";

export type FactoryDocumentSaveMode = "idle" | "pending" | "success" | "error";

export type FactoryDocumentSaveErrorMode =
  | "generic"
  | "factory_not_idle"
  | "stale_version";

export interface MockFactoryDocumentSaveOptions {
  mode?: FactoryDocumentSaveMode;
  mutateAsync?: Mock<
    (input: SaveCurrentFactoryInput) => Promise<CurrentFactoryDocument>
  >;
  isPending?: boolean;
  resolvedDocument?: CurrentFactoryDocument;
  errorMode?: FactoryDocumentSaveErrorMode;
  rejectedError?: unknown;
}

export interface MockPendingFactoryDocumentSave {
  deferred: ReturnType<typeof createDeferredPromise<CurrentFactoryDocument>>;
  mutateAsync: Mock<
    (input: SaveCurrentFactoryInput) => Promise<CurrentFactoryDocument>
  >;
  saveMutation: ReturnType<typeof mockFactoryDocumentSave>;
}

const defaultSavedDocument: CurrentFactoryDocument = {
  name: "Current Factory",
  version: {
    logical: "7",
    physical: "2026-05-23T15:52:00Z",
  },
  workers: [],
  workstations: [],
  workTypes: [],
};

export function factoryDocumentSaveError(
  mode: FactoryDocumentSaveErrorMode,
  overrides?: { message?: string },
): unknown {
  switch (mode) {
    case "factory_not_idle":
      return new CurrentFactoryDefinitionError(
        overrides?.message ??
          "Current factory runtime must be idle before activation.",
        {
          code: "FACTORY_NOT_IDLE",
        },
      );
    case "stale_version":
      return new CurrentFactoryDefinitionError(
        overrides?.message ??
          "Current factory definition is stale. Refresh the graph before saving.",
        {
          code: "STALE_FACTORY_VERSION",
          status: 409,
          targets: [staleFactoryVersionTarget()],
        },
      );
    default:
      return overrides?.message
        ? new Error(overrides.message)
        : new Error("Network dropped");
  }
}

export function mockFactoryDocumentSave(
  options: MockFactoryDocumentSaveOptions = {},
) {
  const mode = options.mode ?? "idle";
  const mutateAsync =
    options.mutateAsync ?? buildFactoryDocumentSaveMutateAsync(options);
  const isPending = options.isPending ?? mode === "pending";

  return {
    isPending,
    mutateAsync,
  };
}

export function mockPendingFactoryDocumentSave(): MockPendingFactoryDocumentSave {
  const deferred = createDeferredPromise<CurrentFactoryDocument>();
  const mutateAsync = vi.fn().mockReturnValue(deferred.promise) as Mock<
    (input: SaveCurrentFactoryInput) => Promise<CurrentFactoryDocument>
  >;

  return {
    deferred,
    mutateAsync,
    saveMutation: mockFactoryDocumentSave({
      isPending: true,
      mode: "pending",
      mutateAsync,
    }),
  };
}

function buildFactoryDocumentSaveMutateAsync(
  options: MockFactoryDocumentSaveOptions,
): Mock<(input: SaveCurrentFactoryInput) => Promise<CurrentFactoryDocument>> {
  const mode = options.mode ?? "idle";

  if (mode === "error") {
    const rejectedError =
      options.rejectedError ??
      factoryDocumentSaveError(options.errorMode ?? "generic");
    return vi.fn().mockRejectedValue(rejectedError) as Mock<
      (input: SaveCurrentFactoryInput) => Promise<CurrentFactoryDocument>
    >;
  }

  if (mode === "pending") {
    const deferred = createDeferredPromise<CurrentFactoryDocument>();
    return vi.fn().mockReturnValue(deferred.promise) as Mock<
      (input: SaveCurrentFactoryInput) => Promise<CurrentFactoryDocument>
    >;
  }

  return vi
    .fn()
    .mockResolvedValue(
      options.resolvedDocument ?? defaultSavedDocument,
    ) as Mock<
    (input: SaveCurrentFactoryInput) => Promise<CurrentFactoryDocument>
  >;
}
