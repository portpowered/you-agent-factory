import { render, screen } from "@testing-library/react";

import { FileInput } from "./file-input";

describe("FileInput", () => {
  it("renders native file input chrome with neutral dashboard tokens", () => {
    render(
      <FileInput aria-label="Factory cover image" className="custom-file" />,
    );

    const input = screen.getByLabelText("Factory cover image");

    expect(input.getAttribute("type")).toBe("file");
    expect(input.className).toContain("text-on-surface-variant");
    expect(input.className).toContain("file:bg-surface-container-high");
    expect(input.className).toContain("file:text-on-surface");
    expect(input.className).not.toContain("file:bg-primary-container");
    expect(input.className).not.toContain("file:text-primary");
    expect(input.className).toContain("custom-file");
  });
});
