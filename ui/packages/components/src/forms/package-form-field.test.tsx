// @vitest-environment happy-dom

import { describe, expect, it } from "vitest";

import { renderPackageComponent, screen } from "../testing/render";
import { PackageCheckbox } from "./package-checkbox";
import {
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
import { NativeSelect } from "./package-native-select";
import { PackageTextarea } from "./package-textarea";

describe("Package form-field messaging", () => {
  it("renders field layout, visible labels, and supporting descriptions", () => {
    renderPackageComponent(
      <FormField className="custom-field" data-testid="form-field">
        <FormLabel htmlFor="factory-name">Factory name</FormLabel>
        <PackageInput id="factory-name" />
        <FormDescription>Used for exported filenames.</FormDescription>
      </FormField>,
    );

    const field = screen.getByTestId("form-field");
    const label = screen.getByText("Factory name");
    const description = screen.getByText("Used for exported filenames.");

    expect(field.className).toContain("space-y-2");
    expect(field.className).toContain("custom-field");
    expect(label.tagName).toBe("LABEL");
    expect(label.className).toContain("font-semibold");
    expect(label.className).toContain("text-on-surface");
    expect(description.className).toContain("text-body-small");
    expect(description.className).toContain("text-on-surface-variant");
  });

  it("renders validation errors with alert semantics and error color roles", () => {
    renderPackageComponent(
      <FormError id="factory-name-error">Name is required.</FormError>,
    );

    const error = screen.getByRole("alert");

    expect(error).toHaveAttribute("id", "factory-name-error");
    expect(error.className).toContain("text-body-small");
    expect(error.className).toContain("font-medium");
    expect(error.className).toContain("text-on-error-container");
  });

  it("supports non-label field titles with the same visual contract", () => {
    renderPackageComponent(<FormLabel as="p">Prompt body</FormLabel>);

    const title = screen.getByText("Prompt body");

    expect(title.tagName).toBe("P");
    expect(title.className).toContain("font-semibold");
    expect(title.className).toContain("text-on-surface");
  });

  it("can render body-weight descriptions for compatibility wrappers", () => {
    renderPackageComponent(
      <FormDescription variant="body">Definition unavailable.</FormDescription>,
    );

    expect(screen.getByText("Definition unavailable.").className).toContain(
      "text-body-medium",
    );
  });

  it("renders helper, warning, and success feedback with distinct color roles", () => {
    renderPackageComponent(
      <>
        <FormHelperText id="factory-name-helper">
          Shown on exported factory cards.
        </FormHelperText>
        <FormWarning role="status">Server value changed.</FormWarning>
        <FormSuccess>Saved successfully.</FormSuccess>
      </>,
    );

    const helper = screen.getByText("Shown on exported factory cards.");
    const warning = screen.getByText("Server value changed.");
    const success = screen.getByText("Saved successfully.");

    expect(helper.className).toContain("text-on-surface-variant");
    expect(warning.className).toContain("text-on-warning-container");
    expect(success.className).toContain("text-on-success-container");
  });
});

describe("Package form-field representative compositions", () => {
  it("renders a text input field through the package import surface", () => {
    renderPackageComponent(
      <FormField>
        <FormLabel htmlFor="name-input">Display name</FormLabel>
        <PackageInput aria-describedby="name-helper" id="name-input" />
        <FormHelperText id="name-helper">Visible in the header.</FormHelperText>
      </FormField>,
    );

    expect(screen.getByLabelText("Display name")).toHaveAttribute(
      "aria-describedby",
      "name-helper",
    );
  });

  it("renders a textarea field with host-supplied invalid and error messaging", () => {
    renderPackageComponent(
      <FormField>
        <FormLabel htmlFor="notes-input">Notes</FormLabel>
        <PackageTextarea
          aria-describedby="notes-error"
          aria-invalid
          id="notes-input"
        />
        <FormError id="notes-error">Notes are required.</FormError>
      </FormField>,
    );

    const textarea = screen.getByLabelText("Notes");

    expect(textarea).toHaveAttribute("aria-invalid", "true");
    expect(textarea).toHaveAttribute("aria-describedby", "notes-error");
    expect(screen.getByRole("alert")).toHaveTextContent("Notes are required.");
  });

  it("renders a checkbox field with required and disabled host props", () => {
    renderPackageComponent(
      <FormField>
        <FormLabel htmlFor="cron-checkbox">Enable cron trigger</FormLabel>
        <PackageCheckbox disabled id="cron-checkbox" required />
      </FormField>,
    );

    const checkbox = screen.getByLabelText("Enable cron trigger");

    expect(checkbox).toBeDisabled();
    expect(checkbox).toBeRequired();
  });

  it("renders a native select field with host-supplied label and description", () => {
    renderPackageComponent(
      <FormField>
        <FormLabel htmlFor="status-select">Status</FormLabel>
        <NativeSelect aria-describedby="status-description" id="status-select">
          <option value="active">Active</option>
          <option value="paused">Paused</option>
        </NativeSelect>
        <FormDescription id="status-description">
          Controls whether the factory accepts new work.
        </FormDescription>
      </FormField>,
    );

    expect(screen.getByLabelText("Status")).toHaveAttribute(
      "aria-describedby",
      "status-description",
    );
  });

  it("renders grouped controls with fieldset semantics and shared messaging", () => {
    renderPackageComponent(
      <FormFieldGroup aria-describedby="group-description">
        <FormFieldGroupLabel>Notification channels</FormFieldGroupLabel>
        <FormDescription id="group-description">
          Choose one or more delivery channels.
        </FormDescription>
        <FormField>
          <FormLabel htmlFor="email-channel">Email</FormLabel>
          <PackageCheckbox id="email-channel" />
        </FormField>
        <FormField>
          <FormLabel htmlFor="webhook-channel">Webhook</FormLabel>
          <PackageCheckbox id="webhook-channel" />
        </FormField>
      </FormFieldGroup>,
    );

    const group = screen.getByRole("group", { name: "Notification channels" });

    expect(group.tagName).toBe("FIELDSET");
    expect(group).toHaveAttribute("aria-describedby", "group-description");
    expect(screen.getByLabelText("Email")).toBeTruthy();
    expect(screen.getByLabelText("Webhook")).toBeTruthy();
  });
});
