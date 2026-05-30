import { useCallback } from "react";

import type { FactoryImportSaveChoice, FactoryValue } from "../../../api/named-factory";
import type { FactoryPngImportValue } from "../../import/lib/factory-png-import";
import {
  type FactoryImportActivationState,
  useFactoryImportActivation,
} from "../../import/hooks/use-factory-import-activation";
import {
  type FactoryImportPreviewState,
  useFactoryImportPreview,
} from "../../import/hooks/use-factory-import-preview";
import {
  type FactoryPngDropState,
  type ReadFactoryImportFile,
  useFactoryPngDrop,
} from "../../import/hooks/use-factory-png-drop";

export interface CurrentActivityImportController {
  activateImport: (
    value: FactoryPngImportValue,
    choice?: FactoryImportSaveChoice,
  ) => Promise<void>;
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
  locale?: string | null;
  onFactoryActivated?: () => void;
  onFactoryImportReady?: (value: FactoryPngImportValue, file: File) => void;
  readFactoryImportFile?: ReadFactoryImportFile;
  sessionID?: string | null;
}

export function useCurrentActivityImportController({
  activateFactory,
  locale,
  onFactoryActivated,
  onFactoryImportReady,
  readFactoryImportFile,
  sessionID,
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
    sessionID,
  });
  const handleImportPreviewReady = useCallback((value: FactoryPngImportValue, file: File) => {
    clearActivationError();
    openPreview(value, file);
  }, [clearActivationError, openPreview]);
  const drop = useFactoryPngDrop({
    locale,
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
