import { render, screen } from "@testing-library/react";
import {
  DetailCardFactorySaveFeedback,
  mergeDetailCardSaveFieldErrors,
} from "./detail-card-factory-save-feedback";

const messages = {
  errorPrefix: "Saving failed.",
  staleVersionDetail:
    "Reload the latest running-factory values or keep this draft and retry after the editor refreshes.",
  successMessage: "Running factory saved for alpha-worker.",
};

describe("DetailCardFactorySaveFeedback", () => {
  it("renders nothing for idle, confirming, and submitting save states", () => {
    const { container, rerender } = render(
      <DetailCardFactorySaveFeedback
        messages={messages}
        saveState={{ status: "idle" }}
      />,
    );

    expect(container.firstChild).toBeNull();

    rerender(
      <DetailCardFactorySaveFeedback
        messages={messages}
        saveState={{ status: "confirming" }}
      />,
    );
    expect(container.firstChild).toBeNull();

    rerender(
      <DetailCardFactorySaveFeedback
        messages={messages}
        saveState={{ status: "submitting" }}
      />,
    );
    expect(container.firstChild).toBeNull();
  });

  it("renders success feedback with alert copy that inherits the success tone", () => {
    render(
      <DetailCardFactorySaveFeedback
        messages={messages}
        saveState={{ status: "success" }}
      />,
    );

    const status = screen.getByRole("status");
    const copy = screen.getByText(messages.successMessage);

    expect(status.className).toContain("bg-success-container");
    expect(copy.className).toContain("af-body-text");
    expect(copy.className).toContain("!text-current");
  });

  it("renders stale-version warnings with supporting detail copy", () => {
    render(
      <DetailCardFactorySaveFeedback
        messages={messages}
        saveState={{
          message: "Running factory changed on disk.",
          status: "warning",
        }}
      />,
    );

    const alert = screen.getByRole("alert");
    const detail = screen.getByText(messages.staleVersionDetail);

    expect(alert.className).toContain("bg-warning-container");
    expect(
      screen.getByText("Running factory changed on disk.").className,
    ).toContain("!text-current");
    expect(detail.className).toContain("af-supporting-text");
    expect(detail.className).toContain("text-on-surface-subtle");
  });

  it("renders save errors with inherited danger alert copy", () => {
    render(
      <DetailCardFactorySaveFeedback
        messages={messages}
        saveState={{
          errorMessage: "Could not write factory.json.",
          status: "error",
        }}
      />,
    );

    const alert = screen.getByRole("alert");
    const copy = screen.getByText(
      "Saving failed. Could not write factory.json.",
    );

    expect(alert.className).toContain("bg-error-container");
    expect(copy.className).toContain("!text-current");
  });
});

describe("mergeDetailCardSaveFieldErrors", () => {
  it("returns validation errors unchanged when save state has no field errors", () => {
    const validationErrors = { prompt: "Enter a prompt." };

    expect(
      mergeDetailCardSaveFieldErrors(validationErrors, { status: "success" }),
    ).toEqual(validationErrors);
    expect(
      mergeDetailCardSaveFieldErrors(validationErrors, {
        errorMessage: "Saving failed.",
        status: "error",
      }),
    ).toEqual(validationErrors);
  });

  it("merges save field errors over existing validation errors at the presentation boundary", () => {
    expect(
      mergeDetailCardSaveFieldErrors(
        { prompt: "Enter a prompt.", workerName: "Select a worker." },
        {
          errorMessage: "Saving failed.",
          fieldErrors: {
            prompt: "Prompt template is invalid.",
            runnerName: "Runner is unavailable.",
          },
          status: "error",
        },
      ),
    ).toEqual({
      prompt: "Prompt template is invalid.",
      runnerName: "Runner is unavailable.",
      workerName: "Select a worker.",
    });
  });
});
