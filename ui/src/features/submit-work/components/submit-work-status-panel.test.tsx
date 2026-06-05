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

    expect(screen.getByText("Choose a work type.").id).toBe(
      "submit-guidance",
    );
    expect(screen.getByText("Choose a work type.").className).toContain(
      "bg-surface-container-low",
    );
    expect(screen.getByText("Submitting work.").className).toContain(
      "bg-info-container",
    );
    expect(screen.getAllByRole("status")).toHaveLength(2);
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

    expect(screen.getByText("Submitted.").className).toContain(
      "bg-success-container",
    );
    expect(screen.getAllByRole("alert")).toHaveLength(2);
    expect(screen.getByText("Submit failed.").className).toContain(
      "bg-error-container",
    );
    expect(screen.getByText("Add at least one item.").className).toContain(
      "bg-error-container",
    );
  });
});
