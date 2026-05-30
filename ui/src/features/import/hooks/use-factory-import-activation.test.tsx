import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

import {
  activateImportedFactoryForSession,
  createFactory,
  NamedFactoryAPIError,
  type FactoryValue,
} from "../../../api/named-factory";

vi.mock("../../../api/named-factory", async () => {
  const actual = await vi.importActual<typeof import("../../../api/named-factory")>(
    "../../../api/named-factory",
  );
  return {
    ...actual,
    activateImportedFactoryForSession: vi.fn(actual.activateImportedFactoryForSession),
    createFactory: vi.fn(actual.createFactory),
  };
});
import { writeFactoryExportPng } from "../../export/lib/factory-png-export";
import {
  PORT_OS_FACTORY_PNG_SCHEMA_VERSION,
  readFactoryImportPng,
} from "../lib/factory-png-import";
import { createFactoryImportConfirmInput } from "../lib/factory-import-confirm-input.test-helpers";
import { useFactoryImportActivation } from "./use-factory-import-activation";

const ONE_PIXEL_PNG_BASE64 =
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4////fwAJ+wP9KobjigAAAABJRU5ErkJggg==";

const canonicalFactory: FactoryValue = {
  id: "agent-factory",
  name: "Factory Roundtrip",
  workTypes: [
    {
      name: "story",
      states: [
        { name: "new", type: "INITIAL" },
        { name: "done", type: "TERMINAL" },
      ],
    },
  ],
  workers: [
    {
      executorProvider: "SCRIPT_WRAP",
      model: "codex-mini",
      modelProvider: "CODEX",
      name: "writer",
      type: "MODEL_WORKER",
    },
  ],
  workstations: [
    {
      inputs: [{ state: "new", workType: "story" }],
      name: "draft",
      onContinue: [
        { state: "new", workType: "story" },
        { state: "queued", workType: "story" },
      ],
      onFailure: [
        { state: "done", workType: "story" },
        { state: "blocked", workType: "story" },
      ],
      onRejection: [
        { state: "retry", workType: "story" },
        { state: "backlog", workType: "story" },
      ],
      outputs: [{ state: "done", workType: "story" }],
      worker: "writer",
    },
  ],
};

const mockedActivateImportedFactoryForSession = vi.mocked(activateImportedFactoryForSession);
const mockedCreateFactory = vi.mocked(createFactory);

