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
      <DetailCardFactorySaveFeedback messages={messages} saveState={{ status: "idle" }} />,
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

  it("renders success feedback with role=status and caller-supplied success copy", () => {
    render(
      <DetailCardFactorySaveFeedback
        messages={messages}
        saveState={{ status: "success" }}
      />,
    );

    expect(screen.getByRole("status").textContent).toBe(messages.successMessage);
  });

  it("renders stale-version warning with role=alert and shared stale-version detail copy", () => {
    render(
      <DetailCardFactorySaveFeedback
        messages={messages}
        saveState={{
          message: "The running factory changed while this draft was open.",
          status: "warning",
        }}
      />,
    );

    expect(screen.getByRole("alert").textContent).toContain(
      "The running factory changed while this draft was open.",
    );
    expect(screen.getByText(messages.staleVersionDetail)).toBeTruthy();
  });

  it("renders API error feedback with role=alert and the shared error prefix", () => {
    render(
      <DetailCardFactorySaveFeedback
        messages={messages}
        saveState={{
          errorMessage: "Factory validation failed.",
          status: "error",
        }}
      />,
    );

    expect(screen.getByRole("alert").textContent).toBe(
      "Saving failed. Factory validation failed.",
    );
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
