import type { Decorator } from "@storybook/react-vite";
import { useId, useState, type ReactNode } from "react";

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "./package-select";

export const MOBILE_STORY_WIDTH = "320px";
export const PACKAGE_SELECT_STORY_LABEL = "Work type";

export const PACKAGE_SELECT_STORY_OPTIONS = [
  { label: "Story", value: "story" },
  { label: "Task", value: "task", disabled: true },
  { label: "Bug", value: "bug" },
] as const;

export type PackageSelectStoryControlProps = {
  id: string;
  "aria-describedby"?: string;
  "aria-invalid"?: boolean;
};

export const withMobileWidth: Decorator = (Story) => (
  <div style={{ maxWidth: "100%", width: MOBILE_STORY_WIDTH }}>
    <Story />
  </div>
);

export function PackageSelectStoryField({
  children,
  errorText,
  helperText,
  invalid = false,
  label,
}: {
  children: (props: PackageSelectStoryControlProps) => ReactNode;
  errorText?: string;
  helperText?: string;
  invalid?: boolean;
  label: string;
}) {
  const reactId = useId();
  const controlId = `${reactId}-control`;
  const helperId = helperText ? `${reactId}-helper` : undefined;
  const errorId = errorText ? `${reactId}-error` : undefined;
  const describedBy =
    [helperId, errorId].filter(Boolean).join(" ") || undefined;

  const controlProps: PackageSelectStoryControlProps = {
    id: controlId,
    ...(describedBy ? { "aria-describedby": describedBy } : {}),
    ...(invalid || errorText ? { "aria-invalid": true } : {}),
  };

  return (
    <div className="flex w-full max-w-md flex-col gap-1.5 text-on-surface">
      <label className="text-sm font-medium" htmlFor={controlId}>
        {label}
      </label>
      {children(controlProps)}
      {helperText ? (
        <p className="text-xs text-on-surface-variant" id={helperId}>
          {helperText}
        </p>
      ) : null}
      {errorText ? (
        <p className="text-xs text-af-danger" id={errorId} role="alert">
          {errorText}
        </p>
      ) : null}
    </div>
  );
}

function PackageSelectStoryOptions() {
  return (
    <>
      {PACKAGE_SELECT_STORY_OPTIONS.map((option) => (
        <SelectItem
          disabled={"disabled" in option ? option.disabled : undefined}
          key={option.value}
          value={option.value}
        >
          {option.label}
        </SelectItem>
      ))}
    </>
  );
}

export function ControlledSelectStoryExample() {
  const [value, setValue] = useState("story");

  return (
    <PackageSelectStoryField label={PACKAGE_SELECT_STORY_LABEL}>
      {(controlProps) => (
        <Select onValueChange={setValue} value={value}>
          <SelectTrigger aria-label={PACKAGE_SELECT_STORY_LABEL} {...controlProps}>
            <SelectValue placeholder="Select a work type" />
          </SelectTrigger>
          <SelectContent>
            <PackageSelectStoryOptions />
          </SelectContent>
        </Select>
      )}
    </PackageSelectStoryField>
  );
}

export function ControlledOpenSelectStoryExample() {
  const [open, setOpen] = useState(false);
  const [value, setValue] = useState("story");

  return (
    <PackageSelectStoryField
      helperText={open ? "Menu is open." : "Menu is closed."}
      label={PACKAGE_SELECT_STORY_LABEL}
    >
      {(controlProps) => (
        <Select
          onOpenChange={setOpen}
          onValueChange={setValue}
          open={open}
          value={value}
        >
          <SelectTrigger aria-label={PACKAGE_SELECT_STORY_LABEL} {...controlProps}>
            <SelectValue placeholder="Select a work type" />
          </SelectTrigger>
          <SelectContent>
            <PackageSelectStoryOptions />
          </SelectContent>
        </Select>
      )}
    </PackageSelectStoryField>
  );
}

export function KeyboardSelectStoryExample() {
  const [value, setValue] = useState<string | undefined>();

  return (
    <PackageSelectStoryField
      helperText="Use Tab to focus, ArrowDown or Space to open, arrows to move, Enter to select."
      label={PACKAGE_SELECT_STORY_LABEL}
    >
      {(controlProps) => (
        <Select onValueChange={setValue} value={value}>
          <SelectTrigger aria-label={PACKAGE_SELECT_STORY_LABEL} {...controlProps}>
            <SelectValue placeholder="Select a work type" />
          </SelectTrigger>
          <SelectContent>
            <PackageSelectStoryOptions />
          </SelectContent>
        </Select>
      )}
    </PackageSelectStoryField>
  );
}
