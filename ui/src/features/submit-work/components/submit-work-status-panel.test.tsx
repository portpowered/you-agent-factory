import { render, screen } from "@testing-library/react";

import { SubmitWorkStatusPanel } from "./submit-work-status-panel";

describe("SubmitWorkStatusPanel", () => {
  it("renders guidance and submitting states as status panels", () => {
    render(
      <>
        <SubmitWorkStatusPanel
          id="submit-guidance"
          status={{ kind: "guidance", message: "Choose a work type." }}
        />
        <SubmitWorkStatusPanel
          id="submit-progress"
          status={{ kind: "submitting", message: "Submitting work." }}
        />
      </>,
    );

    const statuses = screen.getAllByRole("status");
    const guidanceCopy = screen.getByText("Choose a work type.");
    const submittingCopy = screen.getByText("Submitting work.");

    expect(statuses).toHaveLength(2);
    expect(document.getElementById("submit-guidance")?.className).toContain(
      "bg-surface-container-low",
    );
    expect(document.getElementById("submit-progress")?.className).toContain(
      "bg-info-container",
    );
    expect(document.getElementById("submit-progress")?.className).toContain(
      "w-full",
    );
    expect(document.getElementById("submit-progress")?.className).toContain(
      "max-w-none",
    );
    expect(guidanceCopy.className).toContain("!text-current");
    expect(submittingCopy.className).toContain("text-body-medium");
  });

  it("renders success and failure states with semantic alert tones", () => {
    render(
      <>
        <SubmitWorkStatusPanel
          id="submit-success"
          status={{ kind: "success", message: "Submitted." }}
        />
        <SubmitWorkStatusPanel
          id="submit-error"
          status={{ kind: "error", message: "Submit failed." }}
        />
        <SubmitWorkStatusPanel
          id="submit-validation"
          status={{
            kind: "validation-error",
            message: "Add at least one item.",
          }}
        />
      </>,
    );

    expect(document.getElementById("submit-success")?.className).toContain(
      "bg-success-container",
    );
    expect(screen.getAllByRole("alert")).toHaveLength(2);
    expect(document.getElementById("submit-error")?.className).toContain(
      "bg-error-container",
    );
    expect(document.getElementById("submit-validation")?.className).toContain(
      "bg-error-container",
    );
    expect(screen.getByText("Submitted.").className).toContain("!text-current");
    expect(screen.getByText("Submit failed.").className).toContain(
      "text-body-medium",
    );
  });
});
