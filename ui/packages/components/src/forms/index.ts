/** Stable category path for `@you-agent-factory/components/forms`. */
export const COMPONENTS_CATEGORY = "forms" as const;

export type ComponentsCategory = typeof COMPONENTS_CATEGORY;

export { PackageCheckbox } from "./package-checkbox";
export type { PackageCheckboxProps } from "./package-checkbox";
export { PackageFileInput } from "./package-file-input";
export type { PackageFileInputProps } from "./package-file-input";
export { PackageInput, inputVariants } from "./package-input";
export type { PackageInputProps } from "./package-input";
export { PackageTextarea, textareaVariants } from "./package-textarea";
export type { PackageTextareaProps } from "./package-textarea";
