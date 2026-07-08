// @vitest-environment happy-dom

import { describe, expect, it } from "vitest";

import { renderPackageComponent, screen } from "../testing/render";
import { PackageCheckbox } from "./package-checkbox";
import {
  buildFormFieldAriaDescribedBy,
  FormDescription,
  FormError,
  FormField,
  FormHelperText,
  FormLabel,
  FormSuccess,
  FormWarning,
} from "./package-form-field";
import { PackageInput } from "./package-input";

const CONTROL_ID = "display-name";

function renderAccessibleTextField({
  ariaDescribedBy,
  ariaErrorMessage,
  description,
  descriptionId,
  disabled = false,
  error,
  errorId,
  helper,
  helperId,
  invalid = false,
  label = "Display name",
  required = false,
  requiredAffordance,
  success,
  successId,
  warning,
  warningId,
}: {
  ariaDescribedBy?: string;
  ariaErrorMessage?: string;
  description?: string;
  descriptionId?: string;
  disabled?: boolean;
  error?: string;
  errorId?: string;
  helper?: string;
  helperId?: string;
  invalid?: boolean;
  label?: string;
  required?: boolean;
  requiredAffordance?: string;
  success?: string;
  successId?: string;
  warning?: string;
  warningId?: string;
}) {
  renderPackageComponent(
    <FormField>
      <FormLabel htmlFor={CONTROL_ID}>
        {label}
        {requiredAffordance ? (
          <span className="text-on-error-container"> {requiredAffordance}</span>
        ) : null}
      </FormLabel>
      <PackageInput
        aria-describedby={ariaDescribedBy}
        aria-errormessage={ariaErrorMessage}
        aria-invalid={invalid || error ? true : undefined}
        disabled={disabled}
        id={CONTROL_ID}
        required={required}
      />
      {description ? (
        <FormDescription id={descriptionId}>{description}</FormDescription>
      ) : null}
      {helper ? <FormHelperText id={helperId}>{helper}</FormHelperText> : null}
      {warning ? <FormWarning id={warningId}>{warning}</FormWarning> : null}
      {error ? <FormError id={errorId}>{error}</FormError> : null}
      {success ? <FormSuccess id={successId}>{success}</FormSuccess> : null}
    </FormField>,
  );
}

