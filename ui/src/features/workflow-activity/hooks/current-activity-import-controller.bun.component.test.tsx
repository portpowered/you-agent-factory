import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, mock } from "bun:test";
import type { ReactNode } from "react";

import type { ImportFactoryValue } from "../../../api/session-factory";
import type {
  ReadFactoryImportFile,
  UseFactoryPngDropResult,
} from "../../import/hooks/use-factory-png-drop";
import { createFactoryImportConfirmInput } from "../../import/lib/factory-import-confirm-input.test-helpers";
import type { FactoryPngImportValue } from "../../import/lib/factory-png-import";
import { useCurrentActivityImportController } from "./current-activity-import-controller";

describe("useCurrentActivityImportController", () => {
  it("opens the feature-owned preview when a dropped factory is ready", async () => {
    const importValue = createFactoryImportValue();
    const file = createFactoryPngFile();
    const onFactoryImportReady = mock(() => {});
    const readFactoryImportFile: ReadFactoryImportFile = mock(async () => ({
      ok: true,
      value: importValue,
    }));
    const { result } = renderHook(
      () =>
        useCurrentActivityImportController({
          onFactoryImportReady,
          readFactoryImportFile,
        }),
      { wrapper: createQueryClientWrapper() },
    );

    await act(async () => {
      await result.current.onDrop(createFileDropEvent(file));
    });

    expect(readFactoryImportFile).toHaveBeenCalledWith(file);
    expect(onFactoryImportReady).toHaveBeenCalledWith(importValue, file);
    expect(result.current.importPreviewState).toEqual({
      file,
      status: "ready",
      value: importValue,
    });
  });

  it("closes the preview and requests a refresh after successful activation", async () => {
    const importValue = createFactoryImportValue();
    const onFactoryActivated = mock(() => {});
    const activateFactory = mock(
      async (): Promise<ImportFactoryValue> => importValue.factory,
    );
    const { result } = renderHook(
      () =>
        useCurrentActivityImportController({
          activateFactory,
          onFactoryActivated,
          readFactoryImportFile: async () => ({ ok: true, value: importValue }),
        }),
      { wrapper: createQueryClientWrapper() },
    );

    await act(async () => {
      await result.current.onDrop(createFileDropEvent(createFactoryPngFile()));
      await result.current.activateImport(
        createFactoryImportConfirmInput(importValue),
      );
    });

    await waitFor(() => {
      expect(result.current.importPreviewState).toEqual({ status: "idle" });
    });
    expect(activateFactory).toHaveBeenCalledWith(
      createFactoryImportConfirmInput(importValue),
    );
    expect(onFactoryActivated).toHaveBeenCalledTimes(1);
    expect(importValue.revokePreviewImageSrc).toHaveBeenCalledTimes(1);
  });

  it("keeps the preview open and does not refresh after failed activation", async () => {
    const importValue = createFactoryImportValue();
    const onFactoryActivated = mock(() => {});
    const { result } = renderHook(
      () =>
        useCurrentActivityImportController({
          activateFactory: async () => {
            throw new Error("activation failed");
          },
          onFactoryActivated,
          readFactoryImportFile: async () => ({ ok: true, value: importValue }),
        }),
      { wrapper: createQueryClientWrapper() },
    );

    await act(async () => {
      await result.current.onDrop(createFileDropEvent(createFactoryPngFile()));
      await result.current.activateImport(
        createFactoryImportConfirmInput(importValue),
      );
    });

    expect(result.current.importPreviewState.status).toBe("ready");
    expect(result.current.activationState.status).toBe("error");
    expect(onFactoryActivated).not.toHaveBeenCalled();
    expect(importValue.revokePreviewImageSrc).not.toHaveBeenCalled();
  });
});

function createFactoryImportValue(): FactoryPngImportValue {
  return {
    factory: {
      name: "Dropped Factory",
      workTypes: [],
      workers: [],
      workstations: [],
    },
    previewImageSrc: "blob:factory-preview",
    revokePreviewImageSrc: mock(() => {}),
    schemaVersion: "portos.agent-factory.png.v1",
  };
}

function createFactoryPngFile(): File {
  return new File(["png"], "factory-import.png", { type: "image/png" });
}

function createFileDropEvent(
  file: File,
): Parameters<UseFactoryPngDropResult["onDrop"]>[0] {
  return {
    dataTransfer: {
      dropEffect: "none",
      files: [file],
      types: ["Files"],
    },
    preventDefault: mock(() => {}),
  } as unknown as Parameters<UseFactoryPngDropResult["onDrop"]>[0];
}

function createQueryClientWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      mutations: { retry: false },
      queries: { gcTime: Infinity, retry: false },
    },
  });

  return function QueryClientWrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}
