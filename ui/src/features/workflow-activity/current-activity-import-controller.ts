import { useCallback } from "react";

import type { FactoryValue } from "../../api/named-factory";
import {
  type FactoryImportActivationState,
  type FactoryImportPreviewState,
  type FactoryPngDropState,
  type FactoryPngImportValue,
  type ReadFactoryImportFile,
  useFactoryImportActivation,
  useFactoryImportPreview,
  useFactoryPngDrop,
} from "../import";

export interface CurrentActivityImportController {
  activateImport: (value: FactoryPngImportValue) => Promise<void>;
  activationState: FactoryImportActivationState;
  clearActivationError: () => void;
  clearError: () => void;
  closeImportPreview: () => void;
  dropState: FactoryPngDropState;
  importPreviewState: FactoryImportPreviewState;
  onDragEnter: ReturnType<typeof useFactoryPngDrop>["onDragEnter"];
  onDragLeave: ReturnType<typeof useFactoryPngDrop>["onDragLeave"];
  onDragOver: ReturnType<typeof useFactoryPngDrop>["onDragOver"];
  onDrop: ReturnType<typeof useFactoryPngDrop>["onDrop"];
}

export interface UseCurrentActivityImportControllerOptions {
  activateFactory?: (value: FactoryValue) => Promise<FactoryValue>;
  onFactoryActivated?: () => void;
  onFactoryImportReady?: (value: FactoryPngImportValue, file: File) => void;
  readFactoryImportFile?: ReadFactoryImportFile;
}

export function useCurrentActivityImportController({
  activateFactory,
  onFactoryActivated,
  onFactoryImportReady,
  readFactoryImportFile,
}: UseCurrentActivityImportControllerOptions = {}): CurrentActivityImportController {
  const {
    closePreview: closeImportPreview,
    openPreview,
    previewState: importPreviewState,
  } = useFactoryImportPreview({
    onPreviewReady: onFactoryImportReady,
  });
  const handleFactoryActivated = useCallback(() => {
    closeImportPreview();
    onFactoryActivated?.();
  }, [closeImportPreview, onFactoryActivated]);
  const {
    activateImport,
    activationState,
    clearActivationError,
  } = useFactoryImportActivation({
    activateFactory,
    onActivated: handleFactoryActivated,
  });
  const handleImportPreviewReady = useCallback((value: FactoryPngImportValue, file: File) => {
    clearActivationError();
    openPreview(value, file);
  }, [clearActivationError, openPreview]);
  const drop = useFactoryPngDrop({
    onImportReady: handleImportPreviewReady,
    readFactoryImportFile,
  });

  return {
    activateImport,
    activationState,
    clearActivationError,
    clearError: drop.clearError,
    closeImportPreview,
    dropState: drop.dropState,
    importPreviewState,
    onDragEnter: drop.onDragEnter,
    onDragLeave: drop.onDragLeave,
    onDragOver: drop.onDragOver,
    onDrop: drop.onDrop,
  };
}
