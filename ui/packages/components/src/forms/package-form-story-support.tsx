import type { Decorator } from "@storybook/react-vite";
import { type ReactNode, useId, useState } from "react";

import { PackageCheckbox } from "./package-checkbox";
import { PackageFileInput } from "./package-file-input";
import { PackageInput } from "./package-input";
import { PackageTextarea } from "./package-textarea";

export const MOBILE_STORY_WIDTH = "320px";

export const PACKAGE_INPUT_STORY_LABEL = "Factory name";
export const PACKAGE_TEXTAREA_STORY_LABEL = "Factory notes";
export const PACKAGE_CHECKBOX_STORY_LABEL = "Enable cron trigger";
export const PACKAGE_FILE_INPUT_STORY_LABEL = "Factory cover image";

export type PackageFormStoryControlProps = {
  id: string;
  "aria-describedby"?: string;
  "aria-invalid"?: boolean;
};

export const withMobileWidth: Decorator = (Story) => (
  <div style={{ maxWidth: "100%", width: MOBILE_STORY_WIDTH }}>
    <Story />
  </div>
);

export function PackageFormStoryField({
  children,
  errorText,
  helperText,
  invalid = false,
  label,
}: {
  children: (props: PackageFormStoryControlProps) => ReactNode;
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

  const controlProps: PackageFormStoryControlProps = {
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

export function ControlledInputStoryExample() {
  const [value, setValue] = useState("Alpha factory");

  return (
    <PackageFormStoryField label={PACKAGE_INPUT_STORY_LABEL}>
      {(controlProps) => (
        <PackageInput
          {...controlProps}
          onChange={(event) => {
            setValue(event.target.value);
          }}
          value={value}
        />
      )}
    </PackageFormStoryField>
  );
}

export function UncontrolledInputStoryExample() {
  return (
    <PackageFormStoryField label={PACKAGE_INPUT_STORY_LABEL}>
      {(controlProps) => (
        <PackageInput {...controlProps} defaultValue="Initial factory name" />
      )}
    </PackageFormStoryField>
  );
}

export function ControlledTextareaStoryExample() {
  const [value, setValue] = useState("Initial notes");

  return (
    <PackageFormStoryField label={PACKAGE_TEXTAREA_STORY_LABEL}>
      {(controlProps) => (
        <PackageTextarea
          {...controlProps}
          onChange={(event) => {
            setValue(event.target.value);
          }}
          value={value}
        />
      )}
    </PackageFormStoryField>
  );
}

export function UncontrolledTextareaStoryExample() {
  return (
    <PackageFormStoryField label={PACKAGE_TEXTAREA_STORY_LABEL}>
      {(controlProps) => (
        <PackageTextarea
          {...controlProps}
          defaultValue="Initial factory notes"
        />
      )}
    </PackageFormStoryField>
  );
}

export function ControlledCheckboxStoryExample() {
  const [checked, setChecked] = useState(false);

  return (
    <PackageFormStoryField label={PACKAGE_CHECKBOX_STORY_LABEL}>
      {(controlProps) => (
        <PackageCheckbox
          {...controlProps}
          checked={checked}
          onChange={(event) => {
            setChecked(event.target.checked);
          }}
        />
      )}
    </PackageFormStoryField>
  );
}

export function UncontrolledCheckboxStoryExample() {
  return (
    <PackageFormStoryField label={PACKAGE_CHECKBOX_STORY_LABEL}>
      {(controlProps) => (
        <PackageCheckbox {...controlProps} defaultChecked={false} />
      )}
    </PackageFormStoryField>
  );
}

export function SelectedFileInputStoryExample() {
  const [selectedName, setSelectedName] = useState<string | undefined>();

  return (
    <PackageFormStoryField
      helperText={
        selectedName
          ? `Selected file: ${selectedName}`
          : "PNG or JPEG up to 2 MB."
      }
      label={PACKAGE_FILE_INPUT_STORY_LABEL}
    >
      {(controlProps) => (
        <PackageFileInput
          {...controlProps}
          onChange={(event) => {
            setSelectedName(event.target.files?.[0]?.name);
          }}
        />
      )}
    </PackageFormStoryField>
  );
}