describe("Package form-field accessible relationships", () => {
  it("exposes the accessible name from a visible label", () => {
    renderAccessibleTextField({ label: "Factory name" });

    const control = screen.getByRole("textbox", { name: "Factory name" });

    expect(control).toHaveAccessibleName("Factory name");
    expect(control).not.toHaveAttribute("aria-describedby");
    expect(control).not.toHaveAccessibleDescription();
  });

  it("includes description text in the accessible description when supplied", () => {
    renderAccessibleTextField({
      ariaDescribedBy: "factory-description",
      description: "Used for exported filenames.",
      descriptionId: "factory-description",
    });

    const control = screen.getByRole("textbox", { name: "Display name" });

    expect(control).toHaveAttribute("aria-describedby", "factory-description");
    expect(control).toHaveAccessibleDescription("Used for exported filenames.");
  });

  it("includes helper text in the accessible description when supplied", () => {
    renderAccessibleTextField({
      ariaDescribedBy: "factory-helper",
      helper: "Shown on exported factory cards.",
      helperId: "factory-helper",
    });

    const control = screen.getByRole("textbox", { name: "Display name" });

    expect(control).toHaveAccessibleDescription(
      "Shown on exported factory cards.",
    );
  });

  it("includes warning text in the accessible description when supplied", () => {
    renderAccessibleTextField({
      ariaDescribedBy: "factory-warning",
      warning: "Server value changed.",
      warningId: "factory-warning",
    });

    const control = screen.getByRole("textbox", { name: "Display name" });

    expect(control).toHaveAccessibleDescription("Server value changed.");
  });

  it("connects invalid controls to error text through aria-invalid and aria-errormessage", () => {
    renderAccessibleTextField({
      ariaErrorMessage: "factory-error",
      error: "Name is required.",
      errorId: "factory-error",
      invalid: true,
    });

    const control = screen.getByRole("textbox", { name: "Display name" });

    expect(control).toHaveAttribute("aria-invalid", "true");
    expect(control).toHaveAttribute("aria-errormessage", "factory-error");
    expect(control).toHaveAccessibleErrorMessage("Name is required.");
    expect(screen.getByRole("alert")).toHaveTextContent("Name is required.");
  });

  it("connects invalid controls to error text through aria-describedby", () => {
    renderAccessibleTextField({
      ariaDescribedBy: "factory-error",
      error: "Name is required.",
      errorId: "factory-error",
      invalid: true,
    });

    const control = screen.getByRole("textbox", { name: "Display name" });

    expect(control).toHaveAttribute("aria-invalid", "true");
    expect(control).toHaveAccessibleDescription("Name is required.");
    expect(screen.getByRole("alert")).toHaveTextContent("Name is required.");
  });

  it("includes success text in the accessible description when supplied", () => {
    renderAccessibleTextField({
      ariaDescribedBy: "factory-success",
      success: "Saved successfully.",
      successId: "factory-success",
    });

    const control = screen.getByRole("textbox", { name: "Display name" });

    expect(control).toHaveAccessibleDescription("Saved successfully.");
    expect(screen.getByRole("status")).toHaveTextContent("Saved successfully.");
  });

  it("exposes required state to assistive technology and keeps the affordance visible", () => {
    renderAccessibleTextField({
      label: "Request name",
      required: true,
      requiredAffordance: "(required)",
    });

    const control = screen.getByRole("textbox", { name: /Request name/ });

    expect(control).toBeRequired();
    expect(screen.getByText("(required)")).toBeVisible();
  });

  it("exposes disabled state to assistive technology", () => {
    renderAccessibleTextField({
      disabled: true,
      label: "Factory name",
    });

    const control = screen.getByRole("textbox", { name: "Factory name" });

    expect(control).toBeDisabled();
  });

  it("omits non-supplied description messaging from accessible relationships", () => {
    renderAccessibleTextField({ label: "Factory name" });

    const control = screen.getByRole("textbox", { name: "Factory name" });

    expect(control).not.toHaveAttribute("aria-describedby");
    expect(control).not.toHaveAttribute("aria-errormessage");
    expect(control).not.toHaveAccessibleDescription();
    expect(control).not.toHaveAccessibleErrorMessage();
  });

  it("combines description, helper, warning, and success messaging in one accessible description", () => {
    const messageIds = {
      descriptionId: "factory-description",
      helperId: "factory-helper",
      warningId: "factory-warning",
      successId: "factory-success",
    };
    const ariaDescribedBy = buildFormFieldAriaDescribedBy(messageIds);

    renderAccessibleTextField({
      ariaDescribedBy,
      description: "Used for exported filenames.",
      descriptionId: messageIds.descriptionId,
      helper: "Shown on exported factory cards.",
      helperId: messageIds.helperId,
      success: "Saved successfully.",
      successId: messageIds.successId,
      warning: "Server value changed.",
      warningId: messageIds.warningId,
    });

    const control = screen.getByRole("textbox", { name: "Display name" });

    expect(ariaDescribedBy).toBe(
      "factory-description factory-helper factory-warning factory-success",
    );
    expect(control).toHaveAttribute("aria-describedby", ariaDescribedBy);
    expect(control).toHaveAccessibleDescription(
      "Used for exported filenames. Shown on exported factory cards. Server value changed. Saved successfully.",
    );
  });

  it("combines non-error messaging with a separate aria-errormessage relationship", () => {
    const ariaDescribedBy = buildFormFieldAriaDescribedBy({
      descriptionId: "factory-description",
      helperId: "factory-helper",
    });

    renderAccessibleTextField({
      ariaDescribedBy,
      ariaErrorMessage: "factory-error",
      description: "Used for exported filenames.",
      descriptionId: "factory-description",
      error: "Name is required.",
      errorId: "factory-error",
      helper: "Shown on exported factory cards.",
      helperId: "factory-helper",
      invalid: true,
    });

    const control = screen.getByRole("textbox", { name: "Display name" });

    expect(control).toHaveAccessibleDescription(
      "Used for exported filenames. Shown on exported factory cards.",
    );
    expect(control).toHaveAccessibleErrorMessage("Name is required.");
  });

  it("exposes required and disabled checkbox states with a visible label relationship", () => {
    renderPackageComponent(
      <FormField>
        <FormLabel htmlFor="cron-checkbox">
          Enable cron trigger{" "}
          <span className="text-on-error-container">(required)</span>
        </FormLabel>
        <PackageCheckbox disabled id="cron-checkbox" required />
      </FormField>,
    );

    const checkbox = screen.getByRole("checkbox", {
      name: /Enable cron trigger/,
    });

    expect(checkbox).toBeRequired();
    expect(checkbox).toBeDisabled();
    expect(screen.getByText("(required)")).toBeVisible();
  });
});
