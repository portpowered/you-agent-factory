import { type ComponentProps, useState } from "react";

import {
  SELECT_EMPTY_STATE_VALUE,
  Select,
  SelectContent,
  SelectEmpty,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "./package-select";

export const ENUM_SELECT_EMPTY_VALUE = "__enum-select-empty__";

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
  className?: string;
  disabled?: boolean;
  emptyOptionsLabel?: string;
  id: string;
  loading?: boolean;
  loadingLabel?: string;
  onValueChange: (value: string) => void;
  options: readonly EnumSelectOption[];
  placeholder?: string;
  value: string;
}

export function EnumSelect({
  className,
  disabled,
  emptyOptionsLabel = "No options available",
  id,
  loading = false,
  loadingLabel = "Loading options...",
  onValueChange,
  options,
  placeholder,
  value,
  ...aria
}: EnumSelectProps) {
  const isEmpty = !loading && options.length === 0;
  const resolvedPlaceholder = loading ? loadingLabel : placeholder;

  return (
    <Select
      disabled={disabled || loading}
      onValueChange={onValueChange}
      value={loading ? SELECT_EMPTY_STATE_VALUE : value}
    >
      <SelectTrigger
        aria-busy={loading || undefined}
        className={className}
        id={id}
        {...aria}
      >
        <SelectValue placeholder={resolvedPlaceholder} />
      </SelectTrigger>
      <SelectContent>
        {loading ? (
          <SelectEmpty>{loadingLabel}</SelectEmpty>
        ) : isEmpty ? (
          <SelectEmpty>{emptyOptionsLabel}</SelectEmpty>
        ) : (
          options.map((option) => (
            <SelectItem
              disabled={option.disabled}
              key={option.value}
              value={option.value}
            >
              {option.label}
            </SelectItem>
          ))
        )}
      </SelectContent>
    </Select>
  );
}

export interface OptionalEnumSelectProps extends EnumSelectAriaProps {
  className?: string;
  disabled?: boolean;
  emptyOptionLabel: string;
  id: string;
  onValueChange: (value: string | null) => void;
  options: readonly EnumSelectOption[];
  value: string | null | undefined;
}

export function OptionalEnumSelect({
  className,
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
      <SelectTrigger className={className} id={id} {...aria}>
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
  className?: string;
  disabled?: boolean;
  id: string;
  onValueChange: (value: string) => void;
  options: readonly EnumSelectOption[];
  placeholder: string;
}

export function ResetEnumSelect({
  className,
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
      <SelectTrigger className={className} id={id} {...aria}>
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
