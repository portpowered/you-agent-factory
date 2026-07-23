// @vitest-environment happy-dom

import { describe, expect, it, vi } from "vitest";

import { renderPackageComponent, screen, userEvent } from "../testing/render";
import { PackageCheckbox } from "./package-checkbox";
import { PackageFileInput } from "./package-file-input";
import { inputVariants, PackageInput } from "./package-input";
import { PackageTextarea, textareaVariants } from "./package-textarea";

describe("Package input primitives", () => {
  it("renders text input with shared field styling", () => {
    renderPackageComponent(
      <PackageInput aria-label="Factory name" placeholder="Factory" />,
    );

    const input = screen.getByLabelText("Factory name");
    expect(input.className).toContain("border-outline");
    expect(input.className).toContain("bg-surface-container-high");
    expect(input.className).toContain("text-on-surface");
  });

  it("exposes input variant helpers for sibling composition", () => {
    expect(inputVariants({ className: "custom-input" })).toContain(
      "custom-input",
    );
    expect(textareaVariants({ className: "custom-textarea" })).toContain(
      "custom-textarea",
    );
    expect(textareaVariants()).toContain("min-h-28");
  });

  it("renders textarea field and plain variants", () => {
    renderPackageComponent(
      <>
        <PackageTextarea aria-label="Factory notes" />
        <PackageTextarea aria-label="Plain notes" variant="plain" />
      </>,
    );

    expect(screen.getByLabelText("Factory notes").className).toContain(
      "min-h-28",
    );
    expect(screen.getByLabelText("Plain notes").className).toContain(
      "resize-none",
    );
  });

  it("preserves native checkbox semantics with a styled indicator", () => {
    renderPackageComponent(
      <PackageCheckbox
        aria-label="Enable cron trigger"
        checked
        className="mr-2"
        onChange={() => undefined}
      />,
    );

    const checkbox = screen.getByRole("checkbox", {
      name: "Enable cron trigger",
    });

    expect(checkbox.getAttribute("type")).toBe("checkbox");
    expect(checkbox.className).toContain("sr-only");
    expect(checkbox.getAttribute("checked")).toBe("");

    const indicator = checkbox.nextElementSibling;
    expect(indicator?.className).toContain("peer-checked:bg-primary");
    expect(indicator?.querySelector("svg")).toBeTruthy();
    expect(checkbox.parentElement?.className).toContain("mr-2");
  });

  it("toggles checkbox through label clicks and keyboard interaction", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();

    renderPackageComponent(
      <>
        <label htmlFor="cron-trigger-checkbox">Enable cron trigger</label>
        <PackageCheckbox id="cron-trigger-checkbox" onChange={onChange} />
      </>,
    );

    const checkbox = screen.getByRole("checkbox", {
      name: "Enable cron trigger",
    });

    await user.click(screen.getByText("Enable cron trigger"));
    expect(onChange).toHaveBeenCalledTimes(1);
    expect(onChange.mock.calls[0]?.[0].target.checked).toBe(true);

    checkbox.focus();
    await user.keyboard(" ");
    expect(onChange).toHaveBeenCalledTimes(2);
    expect(onChange.mock.calls[1]?.[0].target.checked).toBe(false);
  });

  it("renders native file input chrome", () => {
    renderPackageComponent(
      <PackageFileInput
        aria-label="Factory cover image"
        className="custom-file"
      />,
    );

    const input = screen.getByLabelText("Factory cover image");

    expect(input.getAttribute("type")).toBe("file");
    expect(input.className).toContain("text-on-surface-variant");
    expect(input.className).toContain("file:bg-surface-container-high");
    expect(input.className).toContain("custom-file");
  });

  it("forwards standard form props such as disabled and required", () => {
    renderPackageComponent(
      <>
        <PackageInput
          aria-label="Required name"
          disabled
          name="factory-name"
          required
        />
        <PackageCheckbox
          aria-label="Required checkbox"
          disabled
          name="enabled"
          required
        />
      </>,
    );

    const input = screen.getByLabelText("Required name");
    expect(input.getAttribute("name")).toBe("factory-name");
    expect(input.hasAttribute("disabled")).toBe(true);
    expect(input.hasAttribute("required")).toBe(true);

    const checkbox = screen.getByRole("checkbox", {
      name: "Required checkbox",
    });
    expect(checkbox.getAttribute("name")).toBe("enabled");
    expect(checkbox.hasAttribute("disabled")).toBe(true);
    expect(checkbox.hasAttribute("required")).toBe(true);
  });
});
