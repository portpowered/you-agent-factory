import { render, screen } from "@testing-library/react";

import { Input, inputVariants } from "./input";
import { NativeSelect } from "./native-select";
import { Textarea } from "./textarea";

describe("Input primitives", () => {
  it("renders role-based input surface styling without exporting raw classes", () => {
    render(<Input aria-label="Factory name" placeholder="Factory" />);

    const input = screen.getByLabelText("Factory name");
    expect(input.className).toContain("border-outline");
    expect(input.className).toContain("bg-surface-container-high");
    expect(input.className).toContain("text-on-surface");
  });

  it("shares input variants with select and textarea controls", () => {
    render(
      <>
        <NativeSelect aria-label="Factory mode">
          <option>Automatic</option>
        </NativeSelect>
        <Textarea aria-label="Factory notes" />
      </>,
    );

    const select = screen.getByLabelText("Factory mode");
    expect(select.className).toContain("appearance-none");
    expect(select.className).toContain("border-outline");

    const textarea = screen.getByLabelText("Factory notes");
    expect(textarea.className).toContain("min-h-28");
    expect(textarea.className).toContain("border-outline");
  });

  it("exposes variant class generation for sibling primitive composition", () => {
    expect(inputVariants({ className: "custom-input" })).toContain(
      "custom-input",
    );
  });

  it("applies the shared invalid field treatment when aria-invalid is true", () => {
    render(<Input aria-invalid="true" aria-label="Factory name" />);

    const input = screen.getByLabelText("Factory name");
    expect(input.className).toContain("aria-invalid:border-af-danger-border");
    expect(input.className).toContain("aria-invalid:ring-af-danger-border");
  });
});
