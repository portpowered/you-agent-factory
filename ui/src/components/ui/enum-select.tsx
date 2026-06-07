import { type ComponentProps, useState } from "react";

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "./select";

export const ENUM_SELECT_EMPTY_VALUE = "__dashboard-enum-select-empty__";

export interface EnumSelectOption {
  disabled?: boolean;
  label: string;
  value: string;
}

type EnumSelectAriaProps = Pick<
  ComponentProps<typeof SelectTrigger>,
  "aria-describedby" | "aria-invalid" | "aria-label" | "aria-labelledby"
>;

export interface EnumSelectProps extends EnumSelectAriaProps {
  disabled?: boolean;
  id: string;
  onValueChange: (value: string) => void;
  options: readonly EnumSelectOption[];
  placeholder?: string;
  value: string;
}

export function EnumSelect({
  disabled,
  id,
  onValueChange,
  options,
  placeholder,
  value,
  ...aria
}: EnumSelectProps) {
  return (
    <Select disabled={disabled} onValueChange={onValueChange} value={value}>
      <SelectTrigger id={id} {...aria}>
        <SelectValue placeholder={placeholder} />
      </SelectTrigger>
      <SelectContent>
        {options.map((option) => (
          <SelectItem
            disabled={option.disabled}
            key={option.value}
            value={option.value}
          >
            {option.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

export interface OptionalEnumSelectProps extends EnumSelectAriaProps {
  disabled?: boolean;
  emptyOptionLabel: string;
  id: string;
  onValueChange: (value: string | null) => void;
  options: readonly EnumSelectOption[];
  value: string | null | undefined;
}

export function OptionalEnumSelect({
  disabled,
  emptyOptionLabel,
  id,
  onValueChange,
  options,
  value,
  ...aria
}: OptionalEnumSelectProps) {
  return (
    <Select
      disabled={disabled}
      onValueChange={(nextValue) =>
        onValueChange(nextValue === ENUM_SELECT_EMPTY_VALUE ? null : nextValue)
      }
      value={value ?? ENUM_SELECT_EMPTY_VALUE}
    >
      <SelectTrigger id={id} {...aria}>
        <SelectValue placeholder={emptyOptionLabel} />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value={ENUM_SELECT_EMPTY_VALUE}>
          {emptyOptionLabel}
        </SelectItem>
        {options.map((option) => (
          <SelectItem
            disabled={option.disabled}
            key={option.value}
            value={option.value}
          >
            {option.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

export interface ResetEnumSelectProps extends EnumSelectAriaProps {
  disabled?: boolean;
  id: string;
  onValueChange: (value: string) => void;
  options: readonly EnumSelectOption[];
  placeholder: string;
}

export function ResetEnumSelect({
  disabled,
  id,
  onValueChange,
  options,
  placeholder,
  ...aria
}: ResetEnumSelectProps) {
  const [instanceKey, setInstanceKey] = useState(0);

  return (
    <Select
      key={instanceKey}
      disabled={disabled}
      onValueChange={(nextValue) => {
        onValueChange(nextValue);
        setInstanceKey((current) => current + 1);
      }}
      value={undefined}
    >
      <SelectTrigger id={id} {...aria}>
        <SelectValue placeholder={placeholder} />
      </SelectTrigger>
      <SelectContent>
        {options.map((option) => (
          <SelectItem
            disabled={option.disabled}
            key={option.value}
            value={option.value}
          >
            {option.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
