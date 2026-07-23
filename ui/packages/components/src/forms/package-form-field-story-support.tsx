import type { Decorator } from "@storybook/react-vite";
import { type ReactNode, useId } from "react";

import { PackageCheckbox } from "./package-checkbox";
import {
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
import { PackageInput } from "./package-input";

export const MOBILE_STORY_WIDTH = "320px";

export const PACKAGE_FORM_FIELD_STORY_LABEL = "Display name";
export const PACKAGE_FORM_FIELD_STORY_DESCRIPTION =
  "Shown in lists and detail views.";
export const PACKAGE_FORM_FIELD_STORY_HELPER = "Use 3 to 40 characters.";
export const PACKAGE_FORM_FIELD_STORY_WARNING =
  "This value may be visible to other users.";
export const PACKAGE_FORM_FIELD_STORY_ERROR = "Display name is required.";
export const PACKAGE_FORM_FIELD_STORY_SUCCESS = "Saved successfully.";
export const PACKAGE_FORM_FIELD_LONG_MESSAGE =
  "This is a longer guidance message that should wrap cleanly inside narrow layouts without forcing horizontal scrolling or clipping adjacent labels.";
export const PACKAGE_FORM_FIELD_LONG_LABEL =
  "Display name with a longer label that should wrap inside narrow layouts";
export const PACKAGE_FORM_FIELD_GROUP_LABEL = "Notification preferences";
export const PACKAGE_FORM_FIELD_GROUP_DESCRIPTION =
  "Choose how you want to receive updates.";

export const withMobileWidth: Decorator = (Story) => (
  <div style={{ maxWidth: "100%", width: MOBILE_STORY_WIDTH }}>
    <Story />
  </div>
);

export const withStoryWidth: Decorator = (Story) => (
  <div className="w-full max-w-md">
    <Story />
  </div>
);

export type PackageFormFieldStoryOptions = {
  autoFocus?: boolean;
  defaultValue?: string;
  description?: string;
  disabled?: boolean;
  errorText?: string;
  helperText?: string;
  invalid?: boolean;
  label?: string;
  required?: boolean;
  requiredAffordance?: string;
  successText?: string;
  useAriaErrorMessage?: boolean;
  warningText?: string;
};

export function PackageFormFieldStoryExample({
  autoFocus = false,
  defaultValue,
  description,
  disabled = false,
  errorText,
  helperText,
  invalid = false,
  label = PACKAGE_FORM_FIELD_STORY_LABEL,
  required = false,
  requiredAffordance,
  successText,
  useAriaErrorMessage = false,
  warningText,
}: PackageFormFieldStoryOptions) {
  const reactId = useId();
  const controlId = `${reactId}-control`;
  const descriptionId = description ? `${reactId}-description` : undefined;
  const helperId = helperText ? `${reactId}-helper` : undefined;
  const warningId = warningText ? `${reactId}-warning` : undefined;
  const errorId = errorText ? `${reactId}-error` : undefined;
  const successId = successText ? `${reactId}-success` : undefined;
  const ariaDescribedBy = buildFormFieldAriaDescribedBy({
    descriptionId,
    helperId,
    warningId,
    ...(useAriaErrorMessage ? {} : { errorId }),
    successId,
  });

  return (
    <FormField>
      <FormLabel htmlFor={controlId}>
        {label}
        {requiredAffordance ? (
          <span className="text-on-error-container"> {requiredAffordance}</span>
        ) : null}
      </FormLabel>
      <PackageInput
        aria-describedby={ariaDescribedBy}
        aria-errormessage={useAriaErrorMessage ? errorId : undefined}
        aria-invalid={invalid || errorText ? true : undefined}
        autoFocus={autoFocus}
        defaultValue={defaultValue}
        disabled={disabled}
        id={controlId}
        required={required}
      />
      {description ? (
        <FormDescription id={descriptionId}>{description}</FormDescription>
      ) : null}
      {helperText ? (
        <FormHelperText id={helperId}>{helperText}</FormHelperText>
      ) : null}
      {warningText ? (
        <FormWarning id={warningId} role="status">
          {warningText}
        </FormWarning>
      ) : null}
      {errorText ? <FormError id={errorId}>{errorText}</FormError> : null}
      {successText ? (
        <FormSuccess id={successId} role="status">
          {successText}
        </FormSuccess>
      ) : null}
    </FormField>
  );
}

export function PackageFormFieldGroupedControlStoryExample({
  children,
}: {
  children?: ReactNode;
}) {
  const reactId = useId();
  const descriptionId = `${reactId}-group-description`;
  const emailId = `${reactId}-email`;
  const smsId = `${reactId}-sms`;

  return (
    <FormFieldGroup aria-describedby={descriptionId}>
      <FormFieldGroupLabel>
        {PACKAGE_FORM_FIELD_GROUP_LABEL}
      </FormFieldGroupLabel>
      <FormDescription id={descriptionId}>
        {PACKAGE_FORM_FIELD_GROUP_DESCRIPTION}
      </FormDescription>
      <FormField>
        <FormLabel htmlFor={emailId}>Email</FormLabel>
        <PackageCheckbox defaultChecked id={emailId} />
      </FormField>
      <FormField>
        <FormLabel htmlFor={smsId}>Text message</FormLabel>
        <PackageCheckbox id={smsId} />
      </FormField>
      {children}
    </FormFieldGroup>
  );
}
