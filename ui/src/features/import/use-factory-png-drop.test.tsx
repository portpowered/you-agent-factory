import { act, renderHook, waitFor } from "@testing-library/react";

import type { FactoryPngImportValue, ReadFactoryImportPngResult } from "./factory-png-import";
import { type ReadFactoryImportFile, type UseFactoryPngDropResult, useFactoryPngDrop } from "./use-factory-png-drop";

describe("useFactoryPngDrop", () => {
  it("ignores non-file drag events and leaves the drop state idle", () => {
    const onImportReady = vi.fn<(value: FactoryPngImportValue, file: File) => void>();
    const readFactoryImportFile = vi.fn<ReadFactoryImportFile>();
    const { result } = renderHook(() =>
      useFactoryPngDrop({ onImportReady, readFactoryImportFile }),
    );
    const preventDefault = vi.fn();
    const nonFileEvent = {
      dataTransfer: {
        dropEffect: "none",
        files: [],
        types: ["text/plain"],
      },
      preventDefault,
    } as unknown as Parameters<UseFactoryPngDropResult["onDragEnter"]>[0];

    act(() => {
      result.current.onDragEnter(nonFileEvent);
      result.current.onDragOver(nonFileEvent);
      result.current.onDragLeave(nonFileEvent);
      void result.current.onDrop(nonFileEvent);
      result.current.clearError();
    });

    expect(preventDefault).not.toHaveBeenCalled();
    expect(readFactoryImportFile).not.toHaveBeenCalled();
    expect(result.current.dropState).toEqual({ status: "idle" });
  });

  it("keeps the drag-active state until the nested file drag fully leaves", () => {
    const { result } = renderHook(() => useFactoryPngDrop());
    const file = new File(["png"], "factory-import.png", { type: "image/png" });
    const enterEvent = createFileDragEvent(file);
    const leaveEvent = createFileDragEvent(file);

    act(() => {
      result.current.onDragEnter(enterEvent);
      result.current.onDragEnter(enterEvent);
    });

    expect(result.current.dropState).toEqual({ status: "drag-active" });

    act(() => {
      result.current.onDragLeave(leaveEvent);
    });

    expect(result.current.dropState).toEqual({ status: "drag-active" });

    act(() => {
      result.current.onDragLeave(leaveEvent);
    });

    expect(result.current.dropState).toEqual({ status: "idle" });
  });

  it("preserves the reading state during drag enter, over, and leave transitions", async () => {
    const file = new File(["png"], "factory-import.png", { type: "image/png" });
    const pending = createDeferred<ReadFactoryImportPngResult>();
    const readFactoryImportFile = vi.fn<ReadFactoryImportFile>().mockReturnValue(pending.promise);
    const { result } = renderHook(() => useFactoryPngDrop({ readFactoryImportFile }));
    const dragEvent = createFileDragEvent(file);

    act(() => {
      void result.current.onDrop(createFileDropEvent(file));
    });

    await waitFor(() => {
      expect(result.current.dropState).toEqual({
        fileName: "factory-import.png",
        status: "reading",
      });
    });

    act(() => {
      result.current.onDragEnter(dragEvent);
      result.current.onDragOver(dragEvent);
      result.current.onDragLeave(dragEvent);
    });

    expect(result.current.dropState).toEqual({
      fileName: "factory-import.png",
      status: "reading",
    });

    await act(async () => {
      pending.resolve({ ok: true, value: createFactoryImportValue() });
      await pending.promise;
    });
  });

  it("returns to idle and surfaces read errors when the dropped file cannot be imported", async () => {
    const file = new File(["png"], "factory-import.png", { type: "image/png" });
    const onImportReady = vi.fn<(value: FactoryPngImportValue, file: File) => void>();
    const readFactoryImportFile = vi.fn<ReadFactoryImportFile>().mockResolvedValue({
      error: {
        code: "PNG_METADATA_INVALID",
        message: "The Port OS factory metadata is not valid JSON.",
      },
      ok: false,
    });
    const { result } = renderHook(() =>
      useFactoryPngDrop({ onImportReady, readFactoryImportFile }),
    );

    await act(async () => {
      await result.current.onDrop(createFileDropEvent(file));
    });

    expect(result.current.dropState).toEqual({
      error: {
        code: "PNG_METADATA_INVALID",
        message: "The Port OS factory metadata is not valid JSON.",
      },
      fileName: "factory-import.png",
      status: "error",
    });
    expect(onImportReady).not.toHaveBeenCalled();

    act(() => {
      result.current.clearError();
    });

    expect(result.current.dropState).toEqual({ status: "idle" });
  });

  it("returns to idle when a file drag drops without a file payload", async () => {
    const { result } = renderHook(() => useFactoryPngDrop());

    await act(async () => {
      await result.current.onDrop({
        dataTransfer: {
          dropEffect: "none",
          files: [],
          types: ["Files"],
        },
        preventDefault: vi.fn(),
      } as unknown as Parameters<UseFactoryPngDropResult["onDrop"]>[0]);
    });

    expect(result.current.dropState).toEqual({ status: "idle" });
  });

  it("returns to idle when a file drag payload does not expose a File instance", async () => {
    const { result } = renderHook(() => useFactoryPngDrop());

    await act(async () => {
      await result.current.onDrop({
        dataTransfer: {
          dropEffect: "none",
          files: ["factory-import.png"],
          types: ["Files"],
        },
        preventDefault: vi.fn(),
      } as unknown as Parameters<UseFactoryPngDropResult["onDrop"]>[0]);
    });

    expect(result.current.dropState).toEqual({ status: "idle" });
  });

  it("ignores drag events when the browser does not expose drag types", () => {
    const { result } = renderHook(() => useFactoryPngDrop());
    const preventDefault = vi.fn();
    const dragEventWithoutTypes = {
      dataTransfer: {},
      preventDefault,
    } as unknown as Parameters<UseFactoryPngDropResult["onDragOver"]>[0];

    act(() => {
      result.current.onDragEnter(dragEventWithoutTypes);
      result.current.onDragOver(dragEventWithoutTypes);
      result.current.onDragLeave(dragEventWithoutTypes);
    });

    expect(preventDefault).not.toHaveBeenCalled();
    expect(result.current.dropState).toEqual({ status: "idle" });
  });

  it("keeps superseded failed imports from surfacing a stale drop error", async () => {
    const firstFile = new File(["first"], "first.png", { type: "image/png" });
    const secondFile = new File(["second"], "second.png", { type: "image/png" });
    const firstPending = createDeferred<ReadFactoryImportPngResult>();
    const secondPending = createDeferred<ReadFactoryImportPngResult>();
    const readFactoryImportFile = vi.fn<ReadFactoryImportFile>()
      .mockReturnValueOnce(firstPending.promise)
      .mockReturnValueOnce(secondPending.promise);
    const { result } = renderHook(() => useFactoryPngDrop({ readFactoryImportFile }));

    act(() => {
      void result.current.onDrop(createFileDropEvent(firstFile));
    });
    act(() => {
      void result.current.onDrop(createFileDropEvent(secondFile));
    });

    await act(async () => {
      firstPending.resolve({
        error: {
          code: "PNG_METADATA_INVALID",
          message: "The Port OS factory metadata is not valid JSON.",
        },
        ok: false,
      });
      await firstPending.promise;
    });

    expect(result.current.dropState).toEqual({
      fileName: "second.png",
      status: "reading",
    });

    await act(async () => {
      secondPending.resolve({ ok: true, value: createFactoryImportValue("blob:second-preview") });
      await secondPending.promise;
    });

    expect(result.current.dropState).toEqual({ status: "idle" });
  });

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

function createFileDragEvent(file: File): Parameters<UseFactoryPngDropResult["onDragEnter"]>[0] {
  return {
    dataTransfer: {
      dropEffect: "none",
      files: [file],
      types: ["Files"],
    },
    preventDefault: vi.fn(),
  } as unknown as Parameters<UseFactoryPngDropResult["onDragEnter"]>[0];
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
