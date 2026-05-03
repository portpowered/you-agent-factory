import { useCallback, useEffect, useRef, useState } from "react";

import type { FactoryPngImportValue } from "./factory-png-import";

export type FactoryImportPreviewState =
  | { status: "idle" }
  | { file: File; status: "ready"; value: FactoryPngImportValue };

export interface UseFactoryImportPreviewOptions {
  onPreviewReady?: (value: FactoryPngImportValue, file: File) => void;
}

export interface UseFactoryImportPreviewResult {
  closePreview: () => void;
  openPreview: (value: FactoryPngImportValue, file: File) => void;
  previewState: FactoryImportPreviewState;
}

const IDLE_PREVIEW_STATE: FactoryImportPreviewState = { status: "idle" };

export function useFactoryImportPreview({
  onPreviewReady,
}: UseFactoryImportPreviewOptions = {}): UseFactoryImportPreviewResult {
  const [previewState, setPreviewState] = useState<FactoryImportPreviewState>(IDLE_PREVIEW_STATE);
  const activePreviewRef = useRef<Extract<FactoryImportPreviewState, { status: "ready" }> | null>(
    null,
  );

  const revokeActivePreview = useCallback(() => {
    const activePreview = activePreviewRef.current;
    if (!activePreview) {
      return;
    }

    activePreview.value.revokePreviewImageSrc();
    activePreviewRef.current = null;
  }, []);

  const closePreview = useCallback(() => {
    revokeActivePreview();
    setPreviewState(IDLE_PREVIEW_STATE);
  }, [revokeActivePreview]);

  const openPreview = useCallback((value: FactoryPngImportValue, file: File) => {
    revokeActivePreview();

    const nextPreviewState = { file, status: "ready", value } as const;
    activePreviewRef.current = nextPreviewState;
    setPreviewState(nextPreviewState);
    onPreviewReady?.(value, file);
  }, [onPreviewReady, revokeActivePreview]);

  useEffect(() => () => {
    revokeActivePreview();
  }, [revokeActivePreview]);

  return {
    closePreview,
    openPreview,
    previewState,
  };
}

