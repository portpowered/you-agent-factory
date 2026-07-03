import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { WIDGET_FRAME_BODY_TEXT_CLASS } from "@you-agent-factory/components/recipes";
import { inferenceAttempt } from "../../../base/components/detail-card/detail-card-test-helpers";
import { CurrentSelectionLocaleProvider } from "../../../base/components/presentation/current-selection-locale";
import {
  InferenceAttemptRequestBodySection,
  InferenceAttemptResponseSection,
} from "./inference-attempt-body-sections";

describe("inference attempt body sections", () => {
  it("renders request body disclosure for non-empty prompts", () => {
    render(
      <InferenceAttemptRequestBodySection
        attempt={inferenceAttempt("dispatch-review", {
          prompt: "  Review the work item.  ",
        })}
      />,
    );

    const trigger = screen.getByRole("button", {
      name: "Expand request body",
    });

    expect(trigger.getAttribute("aria-expanded")).toBe("false");

    fireEvent.click(trigger);

    expect(trigger.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByText("Review the work item.")).toBeTruthy();
  });

  it("omits request body disclosure for empty prompts", () => {
    const { container } = render(
      <InferenceAttemptRequestBodySection
        attempt={inferenceAttempt("dispatch-review", {
          prompt: "   ",
        })}
      />,
    );

    expect(container.firstChild).toBeNull();
  });

  it("renders response body disclosure when response text is available", () => {
    render(
      <InferenceAttemptResponseSection
        attempt={inferenceAttempt("dispatch-review", {
          response: "Ready for handoff.",
        })}
      />,
    );

    const trigger = screen.getByRole("button", {
      name: "Expand response body",
    });

    fireEvent.click(trigger);

    expect(screen.getByText("Ready for handoff.")).toBeTruthy();
  });

  it("renders unavailable response copy for completed attempts without response text", () => {
    render(
      <InferenceAttemptResponseSection
        attempt={inferenceAttempt("dispatch-review", {
          outcome: "FAILED",
          response: undefined,
        })}
      />,
    );

    expect(
      screen.getByText(
        "Provider response text is not available for this inference attempt.",
      ).className,
    ).toContain(WIDGET_FRAME_BODY_TEXT_CLASS);
  });

  it("renders awaiting response copy for pending attempts", () => {
    render(
      <InferenceAttemptResponseSection
        attempt={inferenceAttempt("dispatch-review", {
          outcome: undefined,
          response: undefined,
        })}
      />,
    );

    expect(screen.getByText("Awaiting provider response.")).toBeTruthy();
  });

  it("localizes body disclosure actions", () => {
    render(
      <CurrentSelectionLocaleProvider locale="zh-CN">
        <InferenceAttemptResponseSection
          attempt={inferenceAttempt("dispatch-review", {
            response: "完成。",
          })}
        />
      </CurrentSelectionLocaleProvider>,
    );

    expect(screen.getByRole("button", { name: "展开响应正文" })).toBeTruthy();
  });
});
