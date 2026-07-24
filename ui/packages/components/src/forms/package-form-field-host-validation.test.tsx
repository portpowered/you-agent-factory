// @vitest-environment happy-dom

import { type ChangeEvent, useState } from "react";
import { describe, expect, it } from "vitest";

import { renderPackageComponent, screen, userEvent } from "../testing/render";
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

function HostValidatedTextField() {
  const [value, setValue] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  const handleChange = (event: ChangeEvent<HTMLInputElement>) => {
    const nextValue = event.target.value;
    setValue(nextValue);

    if (nextValue.trim().length === 0) {
      setError("Display name is required.");
      setSuccess(null);
      return;
    }

    if (nextValue.length < 3) {
      setError("Use at least three characters.");
      setSuccess(null);
      return;
    }

    setError(null);
    setSuccess("Looks good.");
  };

  const ariaDescribedBy = buildFormFieldAriaDescribedBy({
    helperId: "display-name-helper",
    successId: success ? "display-name-success" : undefined,
  });

  return (
    <FormField>
      <FormLabel htmlFor={CONTROL_ID}>Display name</FormLabel>
      <PackageInput
        aria-describedby={error ? "display-name-error" : ariaDescribedBy}
        aria-errormessage={error ? "display-name-error" : undefined}
        aria-invalid={error ? true : undefined}
        id={CONTROL_ID}
        onChange={handleChange}
        value={value}
      />
      <FormHelperText id="display-name-helper">
        Visible in the header.
      </FormHelperText>
      {error ? <FormError id="display-name-error">{error}</FormError> : null}
      {success ? (
        <FormSuccess id="display-name-success">{success}</FormSuccess>
      ) : null}
    </FormField>
  );
}

function HostMessageShowcase({
  description,
  error,
  helper,
  label,
  success,
  warning,
}: {
  description?: string;
  error?: string;
  helper?: string;
  label: string;
  success?: string;
  warning?: string;
}) {
  const messageIds = {
    descriptionId: description ? "field-description" : undefined,
    errorId: error ? "field-error" : undefined,
    helperId: helper ? "field-helper" : undefined,
    successId: success ? "field-success" : undefined,
    warningId: warning ? "field-warning" : undefined,
  };
  const ariaDescribedBy = buildFormFieldAriaDescribedBy(messageIds);

  return (
    <FormField>
      <FormLabel htmlFor={CONTROL_ID}>{label}</FormLabel>
      <PackageInput
        aria-describedby={error ? messageIds.errorId : ariaDescribedBy}
        aria-errormessage={error ? messageIds.errorId : undefined}
        aria-invalid={error ? true : undefined}
        id={CONTROL_ID}
      />
      {description ? (
        <FormDescription id={messageIds.descriptionId}>
          {description}
        </FormDescription>
      ) : null}
      {helper ? (
        <FormHelperText id={messageIds.helperId}>{helper}</FormHelperText>
      ) : null}
      {warning ? (
        <FormWarning id={messageIds.warningId}>{warning}</FormWarning>
      ) : null}
      {error ? <FormError id={messageIds.errorId}>{error}</FormError> : null}
      {success ? (
        <FormSuccess id={messageIds.successId}>{success}</FormSuccess>
      ) : null}
    </FormField>
  );
}

describe("Package form-field host-supplied messaging", () => {
  it("renders caller-provided label, description, helper, warning, error, and success text exactly as supplied", () => {
    renderPackageComponent(
      <HostMessageShowcase
        description="Used for exported filenames."
        error="Name is required."
        helper="Shown on exported factory cards."
        label="Factory name *"
        success="Saved successfully."
        warning="Server value changed."
      />,
    );

    expect(screen.getByText("Factory name *")).toBeTruthy();
    expect(screen.getByText("Used for exported filenames.")).toHaveTextContent(
      "Used for exported filenames.",
    );
    expect(
      screen.getByText("Shown on exported factory cards."),
    ).toHaveTextContent("Shown on exported factory cards.");
    expect(screen.getByText("Server value changed.")).toHaveTextContent(
      "Server value changed.",
    );
    expect(screen.getByText("Name is required.")).toHaveTextContent(
      "Name is required.",
    );
    expect(screen.getByText("Saved successfully.")).toHaveTextContent(
      "Saved successfully.",
    );
  });

  it("does not render validation messaging when the host omits message props", () => {
    renderPackageComponent(<HostMessageShowcase label="Factory name" />);

    const control = screen.getByRole("textbox", { name: "Factory name" });

    expect(screen.queryByRole("alert")).toBeNull();
    expect(screen.queryByRole("status")).toBeNull();
    expect(control).not.toHaveAttribute("aria-invalid");
    expect(control).not.toHaveAttribute("aria-describedby");
    expect(control).not.toHaveAttribute("aria-errormessage");
  });

  it("updates validation messaging and accessible relationships from host-owned state after user interaction", async () => {
    const user = userEvent.setup();

    renderPackageComponent(<HostValidatedTextField />);

    const control = screen.getByRole("textbox", { name: "Display name" });

    expect(control).toHaveAccessibleDescription("Visible in the header.");
    expect(control).not.toHaveAttribute("aria-invalid");
    expect(screen.queryByRole("alert")).toBeNull();

    await user.type(control, "a");
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Use at least three characters.",
    );
    expect(control).toHaveAttribute("aria-invalid", "true");
    expect(control).toHaveAccessibleErrorMessage(
      "Use at least three characters.",
    );

    await user.clear(control);
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Display name is required.",
    );
    expect(control).toHaveAccessibleErrorMessage("Display name is required.");

    await user.type(control, "Alpha");
    expect(screen.queryByRole("alert")).toBeNull();
    expect(control).not.toHaveAttribute("aria-invalid");
    expect(control).not.toHaveAccessibleErrorMessage();
    expect(screen.getByRole("status")).toHaveTextContent("Looks good.");
    expect(control).toHaveAccessibleDescription(
      "Visible in the header. Looks good.",
    );
  });
});
