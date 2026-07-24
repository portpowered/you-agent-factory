// @vitest-environment happy-dom

import { useState } from "react";
import { describe, expect, it, vi } from "vitest";

import { renderPackageComponent, screen, userEvent } from "../testing/render";
import { PackageCheckbox } from "./package-checkbox";
import { PackageFileInput } from "./package-file-input";
import { PackageInput } from "./package-input";
import { PackageTextarea } from "./package-textarea";

function ControlledInputExample() {
  const [value, setValue] = useState("");

  return (
    <PackageInput
      aria-label="Factory name"
      onChange={(event) => {
        setValue(event.target.value);
      }}
      value={value}
    />
  );
}

function UncontrolledInputExample() {
  return (
    <PackageInput aria-label="Factory name" defaultValue="Initial value" />
  );
}

function ControlledTextareaExample() {
  const [value, setValue] = useState("");

  return (
    <PackageTextarea
      aria-label="Factory notes"
      onChange={(event) => {
        setValue(event.target.value);
      }}
      value={value}
    />
  );
}

function UncontrolledTextareaExample() {
  return (
    <PackageTextarea aria-label="Factory notes" defaultValue="Initial notes" />
  );
}

function ControlledCheckboxExample() {
  const [checked, setChecked] = useState(false);

  return (
    <PackageCheckbox
      aria-label="Enable cron trigger"
      checked={checked}
      onChange={(event) => {
        setChecked(event.target.checked);
      }}
    />
  );
}

function UncontrolledCheckboxExample() {
  return (
    <PackageCheckbox aria-label="Enable cron trigger" defaultChecked={false} />
  );
}

describe("Package input primitive value state", () => {
  it("updates controlled and uncontrolled text input values through typing", async () => {
    const user = userEvent.setup();

    const { unmount: unmountControlled } = renderPackageComponent(
      <ControlledInputExample />,
    );

    const controlledInput =
      screen.getByLabelText<HTMLInputElement>("Factory name");
    await user.type(controlledInput, "Alpha");
    expect(controlledInput.value).toBe("Alpha");
    unmountControlled();

    renderPackageComponent(<UncontrolledInputExample />);
    const uncontrolledInput =
      screen.getByLabelText<HTMLInputElement>("Factory name");
    expect(uncontrolledInput.value).toBe("Initial value");
    await user.clear(uncontrolledInput);
    await user.type(uncontrolledInput, "Beta");
    expect(uncontrolledInput.value).toBe("Beta");
  });

  it("updates controlled and uncontrolled textarea values through typing", async () => {
    const user = userEvent.setup();

    const { unmount: unmountControlled } = renderPackageComponent(
      <ControlledTextareaExample />,
    );

    const controlledTextarea =
      screen.getByLabelText<HTMLTextAreaElement>("Factory notes");
    await user.type(controlledTextarea, "Updated notes");
    expect(controlledTextarea.value).toBe("Updated notes");
    unmountControlled();

    renderPackageComponent(<UncontrolledTextareaExample />);
    const uncontrolledTextarea =
      screen.getByLabelText<HTMLTextAreaElement>("Factory notes");
    expect(uncontrolledTextarea.value).toBe("Initial notes");
    await user.clear(uncontrolledTextarea);
    await user.type(uncontrolledTextarea, "Revised notes");
    expect(uncontrolledTextarea.value).toBe("Revised notes");
  });

  it("toggles controlled and uncontrolled checkbox state through pointer and keyboard", async () => {
    const user = userEvent.setup();

    const { unmount: unmountControlled } = renderPackageComponent(
      <ControlledCheckboxExample />,
    );

    const controlledCheckbox = screen.getByRole("checkbox", {
      name: "Enable cron trigger",
    });
    expect(controlledCheckbox).not.toBeChecked();

    await user.click(controlledCheckbox);
    expect(controlledCheckbox).toBeChecked();

    controlledCheckbox.focus();
    await user.keyboard(" ");
    expect(controlledCheckbox).not.toBeChecked();
    unmountControlled();

    renderPackageComponent(<UncontrolledCheckboxExample />);
    const uncontrolledCheckbox = screen.getByRole("checkbox", {
      name: "Enable cron trigger",
    });
    expect(uncontrolledCheckbox).not.toBeChecked();

    await user.click(uncontrolledCheckbox);
    expect(uncontrolledCheckbox).toBeChecked();

    uncontrolledCheckbox.focus();
    await user.keyboard(" ");
    expect(uncontrolledCheckbox).not.toBeChecked();
  });

  it("exposes selected file names without owning upload side effects", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();

    renderPackageComponent(
      <PackageFileInput aria-label="Factory cover image" onChange={onChange} />,
    );

    const input = screen.getByLabelText<HTMLInputElement>(
      "Factory cover image",
    );
    const file = new File(["cover"], "cover.png", { type: "image/png" });

    await user.upload(input, file);

    expect(onChange).toHaveBeenCalledTimes(1);
    expect(input.files).toHaveLength(1);
    expect(input.files?.[0]?.name).toBe("cover.png");
    expect(onChange.mock.calls[0]?.[0].target.files?.[0]?.name).toBe(
      "cover.png",
    );
  });
});

