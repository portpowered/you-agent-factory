import type { FactoryPngImportValue } from "./factory-png-import";

export type FactoryImportSaveChoice = "replace_current" | "create_new_named";

export interface FactoryImportConfirmInput {
  choice: FactoryImportSaveChoice;
  createFactoryName: string;
  value: FactoryPngImportValue;
}
