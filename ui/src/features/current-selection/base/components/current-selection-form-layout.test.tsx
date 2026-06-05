import { render, screen } from "@testing-library/react";

import {
  CurrentSelectionFormField,
  CurrentSelectionFormFields,
} from "./current-selection-form-layout";

describe("CurrentSelectionFormFields", () => {
  it("renders a single-column editable field stack", () => {
    render(
      <CurrentSelectionFormFields data-testid="fields">
        <div>Field</div>
      </CurrentSelectionFormFields>,
    );

    expect(screen.getByTestId("fields").className).toContain("grid-cols-1");
    expect(screen.getByTestId("fields").className).toContain("gap-3");
  });
});

describe("CurrentSelectionFormField", () => {
  it("renders field contents without adding a panel outline", () => {
    render(
      <CurrentSelectionFormField data-testid="field">
        <label htmlFor="field-input">Name</label>
        <input id="field-input" />
      </CurrentSelectionFormField>,
    );

    const field = screen.getByTestId("field");
    expect(field.className).toContain("grid");
    expect(field.className).not.toContain("border");
  });
});
