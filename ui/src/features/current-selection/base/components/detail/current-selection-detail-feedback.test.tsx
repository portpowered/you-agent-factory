import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { CurrentSelectionDetailFeedback } from "./current-selection-detail-feedback";

describe("CurrentSelectionDetailFeedback", () => {
  it("renders neutral inline detail feedback", () => {
    render(
      <CurrentSelectionDetailFeedback>
        Loading current definition.
      </CurrentSelectionDetailFeedback>,
    );

    const feedback = screen.getByText("Loading current definition.");

    expect(feedback.tagName).toBe("P");
    expect(feedback.className).toContain("text-body-medium");
    expect(feedback.className).toContain("text-on-surface-variant");
  });

  it("renders danger feedback with alert semantics", () => {
    render(
      <CurrentSelectionDetailFeedback role="alert" tone="danger">
        Definition unavailable.
      </CurrentSelectionDetailFeedback>,
    );

    const alert = screen.getByRole("alert");

    expect(alert.textContent).toBe("Definition unavailable.");
    expect(alert.className).toContain("text-on-error-container");
  });

  it("renders danger feedback with status semantics when requested", () => {
    render(
      <CurrentSelectionDetailFeedback role="status" tone="danger">
        Definition changed.
      </CurrentSelectionDetailFeedback>,
    );

    expect(screen.getByRole("status").textContent).toBe("Definition changed.");
  });
});
