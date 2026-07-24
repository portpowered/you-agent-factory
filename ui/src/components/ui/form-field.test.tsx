import "@testing-library/jest-dom/vitest";

import { render, screen } from "@testing-library/react";
import {
  buildFormFieldAriaDescribedBy as PackageBuildFormFieldAriaDescribedBy,
  FormDescription as PackageFormDescription,
  FormError as PackageFormError,
  FormField as PackageFormField,
  FormFieldGroup as PackageFormFieldGroup,
  FormFieldGroupLabel as PackageFormFieldGroupLabel,
  FormHelperText as PackageFormHelperText,
  FormLabel as PackageFormLabel,
  FormSuccess as PackageFormSuccess,
  FormWarning as PackageFormWarning,
} from "@you-agent-factory/components/forms";
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
} from "./form-field";

describe("dashboard form field re-exports", () => {
  it("re-exports package form-field messaging from the dashboard ui surface", () => {
    expect(FormField).toBe(PackageFormField);
    expect(FormLabel).toBe(PackageFormLabel);
    expect(FormDescription).toBe(PackageFormDescription);
    expect(FormError).toBe(PackageFormError);
    expect(FormWarning).toBe(PackageFormWarning);
    expect(FormHelperText).toBe(PackageFormHelperText);
    expect(FormSuccess).toBe(PackageFormSuccess);
    expect(FormFieldGroup).toBe(PackageFormFieldGroup);
    expect(FormFieldGroupLabel).toBe(PackageFormFieldGroupLabel);
    expect(buildFormFieldAriaDescribedBy).toBe(
      PackageBuildFormFieldAriaDescribedBy,
    );
  });

  it("renders field layout, visible labels, and supporting descriptions", () => {
    render(
      <FormField className="custom-field">
        <FormLabel htmlFor="factory-name">Factory name</FormLabel>
        <input id="factory-name" />
        <FormDescription>Used for exported filenames.</FormDescription>
      </FormField>,
    );

    const field = screen.getByText("Factory name").parentElement;
    const label = screen.getByText("Factory name");
    const description = screen.getByText("Used for exported filenames.");

    expect(field?.className).toContain("space-y-2");
    expect(field?.className).toContain("custom-field");
    expect(label.tagName).toBe("LABEL");
    expect(label.className).toContain("font-semibold");
    expect(label.className).toContain("text-on-surface");
    expect(description.className).toContain("text-body-small");
    expect(description.className).toContain("text-on-surface-variant");
  });

  it("renders validation errors with alert semantics and error color roles", () => {
    render(<FormError id="factory-name-error">Name is required.</FormError>);

    const error = screen.getByRole("alert");

    expect(error).toHaveAttribute("id", "factory-name-error");
    expect(error.className).toContain("text-body-small");
    expect(error.className).toContain("font-medium");
    expect(error.className).toContain("text-on-error-container");
  });

  it("supports non-label field titles with the same visual contract", () => {
    render(<FormLabel as="p">Prompt body</FormLabel>);

    const title = screen.getByText("Prompt body");

    expect(title.tagName).toBe("P");
    expect(title.className).toContain("font-semibold");
    expect(title.className).toContain("text-on-surface");
  });

  it("can render body-weight descriptions for compatibility wrappers", () => {
    render(
      <FormDescription variant="body">Definition unavailable.</FormDescription>,
    );

    expect(screen.getByText("Definition unavailable.").className).toContain(
      "text-body-medium",
    );
  });

  it("renders warning feedback with warning color roles", () => {
    render(<FormWarning role="status">Server value changed.</FormWarning>);

    const warning = screen.getByRole("status");

    expect(warning.className).toContain("text-body-small");
    expect(warning.className).toContain("font-medium");
    expect(warning.className).toContain("text-on-warning-container");
  });

  it("renders helper text and success messaging with the package typography contract", () => {
    render(
      <>
        <FormHelperText id="factory-helper">Shown after save.</FormHelperText>
        <FormSuccess id="factory-success">Saved successfully.</FormSuccess>
      </>,
    );

    expect(screen.getByText("Shown after save.").className).toContain(
      "text-body-small",
    );
    expect(screen.getByText("Saved successfully.").className).toContain(
      "text-body-small",
    );
  });

  it("renders grouped field labels through the fieldset legend contract", () => {
    render(
      <FormFieldGroup>
        <FormFieldGroupLabel>Notification channels</FormFieldGroupLabel>
      </FormFieldGroup>,
    );

    const legend = screen.getByText("Notification channels");
    expect(legend.tagName).toBe("LEGEND");
    expect(legend.className).toContain("font-semibold");
  });

  it("joins host message element ids through buildFormFieldAriaDescribedBy", () => {
    expect(
      buildFormFieldAriaDescribedBy({
        descriptionId: "factory-description",
        helperId: "factory-helper",
        warningId: "factory-warning",
        successId: "factory-success",
      }),
    ).toBe(
      "factory-description factory-helper factory-warning factory-success",
    );
    expect(buildFormFieldAriaDescribedBy({})).toBeUndefined();
  });
});
