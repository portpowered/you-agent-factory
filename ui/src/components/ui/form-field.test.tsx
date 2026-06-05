import "@testing-library/jest-dom/vitest";

import { render, screen } from "@testing-library/react";

import {
  FormDescription,
  FormError,
  FormField,
  FormLabel,
  FormWarning,
} from "./form-field";

describe("form field primitives", () => {
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
    expect(description.className).toContain("af-dashboard-supporting-text");
  });

  it("renders validation errors with alert semantics and error color roles", () => {
    render(<FormError id="factory-name-error">Name is required.</FormError>);

    const error = screen.getByRole("alert");

    expect(error).toHaveAttribute("id", "factory-name-error");
    expect(error.className).toContain("af-dashboard-supporting-text");
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
      "af-dashboard-body-text",
    );
  });

  it("renders warning feedback with warning color roles", () => {
    render(<FormWarning role="status">Server value changed.</FormWarning>);

    const warning = screen.getByRole("status");

    expect(warning.className).toContain("af-dashboard-supporting-text");
    expect(warning.className).toContain("font-medium");
    expect(warning.className).toContain("text-on-warning-container");
  });
});
