import { render, screen } from "@testing-library/react";

import { Checkbox } from "./checkbox";

describe("Checkbox", () => {
  it("renders shared checkbox styling and preserves input attributes", () => {
    render(
      <Checkbox
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
    expect(checkbox.className).toContain("size-4");
    expect(checkbox.className).toContain("border-outline");
    expect(checkbox.className).toContain("focus-visible:ring-af-focus-ring");
    expect(checkbox.className).toContain("mr-2");
    expect(checkbox.getAttribute("checked")).toBe("");
  });
});
