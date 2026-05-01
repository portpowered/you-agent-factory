import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";

import type { FactoryValue } from "../../api/named-factory";
import { writeFactoryExportPng } from "../export/factory-png-export";
import { PORT_OS_FACTORY_PNG_SCHEMA_VERSION, readFactoryImportPng } from "./factory-png-import";
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
      onFailure: { state: "done", workType: "story" },
      outputs: [{ state: "done", workType: "story" }],
      worker: "writer",
    },
  ],
};

describe("useFactoryImportActivation", () => {
  it("activates the direct factory payload while preserving the PNG factory metadata", async () => {
    const activateFactory = vi.fn<(value: FactoryValue) => Promise<FactoryValue>>()
      .mockImplementation(async (value) => value);
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
      await result.current.activateImport(importResult.value);
    });

    await waitFor(() => {
      expect(activateFactory).toHaveBeenCalledWith(canonicalFactory);
    });
    expect(activateFactory).toHaveBeenCalledTimes(1);
    expect(onActivated).toHaveBeenCalledWith(canonicalFactory);
    expect(importResult.value.factory).toEqual(canonicalFactory);
    expect(importResult.value.schemaVersion).toBe(PORT_OS_FACTORY_PNG_SCHEMA_VERSION);
    expect(result.current.activationState).toEqual({ status: "idle" });
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
