import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { CurrentSelectionTraceButton } from "./current-selection-trace-button";

describe("CurrentSelectionTraceButton", () => {
  it("selects a trace through a real button action", () => {
    const onSelectTraceID = vi.fn();

    render(
      <CurrentSelectionTraceButton
        onSelectTraceID={onSelectTraceID}
        traceID="trace-active-story"
      />,
    );

    const button = screen.getByRole("button", { name: "trace-active-story" });

    expect(button.tagName).toBe("BUTTON");
    expect(button.className).toContain("border-outline");

    fireEvent.click(button);

    expect(onSelectTraceID).toHaveBeenCalledWith("trace-active-story");
  });

  it("marks the active trace as pressed and appends the selected suffix", () => {
    render(
      <CurrentSelectionTraceButton
        activeTraceID="trace-active-story"
        selectedTraceSuffix=" selected"
        traceID="trace-active-story"
      />,
    );

    const button = screen.getByRole("button", {
      name: "trace-active-story selected",
    });

    expect(button.getAttribute("aria-pressed")).toBe("true");
  });
});
