/**
 * Shared doubles for `useFactoryDocumentSave` in graph-editor and current-selection tests.
 *
 * Contract: mock `saveAsync({ baseVersion?, factory, mode?, sessionID? })` and optional
 * `isPending` / `error` / `reset` / `save` on the hook return value. Prefer
 * `mockFactoryDocumentSave` or `mockPendingFactoryDocumentSave` over inline mutation stubs.
 */
import { type Mock, vi } from "vitest";

import {
  CurrentFactoryDefinitionError,
  type CurrentFactoryDocument,
} from "../api/current-factory-definition";
import type { FactoryDocumentSaveInput } from "../features/current-factory-definition/hooks/useFactoryDocumentSave";
import { createDeferredPromise } from "./app-shell-export-test-utils";
import { staleFactoryVersionTarget } from "./factory-validation-target-fixtures";

export type FactoryDocumentSaveMode = "idle" | "pending" | "success" | "error";

export type FactoryDocumentSaveErrorMode =
  | "generic"
  | "factory_not_idle"
  | "stale_version";

export interface MockFactoryDocumentSaveOptions {
  mode?: FactoryDocumentSaveMode;
  saveAsync?: Mock<
    (input: FactoryDocumentSaveInput) => Promise<CurrentFactoryDocument>
  >;
  isPending?: boolean;
  error?: Error | null;
  resolvedDocument?: CurrentFactoryDocument;
  errorMode?: FactoryDocumentSaveErrorMode;
  rejectedError?: unknown;
}

export interface MockFactoryDocumentSaveReturn {
  error: Error | null;
  isPending: boolean;
  reset: () => void;
  save: (input: FactoryDocumentSaveInput) => void;
  saveAsync: Mock<
    (input: FactoryDocumentSaveInput) => Promise<CurrentFactoryDocument>
  >;
}

export interface MockPendingFactoryDocumentSave {
  deferred: ReturnType<typeof createDeferredPromise<CurrentFactoryDocument>>;
  saveAsync: Mock<
    (input: FactoryDocumentSaveInput) => Promise<CurrentFactoryDocument>
  >;
  saveMutation: MockFactoryDocumentSaveReturn;
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
          "The current factory runtime is still active. Wait until it becomes idle before saving or switching factories.",
        {
          code: "FACTORY_NOT_IDLE",
        },
      );
    case "stale_version":
      return new CurrentFactoryDefinitionError(
        overrides?.message ??
          "Current factory definition is stale. Refresh the dashboard before saving or importing again.",
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
): MockFactoryDocumentSaveReturn {
  const mode = options.mode ?? "idle";
  const saveAsync =
    options.saveAsync ?? buildFactoryDocumentSaveAsync(options);
  const isPending = options.isPending ?? mode === "pending";
  const save = vi.fn((input: FactoryDocumentSaveInput) => {
    void saveAsync(input);
  });

  return {
    error: options.error ?? null,
    isPending,
    reset: vi.fn<() => void>(),
    save,
    saveAsync,
  };
}

export function mockPendingFactoryDocumentSave(): MockPendingFactoryDocumentSave {
  const deferred = createDeferredPromise<CurrentFactoryDocument>();
  const saveAsync = vi.fn().mockReturnValue(deferred.promise) as Mock<
    (input: FactoryDocumentSaveInput) => Promise<CurrentFactoryDocument>
  >;

  return {
    deferred,
    saveAsync,
    saveMutation: mockFactoryDocumentSave({
      isPending: true,
      mode: "pending",
      saveAsync,
    }),
  };
}

function buildFactoryDocumentSaveAsync(
  options: MockFactoryDocumentSaveOptions,
): Mock<(input: FactoryDocumentSaveInput) => Promise<CurrentFactoryDocument>> {
  const mode = options.mode ?? "idle";

  if (mode === "error") {
    const rejectedError =
      options.rejectedError ??
      factoryDocumentSaveError(options.errorMode ?? "generic");
    return vi.fn().mockRejectedValue(rejectedError) as Mock<
      (input: FactoryDocumentSaveInput) => Promise<CurrentFactoryDocument>
    >;
  }

  if (mode === "pending") {
    const deferred = createDeferredPromise<CurrentFactoryDocument>();
    return vi.fn().mockReturnValue(deferred.promise) as Mock<
      (input: FactoryDocumentSaveInput) => Promise<CurrentFactoryDocument>
    >;
  }

  return vi
    .fn()
    .mockResolvedValue(
      options.resolvedDocument ?? defaultSavedDocument,
    ) as Mock<
    (input: FactoryDocumentSaveInput) => Promise<CurrentFactoryDocument>
  >;
}