describe("Package input primitive disabled behavior", () => {
  it("prevents disabled primitives from changing through pointer or keyboard", async () => {
    const user = userEvent.setup();
    const onInputChange = vi.fn();
    const onCheckboxChange = vi.fn();
    const onFileChange = vi.fn();

    renderPackageComponent(
      <>
        <PackageInput
          aria-label="Disabled name"
          defaultValue="Locked"
          disabled
          onChange={onInputChange}
        />
        <PackageCheckbox
          aria-label="Disabled checkbox"
          defaultChecked={false}
          disabled
          onChange={onCheckboxChange}
        />
        <PackageFileInput
          aria-label="Disabled file"
          disabled
          onChange={onFileChange}
        />
      </>,
    );

    const input = screen.getByLabelText("Disabled name");
    const checkbox = screen.getByRole("checkbox", {
      name: "Disabled checkbox",
    });
    const fileInput = screen.getByLabelText("Disabled file");

    expect(input).toBeDisabled();
    expect(checkbox).toBeDisabled();
    expect(fileInput).toBeDisabled();

    await user.click(input);
    await user.type(input, "changed");
    expect(onInputChange).not.toHaveBeenCalled();
    expect(input).toHaveValue("Locked");

    await user.click(checkbox);
    checkbox.focus();
    await user.keyboard(" ");
    expect(onCheckboxChange).not.toHaveBeenCalled();
    expect(checkbox).not.toBeChecked();

    const file = new File(["cover"], "cover.png", { type: "image/png" });
    await user.upload(fileInput, file);
    expect(onFileChange).not.toHaveBeenCalled();
    expect(fileInput.files).toHaveLength(0);
  });
});

describe("Package input primitive invalid and error relationships", () => {
  it("forwards invalid state and error-message relationships from the host application", () => {
    renderPackageComponent(
      <>
        <PackageInput
          aria-describedby="factory-name-error"
          aria-invalid="true"
          aria-label="Factory name"
          id="factory-name"
        />
        <p id="factory-name-error" role="alert">
          Name is required.
        </p>
        <PackageTextarea
          aria-describedby="factory-notes-error"
          aria-invalid="true"
          aria-label="Factory notes"
          id="factory-notes"
        />
        <p id="factory-notes-error" role="alert">
          Notes are required.
        </p>
        <PackageCheckbox
          aria-describedby="cron-trigger-error"
          aria-invalid="true"
          aria-label="Enable cron trigger"
          id="cron-trigger"
        />
        <p id="cron-trigger-error" role="alert">
          Confirmation is required.
        </p>
        <PackageFileInput
          aria-describedby="cover-image-error"
          aria-invalid="true"
          aria-label="Factory cover image"
          id="cover-image"
        />
        <p id="cover-image-error" role="alert">
          Cover image is required.
        </p>
      </>,
    );

    const input = screen.getByLabelText("Factory name");
    const textarea = screen.getByLabelText("Factory notes");
    const checkbox = screen.getByRole("checkbox", {
      name: "Enable cron trigger",
    });
    const fileInput = screen.getByLabelText("Factory cover image");

    expect(input).toHaveAttribute("aria-invalid", "true");
    expect(input).toHaveAttribute("aria-describedby", "factory-name-error");
    expect(screen.getByText("Name is required.")).toHaveAttribute(
      "id",
      "factory-name-error",
    );

    expect(textarea).toHaveAttribute("aria-invalid", "true");
    expect(textarea).toHaveAttribute("aria-describedby", "factory-notes-error");
    expect(screen.getByText("Notes are required.")).toHaveAttribute(
      "id",
      "factory-notes-error",
    );

    expect(checkbox).toHaveAttribute("aria-invalid", "true");
    expect(checkbox).toHaveAttribute("aria-describedby", "cron-trigger-error");
    expect(screen.getByText("Confirmation is required.")).toHaveAttribute(
      "id",
      "cron-trigger-error",
    );

    expect(fileInput).toHaveAttribute("aria-invalid", "true");
    expect(fileInput).toHaveAttribute("aria-describedby", "cover-image-error");
    expect(screen.getByText("Cover image is required.")).toHaveAttribute(
      "id",
      "cover-image-error",
    );

    expect(input.className).toContain("aria-invalid:border-af-danger-border");
    expect(textarea.className).toContain(
      "aria-invalid:border-af-danger-border",
    );
    expect(fileInput.className).toContain(
      "aria-invalid:border-af-danger-border",
    );

    const checkboxIndicator = checkbox.nextElementSibling;
    expect(checkboxIndicator?.className).toContain(
      "peer-aria-invalid:ring-af-danger-border",
    );
  });
});

describe("Package input primitive keyboard focus", () => {
  it("keeps keyboard focus visible on text input, textarea, checkbox, and file input", async () => {
    const user = userEvent.setup();

    renderPackageComponent(
      <>
        <PackageInput aria-label="Factory name" />
        <PackageTextarea aria-label="Factory notes" />
        <PackageCheckbox aria-label="Enable cron trigger" />
        <PackageFileInput aria-label="Factory cover image" />
      </>,
    );

    const input = screen.getByLabelText("Factory name");
    const textarea = screen.getByLabelText("Factory notes");
    const checkbox = screen.getByRole("checkbox", {
      name: "Enable cron trigger",
    });
    const fileInput = screen.getByLabelText("Factory cover image");

    expect(input.className).toContain("focus-visible:ring-af-focus-ring");
    expect(textarea.className).toContain("focus-visible:ring-af-focus-ring");

    const checkboxIndicator = checkbox.nextElementSibling;
    expect(checkboxIndicator?.className).toContain(
      "peer-focus-visible:ring-af-focus-ring",
    );
    expect(fileInput.className).toContain("focus-visible:ring-af-focus-ring");

    await user.tab();
    expect(input).toHaveFocus();

    await user.tab();
    expect(textarea).toHaveFocus();

    await user.tab();
    expect(checkbox).toHaveFocus();

    await user.tab();
    expect(fileInput).toHaveFocus();
  });
});
