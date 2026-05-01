import type { DragEventHandler } from "react";
import { useCallback, useEffect, useRef, useState } from "react";

import {
  type FactoryPngImportValue,
  type ReadFactoryImportPngError,
  type ReadFactoryImportPngResult,
  readFactoryImportPng,
} from "./factory-png-import";

const FILE_DRAG_DATA_TYPE = "Files";

export type ReadFactoryImportFile = (file: File) => Promise<ReadFactoryImportPngResult>;

export type FactoryPngDropState =
  | { status: "idle" }
  | { status: "drag-active" }
  | { fileName: string; status: "reading" }
  | { error: ReadFactoryImportPngError; fileName: string; status: "error" };

export interface UseFactoryPngDropOptions {
  onImportReady?: (value: FactoryPngImportValue, file: File) => void;
  readFactoryImportFile?: ReadFactoryImportFile;
}

export interface UseFactoryPngDropResult {
  clearError: () => void;
  dropState: FactoryPngDropState;
  onDragEnter: DragEventHandler<HTMLElement>;
  onDragLeave: DragEventHandler<HTMLElement>;
  onDragOver: DragEventHandler<HTMLElement>;
  onDrop: DragEventHandler<HTMLElement>;
}

const IDLE_DROP_STATE: FactoryPngDropState = { status: "idle" };

export function useFactoryPngDrop({
  onImportReady,
  readFactoryImportFile = defaultReadFactoryImportFile,
}: UseFactoryPngDropOptions = {}): UseFactoryPngDropResult {
  const [dropState, setDropState] = useState<FactoryPngDropState>(IDLE_DROP_STATE);
  const dragDepthRef = useRef(0);
  const requestIDRef = useRef(0);
  const invalidatePendingDrop = useCallback(() => {
    dragDepthRef.current = 0;
    requestIDRef.current += 1;
  }, []);

  useEffect(() => () => {
    invalidatePendingDrop();
  }, [invalidatePendingDrop]);

  const clearError = useCallback(() => {
    setDropState((currentState) => {
      if (currentState.status !== "error") {
        return currentState;
      }

      return IDLE_DROP_STATE;
    });
  }, []);

  const onDragEnter = useCallback<DragEventHandler<HTMLElement>>((event) => {
    if (!isFileDragEvent(event)) {
      return;
    }

    event.preventDefault();
    dragDepthRef.current += 1;
    setDropState((currentState) => {
      if (currentState.status === "reading") {
        return currentState;
      }

      return { status: "drag-active" };
    });
  }, []);

  const onDragLeave = useCallback<DragEventHandler<HTMLElement>>((event) => {
    if (!isFileDragEvent(event)) {
      return;
    }

    event.preventDefault();
    dragDepthRef.current = Math.max(0, dragDepthRef.current - 1);
    if (dragDepthRef.current > 0) {
      return;
    }

    setDropState((currentState) => {
      if (currentState.status !== "drag-active") {
        return currentState;
      }

      return IDLE_DROP_STATE;
    });
  }, []);

  const onDragOver = useCallback<DragEventHandler<HTMLElement>>((event) => {
    if (!isFileDragEvent(event)) {
      return;
    }

    event.preventDefault();
    event.dataTransfer.dropEffect = "copy";
    setDropState((currentState) => {
      if (currentState.status === "reading") {
        return currentState;
      }

      return { status: "drag-active" };
    });
  }, []);

  const onDrop = useCallback<DragEventHandler<HTMLElement>>(async (event) => {
    if (!isFileDragEvent(event)) {
      return;
    }

    event.preventDefault();
    dragDepthRef.current = 0;

    const file = fileFromDragEvent(event);
    if (!file) {
      setDropState(IDLE_DROP_STATE);
      return;
    }

    const requestID = requestIDRef.current + 1;
    requestIDRef.current = requestID;
    setDropState({ fileName: file.name, status: "reading" });

    const result = await readFactoryImportFile(file);
    if (requestIDRef.current !== requestID) {
      if (result.ok) {
        result.value.revokePreviewImageSrc();
      }
      return;
    }

    if (!result.ok) {
      setDropState({ error: result.error, fileName: file.name, status: "error" });
      return;
    }

    setDropState(IDLE_DROP_STATE);
    onImportReady?.(result.value, file);
  }, [onImportReady, readFactoryImportFile]);

  return {
    clearError,
    dropState,
    onDragEnter,
    onDragLeave,
    onDragOver,
    onDrop,
  };
}

async function defaultReadFactoryImportFile(file: File): Promise<ReadFactoryImportPngResult> {
  return readFactoryImportPng({ file });
}

function isFileDragEvent(event: Pick<DragEvent, "dataTransfer">): boolean {
  const dragTypes = event.dataTransfer?.types;
  if (!dragTypes) {
    return false;
  }

  return Array.from(dragTypes).includes(FILE_DRAG_DATA_TYPE);
}

function fileFromDragEvent(event: Pick<DragEvent, "dataTransfer">): File | null {
  const files = event.dataTransfer?.files;
  if (!files || files.length === 0) {
    return null;
  }

  const firstFile = files[0];
  return firstFile instanceof File ? firstFile : null;
}
