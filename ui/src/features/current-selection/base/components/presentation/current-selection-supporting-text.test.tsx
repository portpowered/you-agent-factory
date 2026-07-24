import { render, screen } from "@testing-library/react";

import {
  CurrentSelectionSubtleCode,
  CurrentSelectionSupportingText,
} from "./current-selection-supporting-text";

describe("CurrentSelectionSupportingText", () => {
  it("renders notice text with supporting typography", () => {
    render(
      <CurrentSelectionSupportingText>Loading</CurrentSelectionSupportingText>,
    );

    const notice = screen.getByText("Loading");
    expect(notice.className).toContain("af-supporting-text");
    expect(notice.className).toContain("text-on-surface-variant");
  });

  it("renders status text with the subdued status tone", () => {
    render(
      <CurrentSelectionSupportingText tone="status">
        Selected
      </CurrentSelectionSupportingText>,
    );

    expect(screen.getByText("Selected").className).toContain(
      "text-on-surface-subtle",
    );
  });
});

describe("CurrentSelectionSubtleCode", () => {
  it("renders compact code with dashboard code typography", () => {
    render(<CurrentSelectionSubtleCode>input.foo</CurrentSelectionSubtleCode>);

    expect(screen.getByText("input.foo").className).toContain("af-body-code");
  });
});
