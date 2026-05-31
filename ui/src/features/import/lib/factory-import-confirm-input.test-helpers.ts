import type { FactoryImportConfirmInput } from "./factory-import-save-choice";
import type { FactoryPngImportValue } from "./factory-png-import";

export function createFactoryImportConfirmInput(
  value: FactoryPngImportValue,
  overrides: Partial<Omit<FactoryImportConfirmInput, "value">> = {},
): FactoryImportConfirmInput {
  const embeddedName = value.factory.name?.trim() ?? "";

  return {
    choice: "replace_current",
    createFactoryName: embeddedName.length > 0 ? `${embeddedName}-2` : embeddedName,
    existingFactoryNames: ["alpha", embeddedName],
    value,
    ...overrides,
  };
}
