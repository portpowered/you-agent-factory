/** Stable category path for `@you-agent-factory/components/forms`. */
export const COMPONENTS_CATEGORY = "forms" as const;

export type ComponentsCategory = typeof COMPONENTS_CATEGORY;

export type { PackageCheckboxProps } from "./package-checkbox";
export { PackageCheckbox } from "./package-checkbox";
export type {
  EnumSelectOption,
  EnumSelectProps,
  OptionalEnumSelectProps,
  ResetEnumSelectProps,
} from "./package-enum-select";
export {
  ENUM_SELECT_EMPTY_VALUE,
  EnumSelect,
  OptionalEnumSelect,
  ResetEnumSelect,
} from "./package-enum-select";
export type { PackageFileInputProps } from "./package-file-input";
export { PackageFileInput } from "./package-file-input";
export type {
  FormDescriptionProps,
  FormErrorProps,
  FormFieldGroupLabelProps,
  FormFieldGroupProps,
  FormFieldMessageIds,
  FormFieldProps,
  FormHelperTextProps,
  FormLabelProps,
  FormSuccessProps,
  FormWarningProps,
} from "./package-form-field";
export {
  buildFormFieldAriaDescribedBy,
  FormDescription,
  FormError,
  FormField,
  FormFieldGroup,
  FormFieldGroupLabel,
  FormHelperText,
  FormLabel,
  FormSuccess,
  FormWarning,
} from "./package-form-field";
export type { PackageInputProps } from "./package-input";
export { inputVariants, PackageInput } from "./package-input";
export type { NativeSelectProps } from "./package-native-select";
export { NativeSelect } from "./package-native-select";
export type {
  SelectContentProps,
  SelectEmptyProps,
  SelectFieldProps,
  SelectItemProps,
  SelectLabelProps,
  SelectSeparatorProps,
  SelectTriggerProps,
} from "./package-select";
export {
  SELECT_EMPTY_STATE_VALUE,
  Select,
  SelectContent,
  SelectEmpty,
  SelectField,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectSeparator,
  SelectTrigger,
  SelectValue,
} from "./package-select";
export type { PackageTextareaProps } from "./package-textarea";
export { PackageTextarea, textareaVariants } from "./package-textarea";
