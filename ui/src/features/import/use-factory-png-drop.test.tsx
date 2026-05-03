import { act, renderHook, waitFor } from "@testing-library/react";

import type { FactoryPngImportValue, ReadFactoryImportPngResult } from "./factory-png-import";
import { type ReadFactoryImportFile, type UseFactoryPngDropResult, useFactoryPngDrop } from "./use-factory-png-drop";

describe("useFactoryPngDrop", () => {
  it("revokes successful results that resolve after a newer drop supersedes them", async () => {
    const firstFile = new File(["first"], "first.png", { type: "image/png" });
    const secondFile = new File(["second"], "second.png", { type: "image/png" });
    const firstPending = createDeferred<ReadFactoryImportPngResult>();
    const secondPending = createDeferred<ReadFactoryImportPngResult>();
    const firstImport = createFactoryImportValue("blob:first-preview");
    const secondImport = createFactoryImportValue("blob:second-preview");
    const onImportReady = vi.fn<(value: FactoryPngImportValue, file: File) => void>();
    const readFactoryImportFile = vi.fn<ReadFactoryImportFile>()
      .mockReturnValueOnce(firstPending.promise)
      .mockReturnValueOnce(secondPending.promise);
    const { result } = renderHook(() =>
      useFactoryPngDrop({ onImportReady, readFactoryImportFile }),
    );

    act(() => {
      void result.current.onDrop(createFileDropEvent(firstFile));
    });
    act(() => {
      void result.current.onDrop(createFileDropEvent(secondFile));
    });

    await waitFor(() => {
      expect(readFactoryImportFile).toHaveBeenNthCalledWith(1, firstFile);
      expect(readFactoryImportFile).toHaveBeenNthCalledWith(2, secondFile);
    });

    await act(async () => {
      firstPending.resolve({ ok: true, value: firstImport });
      await firstPending.promise;
    });

    expect(firstImport.revokePreviewImageSrc).toHaveBeenCalledTimes(1);
    expect(onImportReady).not.toHaveBeenCalled();

    await act(async () => {
      secondPending.resolve({ ok: true, value: secondImport });
      await secondPending.promise;
    });

    expect(secondImport.revokePreviewImageSrc).not.toHaveBeenCalled();
    expect(onImportReady).toHaveBeenCalledWith(secondImport, secondFile);
  });

  it("revokes successful results that resolve after unmount", async () => {
    const file = new File(["png"], "factory-import.png", { type: "image/png" });
    const pending = createDeferred<ReadFactoryImportPngResult>();
    const importValue = createFactoryImportValue();
    const onImportReady = vi.fn<(value: FactoryPngImportValue, file: File) => void>();
    const readFactoryImportFile = vi.fn<ReadFactoryImportFile>().mockReturnValue(pending.promise);
    const { result, unmount } = renderHook(() =>
      useFactoryPngDrop({ onImportReady, readFactoryImportFile }),
    );

    act(() => {
      void result.current.onDrop(createFileDropEvent(file));
    });

    await waitFor(() => {
      expect(readFactoryImportFile).toHaveBeenCalledWith(file);
    });

    unmount();

    await act(async () => {
      pending.resolve({ ok: true, value: importValue });
      await pending.promise;
    });

    expect(importValue.revokePreviewImageSrc).toHaveBeenCalledTimes(1);
    expect(onImportReady).not.toHaveBeenCalled();
  });
});

function createFactoryImportValue(previewImageSrc = "blob:factory-preview"): FactoryPngImportValue {
  return {
    factory: {
      name: "Dropped Factory",
      workTypes: [],
      workers: [],
      workstations: [],
    },
    previewImageSrc,
    revokePreviewImageSrc: vi.fn(),
    schemaVersion: "portos.agent-factory.png.v1",
  };
}

function createFileDropEvent(file: File): Parameters<UseFactoryPngDropResult["onDrop"]>[0] {
  return {
    dataTransfer: {
      dropEffect: "none",
      files: [file],
      types: ["Files"],
    },
    preventDefault: vi.fn(),
  } as unknown as Parameters<UseFactoryPngDropResult["onDrop"]>[0];
}

function createDeferred<T>() {
  let resolve: (value: T | PromiseLike<T>) => void = () => {};
  let reject: (reason?: unknown) => void = () => {};
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });

  return { promise, reject, resolve };
}

