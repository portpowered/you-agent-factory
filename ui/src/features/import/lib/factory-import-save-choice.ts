import type { FactoryImportSaveChoice } from "../../../api/session-factory";
import type { FactoryPngImportValue } from "./factory-png-import";

export type { FactoryImportSaveChoice };

export interface FactoryImportConfirmInput {
  choice: FactoryImportSaveChoice;
  createFactoryName: string;
  existingFactoryNames: readonly string[];
  value: FactoryPngImportValue;
}
