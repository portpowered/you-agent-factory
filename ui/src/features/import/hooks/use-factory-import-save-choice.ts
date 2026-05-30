import { type Dispatch, type SetStateAction, useEffect, useState } from "react";

import type { FactoryImportSaveChoice } from "../../../api/named-factory";
import type { FactoryImportPreviewState } from "./use-factory-import-preview";

export type ReadyFactoryImportPreview = Extract<
  FactoryImportPreviewState,
  { status: "ready" }
>;

export function useFactoryImportSaveChoice(
  readyImportPreview: ReadyFactoryImportPreview | null,
): [
  FactoryImportSaveChoice,
  Dispatch<SetStateAction<FactoryImportSaveChoice>>,
] {
  const [importSaveChoice, setImportSaveChoice] =
    useState<FactoryImportSaveChoice>("REPLACE_CURRENT");

  useEffect(() => {
    if (readyImportPreview) {
      setImportSaveChoice("REPLACE_CURRENT");
    }
  }, [readyImportPreview]);

  return [importSaveChoice, setImportSaveChoice];
}
