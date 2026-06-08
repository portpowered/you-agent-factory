import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { Checkbox } from "./checkbox";

describe("Checkbox", () => {
  it("renders a styled indicator while preserving native checkbox semantics", () => {
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
    expect(checkbox.className).toContain("sr-only");
    expect(checkbox.getAttribute("checked")).toBe("");

    const indicator = checkbox.nextElementSibling;
    expect(indicator?.className).toContain("border-outline");
    expect(indicator?.className).toContain("peer-checked:bg-primary");
    expect(indicator?.className).toContain(
      "peer-focus-visible:ring-af-focus-ring",
    );
    expect(indicator?.querySelector("svg")).toBeTruthy();
    expect(checkbox.parentElement?.className).toContain("mr-2");
  });

  it("hides the checked indicator when unchecked", () => {
    render(
      <Checkbox
        aria-label="Optional setting"
        checked={false}
        onChange={() => undefined}
      />,
    );

    const checkbox = screen.getByRole("checkbox", { name: "Optional setting" });
    const indicator = checkbox.nextElementSibling;

    expect(checkbox.getAttribute("checked")).toBeNull();
    expect(indicator?.querySelector("svg")?.getAttribute("class")).toContain(
      "opacity-0",
    );
  });

  it("toggles with label clicks and Space while focused", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();

    render(
      <>
        <label htmlFor="cron-trigger-checkbox">Enable cron trigger</label>
        <Checkbox id="cron-trigger-checkbox" onChange={onChange} />
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

  it("applies disabled and invalid styling through the shared indicator", () => {
    render(
      <Checkbox
        aria-invalid="true"
        aria-label="Skip permissions"
        disabled
        onChange={() => undefined}
      />,
    );

    const checkbox = screen.getByRole("checkbox", { name: "Skip permissions" });
    const indicator = checkbox.nextElementSibling;

    expect(checkbox.hasAttribute("disabled")).toBe(true);
    expect(checkbox.getAttribute("aria-invalid")).toBe("true");
    expect(indicator?.className).toContain(
      "peer-disabled:bg-surface-container-low",
    );
    expect(indicator?.className).toContain(
      "peer-aria-invalid:ring-af-danger-border",
    );
    expect(checkbox.parentElement?.className).toContain(
      "[&:has(:disabled)]:cursor-not-allowed",
    );
  });
});