describe("useFactoryImportActivation", () => {
  beforeEach(() => {
    mockedActivateImportedFactoryForSession.mockClear();
    mockedCreateFactory.mockClear();
  });

  it("uses session-scoped activation by default instead of createFactory", async () => {
    mockedActivateImportedFactoryForSession.mockResolvedValue(canonicalFactory);

    const { result } = renderHook(
      () => useFactoryImportActivation({ sessionID: "session-2" }),
      { wrapper: createQueryClientWrapper() },
    );

    const importInput = createFactoryImportConfirmInput({
      factory: canonicalFactory,
      previewImageSrc: "blob:factory-roundtrip-preview",
      revokePreviewImageSrc: vi.fn(),
      schemaVersion: PORT_OS_FACTORY_PNG_SCHEMA_VERSION,
    });

    await act(async () => {
      await result.current.activateImport(importInput);
    });

    await waitFor(() => {
      expect(mockedActivateImportedFactoryForSession).toHaveBeenCalledWith(
        canonicalFactory,
        {
          choice: "replace_current",
          createFactoryName: importInput.createFactoryName,
          existingFactoryNames: importInput.existingFactoryNames,
          sessionID: "session-2",
        },
      );
    });
    expect(mockedCreateFactory).not.toHaveBeenCalled();
  });

  it("activates against the current sessionID when the selected session changes before confirm", async () => {
    mockedActivateImportedFactoryForSession.mockResolvedValue(canonicalFactory);
    const importValue = {
      factory: canonicalFactory,
      previewImageSrc: "blob:factory-roundtrip-preview",
      revokePreviewImageSrc: vi.fn(),
      schemaVersion: PORT_OS_FACTORY_PNG_SCHEMA_VERSION,
    };

    const { result, rerender } = renderHook(
      ({ sessionID }: { sessionID: string }) =>
        useFactoryImportActivation({ sessionID }),
      {
        initialProps: { sessionID: "~default" },
        wrapper: createQueryClientWrapper(),
      },
    );

    rerender({ sessionID: "session-2" });

    await act(async () => {
      await result.current.activateImport(createFactoryImportConfirmInput(importValue));
    });

    await waitFor(() => {
      expect(mockedActivateImportedFactoryForSession).toHaveBeenCalledWith(
        canonicalFactory,
        expect.objectContaining({ sessionID: "session-2", choice: "replace_current" }),
      );
    });
    expect(mockedActivateImportedFactoryForSession).toHaveBeenCalledTimes(1);
  });

  it("activates the direct factory payload while preserving the PNG factory metadata", async () => {
    const activateFactory = vi
      .fn<(input: ReturnType<typeof createFactoryImportConfirmInput>) => Promise<FactoryValue>>()
      .mockImplementation(async (input) => input.value.factory);
    const onActivated = vi.fn<(value: FactoryValue) => void>();
    const pngBytes = fromBase64(ONE_PIXEL_PNG_BASE64);
    const exportResult = await writeFactoryExportPng({
      factory: canonicalFactory,
      image: new Blob([toArrayBuffer(pngBytes)], { type: "image/png" }),
      rasterizeImageToPngBytes: async () => pngBytes,
    });

    expect(exportResult.ok).toBe(true);
    if (!exportResult.ok) {
      throw new Error("expected export to succeed");
    }

    expect(exportResult.metadata).toEqual({
      ...canonicalFactory,
      schemaVersion: PORT_OS_FACTORY_PNG_SCHEMA_VERSION,
    });

    const importResult = await readFactoryImportPng({
      createPreviewImageSrc: () => "blob:factory-roundtrip-preview",
      file: new File([exportResult.blob], "factory-roundtrip.png", { type: "image/png" }),
      validatePreviewImage: async () => {},
    });

    expect(importResult.ok).toBe(true);
    if (!importResult.ok) {
      throw new Error("expected import to succeed");
    }

    const { result } = renderHook(
      () => useFactoryImportActivation({ activateFactory, onActivated }),
      { wrapper: createQueryClientWrapper() },
    );

    await act(async () => {
      await result.current.activateImport(
        createFactoryImportConfirmInput(importResult.value),
      );
    });

    await waitFor(() => {
      expect(activateFactory).toHaveBeenCalledWith(
        expect.objectContaining({ value: importResult.value }),
      );
    });
    expect(activateFactory).toHaveBeenCalledTimes(1);
    expect(onActivated).toHaveBeenCalledWith(canonicalFactory);
    expect(importResult.value.factory).toEqual(canonicalFactory);
    expect(importResult.value.schemaVersion).toBe(PORT_OS_FACTORY_PNG_SCHEMA_VERSION);
    expect(result.current.activationState).toEqual({ status: "idle" });
  });

  it("reports a submitting state until activation resolves", async () => {
    let resolveActivation: ((value: FactoryValue) => void) | null = null;
    const activateFactory = vi
      .fn<(input: ReturnType<typeof createFactoryImportConfirmInput>) => Promise<FactoryValue>>()
      .mockImplementation(
        () =>
          new Promise<FactoryValue>((resolve) => {
            resolveActivation = resolve;
          }),
      );

    const { result } = renderHook(
      () => useFactoryImportActivation({ activateFactory }),
      { wrapper: createQueryClientWrapper() },
    );

    let activationPromise: Promise<void> | null = null;
    await act(async () => {
      activationPromise = result.current.activateImport(
        createFactoryImportConfirmInput({
          factory: canonicalFactory,
          previewImageSrc: "blob:factory-roundtrip-preview",
          revokePreviewImageSrc: vi.fn(),
          schemaVersion: PORT_OS_FACTORY_PNG_SCHEMA_VERSION,
        }),
      );
    });

    await waitFor(() => {
      expect(result.current.activationState).toEqual({ status: "submitting" });
    });

    resolveActivation?.(canonicalFactory);
    await act(async () => {
      await activationPromise;
    });

    await waitFor(() => {
      expect(result.current.activationState).toEqual({ status: "idle" });
    });
  });

  it("stores generic activation failures and clears them on request", async () => {
    const activationError = new Error("Factory name already exists.");
    const activateFactory = vi
      .fn<(input: ReturnType<typeof createFactoryImportConfirmInput>) => Promise<FactoryValue>>()
      .mockRejectedValue(
        Object.assign(activationError, {
          code: "FACTORY_ALREADY_EXISTS",
          name: "NamedFactoryAPIError",
        }),
      );

    const { result } = renderHook(
      () => useFactoryImportActivation({ activateFactory }),
      { wrapper: createQueryClientWrapper() },
    );

    await act(async () => {
      await result.current.activateImport(
        createFactoryImportConfirmInput({
          factory: canonicalFactory,
          previewImageSrc: "blob:factory-roundtrip-preview",
          revokePreviewImageSrc: vi.fn(),
          schemaVersion: PORT_OS_FACTORY_PNG_SCHEMA_VERSION,
        }),
      );
    });

    await waitFor(() => {
      expect(result.current.activationState.status).toBe("error");
    });
    if (result.current.activationState.status !== "error") {
      throw new Error("expected activation failure state");
    }
    expect(result.current.activationState.error).toMatchObject({
      code: "INTERNAL_ERROR",
      message: "Factory name already exists.",
    });

    act(() => {
      result.current.clearActivationError();
    });

    expect(result.current.activationState).toEqual({ status: "idle" });
  });

  it("preserves explicit named factory api errors from activation failures", async () => {
    const activateFactory = vi
      .fn<(input: ReturnType<typeof createFactoryImportConfirmInput>) => Promise<FactoryValue>>()
      .mockRejectedValue(
        new NamedFactoryAPIError(
          "Current factory runtime must be idle before activation.",
          { code: "FACTORY_NOT_IDLE" },
        ),
      );

    const { result } = renderHook(
      () => useFactoryImportActivation({ activateFactory }),
      { wrapper: createQueryClientWrapper() },
    );

    await act(async () => {
      await result.current.activateImport(
        createFactoryImportConfirmInput({
          factory: canonicalFactory,
          previewImageSrc: "blob:factory-roundtrip-preview",
          revokePreviewImageSrc: vi.fn(),
          schemaVersion: PORT_OS_FACTORY_PNG_SCHEMA_VERSION,
        }),
      );
    });

    await waitFor(() => {
      expect(result.current.activationState.status).toBe("error");
    });
    if (result.current.activationState.status !== "error") {
      throw new Error("expected activation failure state");
    }
    expect(result.current.activationState.error).toMatchObject({
      code: "FACTORY_NOT_IDLE",
      message: "Current factory runtime must be idle before activation.",
    });
  });

  it("normalizes non-error activation failures to a generic internal error", async () => {
    const activateFactory = vi
      .fn<(input: ReturnType<typeof createFactoryImportConfirmInput>) => Promise<FactoryValue>>()
      .mockRejectedValue("unstructured failure");

    const { result } = renderHook(
      () => useFactoryImportActivation({ activateFactory }),
      { wrapper: createQueryClientWrapper() },
    );

    await act(async () => {
      await result.current.activateImport(
        createFactoryImportConfirmInput({
          factory: canonicalFactory,
          previewImageSrc: "blob:factory-roundtrip-preview",
          revokePreviewImageSrc: vi.fn(),
          schemaVersion: PORT_OS_FACTORY_PNG_SCHEMA_VERSION,
        }),
      );
    });

    await waitFor(() => {
      expect(result.current.activationState.status).toBe("error");
    });
    if (result.current.activationState.status !== "error") {
      throw new Error("expected activation failure state");
    }
    expect(result.current.activationState.error).toMatchObject({
      code: "INTERNAL_ERROR",
      message: "Factory activation failed.",
    });
  });

  it("forwards create-new-named choices into session-scoped activation", async () => {
    mockedActivateImportedFactoryForSession.mockResolvedValue(canonicalFactory);
    const importInput = createFactoryImportConfirmInput(
      {
        factory: canonicalFactory,
        previewImageSrc: "blob:factory-roundtrip-preview",
        revokePreviewImageSrc: vi.fn(),
        schemaVersion: PORT_OS_FACTORY_PNG_SCHEMA_VERSION,
      },
      {
        choice: "create_new_named",
        createFactoryName: "Factory Roundtrip",
      },
    );

    const { result } = renderHook(
      () => useFactoryImportActivation({ sessionID: "session-2" }),
      { wrapper: createQueryClientWrapper() },
    );

    await act(async () => {
      await result.current.activateImport(importInput);
    });

    await waitFor(() => {
      expect(mockedActivateImportedFactoryForSession).toHaveBeenCalledWith(
        canonicalFactory,
        {
          choice: "create_new_named",
          createFactoryName: "Factory Roundtrip",
          existingFactoryNames: importInput.existingFactoryNames,
          sessionID: "session-2",
        },
      );
    });
  });
});

function createQueryClientWrapper(): ({ children }: { children: ReactNode }) => ReactNode {
  const queryClient = new QueryClient({
    defaultOptions: {
      mutations: {
        retry: false,
      },
      queries: {
        gcTime: Infinity,
        retry: false,
      },
    },
  });

  return function QueryClientWrapper({ children }: { children: ReactNode }): ReactNode {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
  };
}

function fromBase64(value: string): Uint8Array {
  return Uint8Array.from(atob(value), (character) => character.charCodeAt(0));
}

function toArrayBuffer(bytes: Uint8Array): ArrayBuffer {
  const copy = new Uint8Array(bytes.byteLength);
  copy.set(bytes);
  return copy.buffer;
}
